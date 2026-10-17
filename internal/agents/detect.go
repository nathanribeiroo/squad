package agents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/config"
)

// The commands that start each agent, as the user set them (Configurações →
// Agentes): ~/.torre/agents.json. An agent without one uses its own name.
var commands struct {
	sync.Mutex
	loaded bool
	m      map[string]string
}

// commandsPath is ~/.torre/agents.json ("" without a home folder).
func commandsPath() string {
	h, err := config.Home()
	if err != nil {
		return ""
	}
	return filepath.Join(h, "agents.json")
}

func loadCommandsLocked() {
	if commands.loaded {
		return
	}
	commands.loaded = true
	commands.m = map[string]string{}
	if p := commandsPath(); p == "" {
		return
	} else if data, err := os.ReadFile(p); err == nil {
		var f struct {
			Commands map[string]string `json:"commands"`
		}
		if json.Unmarshal(data, &f) == nil && f.Commands != nil {
			commands.m = f.Commands
		}
	}
}

// DefaultCommand is the command that starts an agent when its terminal has
// none of its own: the user's, else the agent's name.
func DefaultCommand(agent string) string {
	commands.Lock()
	defer commands.Unlock()
	loadCommandsLocked()
	if c := commands.m[agent]; c != "" {
		return c
	}
	return Lookup(agent).DefaultCommand
}

// ErrBadCommand refuses a default command: empty is a reset, so this is one
// with more than one line or too long.
var ErrBadCommand = errors.New("the command is one line of at most 300 characters")

// SetDefaultCommand changes the command that starts an agent ("" goes back
// to its name). Only the interface sets it; terminals started afterwards use
// it.
func SetDefaultCommand(agent, cmd string) error {
	if !IsAgentType(agent) {
		return errors.New("unknown agent " + agent)
	}
	cmd = strings.TrimSpace(cmd)
	if strings.ContainsAny(cmd, "\r\n") || utf8.RuneCountInString(cmd) > 300 {
		return ErrBadCommand
	}
	if err := saveCommand(agent, cmd); err != nil {
		return err
	}
	// Look again (after the commands' lock: Detect takes it inside its own).
	detection.Lock()
	detection.at = time.Time{}
	detection.Unlock()
	return nil
}

func saveCommand(agent, cmd string) error {
	commands.Lock()
	defer commands.Unlock()
	loadCommandsLocked()
	next := map[string]string{}
	for k, v := range commands.m {
		next[k] = v
	}
	if cmd == "" || cmd == Lookup(agent).DefaultCommand {
		delete(next, agent)
	} else {
		next[agent] = cmd
	}
	data, err := json.MarshalIndent(map[string]any{"commands": next}, "", "  ")
	if err != nil {
		return err
	}
	path := commandsPath()
	if path == "" {
		return errors.New("no home folder to keep the command in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return err
	}
	commands.m = next
	return nil
}

// IsAgentType reports whether agent is one of the coding agents (not a
// shell or an own command).
func IsAgentType(agent string) bool {
	switch agent {
	case "claude", "codex", "devin", "gemini":
		return true
	}
	return false
}

// Detected is an agent as found on this computer.
type Detected struct {
	Agent          string `json:"agent"`
	Found          bool   `json:"found"`
	Version        string `json:"version,omitempty"`
	Path           string `json:"path,omitempty"`
	DefaultCommand string `json:"defaultCommand"`
	Custom         bool   `json:"custom"` // the user changed the command
	InstallHint    string `json:"installHint,omitempty"`
}

// Detection is GET /api/agents: the agents and the browser for portals.
type Detection struct {
	Agents  []Detected `json:"agents"`
	Browser struct {
		Found bool   `json:"found"`
		Path  string `json:"path,omitempty"`
	} `json:"browser"`
}

var installHints = map[string]string{
	"claude": "npm install -g @anthropic-ai/claude-code",
	"codex":  "npm install -g @openai/codex",
	"devin":  "curl -fsSL https://cli.devin.ai/install.sh | bash",
	"gemini": "npm install -g @google/gemini-cli",
}

var detection struct {
	sync.Mutex
	at   time.Time
	last Detection
}

// Detect finds the agents (the first word of each default command on the
// PATH, and its --version) and the browser. Results last five minutes;
// refresh looks again.
func Detect(refresh bool) Detection {
	detection.Lock()
	defer detection.Unlock()
	if !refresh && !detection.at.IsZero() && time.Since(detection.at) < 5*time.Minute {
		return detection.last
	}
	var d Detection
	var wg sync.WaitGroup
	d.Agents = make([]Detected, 4)
	for i, agent := range []string{"claude", "codex", "devin", "gemini"} {
		cmd := DefaultCommand(agent)
		d.Agents[i] = Detected{Agent: agent, DefaultCommand: cmd, Custom: cmd != Lookup(agent).DefaultCommand, InstallHint: installHints[agent]}
		fields := strings.Fields(cmd)
		if len(fields) == 0 {
			continue
		}
		path, err := exec.LookPath(fields[0])
		if err != nil {
			continue
		}
		d.Agents[i].Found, d.Agents[i].Path = true, path
		wg.Add(1)
		go func(a *Detected) {
			defer wg.Done()
			a.Version = version(a.Path)
		}(&d.Agents[i])
	}
	if p := browser.Find(); p != "" {
		d.Browser.Found, d.Browser.Path = true, p
	}
	wg.Wait()
	detection.at, detection.last = time.Now(), d
	return d
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// version is the first line `path --version` prints (within 4 s).
func version(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	for _, l := range strings.Split(ansi.ReplaceAllString(string(out), ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if utf8.RuneCountInString(l) > 60 {
				l = string([]rune(l)[:60])
			}
			return l
		}
	}
	return ""
}

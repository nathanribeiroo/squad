// Package agents adapts known agent CLIs: how each one is launched inside a
// Squad terminal and how it tells Squad that a turn ended (its native hook),
// so replies come from the agent's final message instead of the screen.
//
//   - Claude Code: a settings file passed with --settings adds Stop,
//     StopFailure and UserPromptSubmit hooks that run `squad hook …`. Hooks
//     from settings files are merged, so the user's own hooks keep working.
//   - Codex: -c notify=[…] runs `squad hook codex-notify <json>` on
//     agent-turn-complete (not tested against a real Codex account).
//   - Gemini, shells and custom commands: no hook; screen heuristics only.
package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Info describes an agent type.
type Info struct {
	Type           string // claude | codex | gemini | shell | custom
	DefaultCommand string // used when the node has no command
	Hooks          bool   // reports turn ends through a native hook
}

var known = map[string]Info{
	"claude": {Type: "claude", DefaultCommand: "claude", Hooks: true},
	"codex":  {Type: "codex", DefaultCommand: "codex", Hooks: true},
	"gemini": {Type: "gemini", DefaultCommand: "gemini"},
	"shell":  {Type: "shell"},
	"custom": {Type: "custom"},
}

// Lookup returns the info for an agent type (custom when unknown).
func Lookup(agent string) Info {
	if i, ok := known[agent]; ok {
		return i
	}
	return known["custom"]
}

// HasHooks reports whether an agent type signals turn ends explicitly.
func HasHooks(agent string) bool { return Lookup(agent).Hooks }

// Launcher prepares launch lines for agents.
type Launcher struct {
	SquadPath      string // absolute path of the squad binary
	ClaudeSettings string // path of the generated Claude settings file
}

// NewLauncher writes the Claude hooks settings file under ~/.squad/run.
func NewLauncher(squadPath string) (*Launcher, error) {
	dir, err := config.Dir("run")
	if err != nil {
		return nil, err
	}
	l := &Launcher{SquadPath: squadPath, ClaudeSettings: filepath.Join(dir, "claude-hooks.json")}
	return l, l.writeClaudeSettings()
}

// hookCommand is the command line Claude Code runs for a hook (through a
// shell), with the squad path quoted.
func (l *Launcher) hookCommand(name string) string {
	return fmt.Sprintf(`"%s" hook %s`, l.SquadPath, name)
}

type claudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type claudeMatcher struct {
	Hooks []claudeHook `json:"hooks"`
}

func (l *Launcher) writeClaudeSettings() error {
	hook := func(name string) []claudeMatcher {
		return []claudeMatcher{{Hooks: []claudeHook{{Type: "command", Command: l.hookCommand(name), Timeout: 10}}}}
	}
	settings := map[string]any{"hooks": map[string]any{
		"Stop":             hook("claude-stop"),
		"StopFailure":      hook("claude-stop-failure"),
		"UserPromptSubmit": hook("claude-prompt"),
	}}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(l.ClaudeSettings, data, 0o600)
}

// Prepare rewrites a terminal spec before start: default command for the
// agent type plus the flags that install Squad's hooks.
func (l *Launcher) Prepare(n store.Node, spec *terminal.Spec) error {
	info := Lookup(spec.Agent)
	if strings.TrimSpace(spec.Command) == "" {
		spec.Command = info.DefaultCommand
	}
	if spec.Command == "" {
		return nil
	}
	_, fam := terminal.ResolveShell(spec.Shell)
	switch info.Type {
	case "claude":
		spec.Command += " --settings " + terminal.Quote(fam, l.ClaudeSettings)
	case "codex":
		spec.Command += " -c " + terminal.Quote(fam, l.codexNotify())
	}
	return nil
}

// codexNotify returns the TOML override for Codex's notify program; TOML
// basic strings share JSON's escaping, so JSON-encoded strings are valid.
func (l *Launcher) codexNotify() string {
	parts := []string{l.SquadPath, "hook", "codex-notify"}
	enc := make([]string, len(parts))
	for i, p := range parts {
		b, _ := json.Marshal(p)
		enc[i] = string(b)
	}
	return "notify=[" + strings.Join(enc, ",") + "]"
}

// LastAssistantFromTranscript returns the text of the last assistant message
// in a Claude Code transcript (JSONL). Only the tail of the file is read.
func LastAssistantFromTranscript(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	const tail = 4 << 20
	if fi, err := f.Stat(); err == nil && fi.Size() > tail {
		if _, err := f.Seek(fi.Size()-tail, 0); err != nil {
			return "", err
		}
	}
	data := make([]byte, tail)
	n, _ := f.Read(data)
	lines := strings.Split(string(data[:n]), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(lines[i]), &entry) != nil || entry.Type != "assistant" {
			continue
		}
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(entry.Message.Content, &parts) == nil {
			var texts []string
			for _, p := range parts {
				if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
					texts = append(texts, p.Text)
				}
			}
			if len(texts) > 0 {
				return strings.Join(texts, "\n"), nil
			}
			continue
		}
		var s string
		if json.Unmarshal(entry.Message.Content, &s) == nil && strings.TrimSpace(s) != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("no assistant message in %s", path)
}

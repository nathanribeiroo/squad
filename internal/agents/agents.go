// Package agents adapts known agent CLIs: how each one is launched inside a
// Squad terminal and how it tells Squad that a turn ended (its native hook),
// so replies come from the agent's final message instead of the screen.
//
//   - Claude Code: a settings file passed with --settings adds Stop,
//     StopFailure, UserPromptSubmit and SessionStart hooks that run `squad
//     hook …`. Hooks from settings files are merged, so the user's own hooks
//     keep working.
//   - Codex: -c notify=[…] runs `squad hook codex-notify <json>` on
//     agent-turn-complete (not tested against a real Codex account).
//   - Devin CLI: --config replaces the user's config.json, so Squad passes
//     a copy of it with Stop, UserPromptSubmit and SessionStart hooks added
//     (Claude Code's format) that run `squad hook devin-…` (writeDevinConfig;
//     the hooks themselves not tested against a real Devin account).
//   - Gemini, shells and custom commands: no hook; screen heuristics only.
//
// A terminal's role reaches its agent as instructions at launch: Claude Code
// reads them from a file (--append-system-prompt-file), Codex from a config
// override (developer_instructions), Devin from its SessionStart hook, and
// Gemini, which has no such option, as its first prompt (-i). Shells and
// custom commands get none: typing instructions into them could run them.
//
// Squad's commands also reach agents as MCP tools (squad mcp): Claude Code
// gets them with --mcp-config and pre-approves the safe ones (SafeMCPTools),
// Codex with mcp_servers config overrides. Devin reads MCP servers only from
// its mcp_config.json, where squad install-skill registers squad; its config
// copy pre-approves the same safe tools.
//
// Each terminal continues its agent's conversation when it starts again
// (conversation.go): Squad remembers the conversation id, from the launch line
// or the hooks.
//
// On a floor (a git worktree), Devin skips its workspace trust prompt: the
// floor is a copy of the repository the user opened in Squad. Claude Code
// doesn't ask there.
package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Info describes an agent type.
type Info struct {
	Type           string // claude | codex | devin | gemini | shell | custom
	DefaultCommand string // used when the node has no command
	Hooks          bool   // reports turn ends through a native hook
	Roles          bool   // receives its role's instructions
}

var known = map[string]Info{
	"claude": {Type: "claude", DefaultCommand: "claude", Hooks: true, Roles: true},
	"codex":  {Type: "codex", DefaultCommand: "codex", Hooks: true, Roles: true},
	"devin":  {Type: "devin", DefaultCommand: "devin", Hooks: true, Roles: true},
	"gemini": {Type: "gemini", DefaultCommand: "gemini", Roles: true},
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

// SafeMCPTools are the squad mcp tools an agent may use without asking the
// user: in their calls the server refuses anything that would run a command
// (keys typed into a terminal, a message to a shell, a new terminal with a
// command line, JavaScript in a page). keys, new_shell, portal_eval and
// floor_land (it merges branches and closes terminals) are the others: the
// user approves each call.
var SafeMCPTools = []string{"list", "ask", "ask_batch", "check", "wait", "reply", "note", "portal", "new", "connect", "close", "floors", "roles"}

// mcpEnv is what squad mcp needs from the terminal's environment.
var mcpEnv = []string{"SQUAD_URL", "SQUAD_TERMINAL_ID", "SQUAD_TOKEN"}

// Launcher prepares launch lines for agents.
type Launcher struct {
	SquadPath      string // absolute path of the squad binary
	ClaudeSettings string // path of the generated Claude settings file
	ClaudeMCP      string // path of the generated Claude MCP config (squad mcp)
	DevinConfig    string // path of the generated Devin config file
	// DevinUserConfig is the user's Devin config.json, copied into
	// DevinConfig ("" when unknown).
	DevinUserConfig string
	RoleDir         string // one file per terminal with its role, for Claude Code
	// Role returns the instructions of a terminal's role (false: no role).
	Role func(n store.Node) (string, bool)
	// HasFlag reports whether a program's --help mentions a flag (nil: ask
	// the program; tests replace it).
	HasFlag func(program, flag string) bool

	devinMu sync.Mutex
}

// NewLauncher writes the Claude hooks and MCP files under ~/.squad/run; the
// Devin config is written at each Devin launch (Prepare).
func NewLauncher(squadPath string) (*Launcher, error) {
	dir, err := config.Dir("run")
	if err != nil {
		return nil, err
	}
	l := &Launcher{SquadPath: squadPath,
		ClaudeSettings: filepath.Join(dir, "claude-hooks.json"),
		ClaudeMCP:      filepath.Join(dir, "claude-mcp.json"),
		DevinConfig:    filepath.Join(dir, "devin-config.json"),
		RoleDir:        filepath.Join(dir, "roles")}
	if d, err := DevinConfigDir(); err == nil {
		l.DevinUserConfig = filepath.Join(d, "config.json")
	}
	_ = os.Remove(filepath.Join(dir, "devin-hooks.json")) // before: hooks only, without the user's config
	allow := make([]string, len(SafeMCPTools))
	for i, t := range SafeMCPTools {
		allow[i] = "mcp__squad__" + t
	}
	servers := map[string]any{"squad": map[string]any{"command": squadPath, "args": []string{"mcp"}}}
	if err := l.writeHooks(l.ClaudeSettings, map[string]string{
		"Stop":             "claude-stop",
		"StopFailure":      "claude-stop-failure",
		"UserPromptSubmit": "claude-prompt",
		"SessionStart":     "claude-session-start",
	}, map[string]any{"permissions": map[string]any{"allow": allow}}); err != nil {
		return nil, err
	}
	if err := writeJSON(l.ClaudeMCP, map[string]any{"mcpServers": servers}); err != nil {
		return nil, err
	}
	return l, nil
}

// hookCommand is the command line Claude Code (or Devin) runs for a hook
// (through a shell), with the squad path quoted.
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

// writeHooks writes a settings file whose "hooks" key maps each event to
// `squad hook <name>`, plus the extra keys (Claude Code's --settings; Devin's
// config copy uses the same hook format).
func (l *Launcher) writeHooks(path string, events map[string]string, extra map[string]any) error {
	hooks := map[string][]claudeMatcher{}
	for event, name := range events {
		hooks[event] = []claudeMatcher{{Hooks: []claudeHook{{Type: "command", Command: l.hookCommand(name), Timeout: 10}}}}
	}
	doc := map[string]any{"hooks": hooks}
	for k, v := range extra {
		doc[k] = v
	}
	return writeJSON(path, doc)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(path, data, 0o600)
}

// Prepare rewrites a terminal spec before start: default command for the
// agent type plus the flags that install Squad's hooks, pass its role and
// start or continue its conversation (conversation.go). It sets
// n.Terminal.Conversation to the conversation the terminal is in ("" when
// the agent picks the id and reports it later) and reports whether the line
// continues one.
func (l *Launcher) Prepare(n *store.Node, spec *terminal.Spec) (resumed bool, err error) {
	if n.Terminal == nil {
		n.Terminal = &store.TerminalData{}
	}
	info := Lookup(spec.Agent)
	if strings.TrimSpace(spec.Command) == "" {
		spec.Command = info.DefaultCommand
	}
	if spec.Command == "" {
		return false, nil
	}
	_, fam := terminal.ResolveShell(spec.Shell)
	role, hasRole := "", false
	if l.Role != nil && info.Roles {
		role, hasRole = l.Role(*n)
	}
	user := userPicksConversation(info.Type, spec.Command)
	tail := ""
	switch info.Type {
	case "claude":
		spec.Command += " --settings " + terminal.Quote(fam, l.ClaudeSettings)
		spec.Command += " --mcp-config " + terminal.Quote(fam, l.ClaudeMCP)
		path := filepath.Join(l.RoleDir, n.ID+".md")
		if !hasRole {
			_ = os.Remove(path)
			break
		}
		if err := os.MkdirAll(l.RoleDir, 0o700); err != nil {
			return false, err
		}
		if err := config.WriteFileAtomic(path, []byte(role), 0o600); err != nil {
			return false, err
		}
		spec.Command += " --append-system-prompt-file " + terminal.Quote(fam, path)
	case "codex":
		spec.Command += " -c " + terminal.Quote(fam, l.codexNotify())
		for _, o := range l.codexMCP() {
			spec.Command += " -c " + terminal.Quote(fam, o)
		}
		if hasRole {
			spec.Command += " -c " + terminal.Quote(fam, "developer_instructions="+TOMLString(role))
		}
	case "devin":
		if err := l.writeDevinConfig(); err != nil {
			return false, err
		}
		spec.Command += " --config " + terminal.Quote(fam, l.DevinConfig)
		if n.Floor != "" {
			spec.Command += " --respect-workspace-trust false"
		}
	case "gemini":
		if hasRole {
			// From the environment: the launch line stays short and unquoted.
			if fam == terminal.ShellCmd { // %VAR% can't hold line breaks or quotes
				role = strings.NewReplacer("\r", "", "\n", " ", `"`, "'").Replace(role)
			}
			if spec.Env == nil {
				spec.Env = map[string]string{}
			}
			spec.Env["SQUAD_ROLE"] = role
			// The first prompt: a continued conversation already has it.
			tail = " -i " + envRef(fam, "SQUAD_ROLE")
		}
	}
	if user {
		spec.Command += tail
		return false, nil
	}
	conv := n.Terminal.Conversation
	if n.Terminal.Empty { // nothing said yet: nothing to continue
		conv = ""
	}
	spec.Command, n.Terminal.Conversation, resumed = l.conversationLine(info.Type, spec.Command, tail, conv, fam)
	return resumed, nil
}

// envRef is an environment variable as one argument of a shell command line.
func envRef(f terminal.ShellFamily, name string) string {
	switch f {
	case terminal.ShellPowerShell:
		return "$env:" + name
	case terminal.ShellCmd:
		return `"%` + name + `%"`
	default:
		return `"$` + name + `"`
	}
}

// Forget removes what Prepare wrote for a terminal.
func (l *Launcher) Forget(id string) {
	_ = os.Remove(filepath.Join(l.RoleDir, id+".md"))
}

// TOMLString encodes s as a TOML basic string that is also plain ASCII with
// no shell metacharacters: everything but letters, digits, spaces and a few
// marks becomes \uXXXX, so no shell (sh, PowerShell, cmd with its %VAR%)
// changes it on a launch line.
func TOMLString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', strings.ContainsRune(" ,.:;-_()?/=+*#@[]{}", r):
			b.WriteRune(r)
		case r > 0xFFFF:
			fmt.Fprintf(&b, `\U%08X`, r)
		default:
			fmt.Fprintf(&b, `\u%04X`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
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

// codexMCP returns the config overrides that give Codex squad mcp: Codex
// passes stdio servers only a few environment variables unless listed, and
// waits 60 s for a tool by default (an ask may take minutes).
func (l *Launcher) codexMCP() []string {
	vars := make([]string, len(mcpEnv))
	for i, v := range mcpEnv {
		vars[i] = TOMLString(v)
	}
	return []string{
		"mcp_servers.squad.command=" + TOMLString(l.SquadPath),
		`mcp_servers.squad.args=["mcp"]`,
		"mcp_servers.squad.env_vars=[" + strings.Join(vars, ",") + "]",
		"mcp_servers.squad.tool_timeout_sec=1800",
	}
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

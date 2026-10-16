package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nathanribeiroo/squad/internal/config"
)

// devinHooks are Squad's hooks for Devin CLI, in Claude Code's format.
var devinHooks = map[string]string{
	"Stop":             "devin-stop",
	"UserPromptSubmit": "devin-prompt",
	"SessionStart":     "devin-session-start", // the role, as context
}

// DevinConfigDir is Devin CLI's user configuration folder: devin under
// $XDG_CONFIG_HOME (default ~/.config, also on macOS), or under %APPDATA% on
// Windows. Its skills are in skills/, its MCP servers in mcp_config.json.
func DevinConfigDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if runtime.GOOS == "windows" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "devin"), nil
}

// writeDevinConfig writes the file Devin gets with --config. That flag
// replaces the user's config.json instead of adding to it, so the file is the
// user's config, read again at each launch, plus Squad's hooks (after the
// user's own) and the safe squad MCP tools allowed. Permissions Devin saved
// into the file during a Squad session ("save to global config") are kept.
// MCP servers are not in it: Devin reads them only from mcp_config.json
// (squad install-skill registers squad there).
func (l *Launcher) writeDevinConfig() error {
	l.devinMu.Lock()
	defer l.devinMu.Unlock()
	doc := map[string]any{}
	if l.DevinUserConfig != "" {
		if data, err := os.ReadFile(l.DevinUserConfig); err == nil {
			if json.Unmarshal(JSONC(data), &doc) != nil {
				doc = map[string]any{} // unreadable: Squad's part alone, as before
			}
		}
	}
	var prev map[string]any
	if data, err := os.ReadFile(l.DevinConfig); err == nil {
		_ = json.Unmarshal(JSONC(data), &prev)
	}

	perms, saved := object(doc["permissions"]), object(prev["permissions"])
	for _, k := range []string{"allow", "ask", "deny"} {
		items := union(list(perms[k]), list(saved[k]))
		if k == "allow" {
			for _, t := range SafeMCPTools {
				items = union(items, []any{"mcp__squad__" + t})
			}
		}
		if len(items) > 0 {
			perms[k] = items
		}
	}
	doc["permissions"] = perms

	hooks := object(doc["hooks"])
	for event, name := range devinHooks {
		hooks[event] = append(list(hooks[event]), claudeMatcher{Hooks: []claudeHook{{Type: "command", Command: l.hookCommand(name), Timeout: 10}}})
	}
	doc["hooks"] = hooks
	return writeJSON(l.DevinConfig, doc)
}

// AddDevinMCP registers the squad MCP server in Devin's mcp_config.json at
// path, keeping the other servers; the file it replaces is kept once as
// .squad-backup. It reports whether the file changed.
func AddDevinMCP(path, squadPath string) (bool, error) {
	doc := map[string]any{}
	old, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(JSONC(old), &doc); err != nil {
			return false, fmt.Errorf("%s: %v (left unchanged)", path, err)
		}
	case !os.IsNotExist(err):
		return false, err
	}
	servers := object(doc["mcpServers"])
	entry := map[string]any{"command": squadPath, "args": []any{"mcp"}}
	if cur, err := json.Marshal(servers["squad"]); err == nil {
		if want, _ := json.Marshal(entry); string(cur) == string(want) {
			return false, nil
		}
	}
	servers["squad"] = entry
	doc["mcpServers"] = servers
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if len(old) > 0 {
		backup := path + ".squad-backup"
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			if err := os.WriteFile(backup, old, 0o600); err != nil {
				return false, err
			}
		}
	}
	return true, config.WriteFileAtomic(path, append(data, '\n'), 0o600)
}

// object is v as a JSON object (a new one when it isn't).
func object(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// list is v as a JSON array (nil when it isn't).
func list(v any) []any {
	l, _ := v.([]any)
	return l
}

// union appends to a the items of b it doesn't have yet.
func union(a, b []any) []any {
	seen := map[string]bool{}
	key := func(v any) string { k, _ := json.Marshal(v); return string(k) }
	for _, v := range a {
		seen[key(v)] = true
	}
	for _, v := range b {
		if k := key(v); !seen[k] {
			seen[k] = true
			a = append(a, v)
		}
	}
	return a
}

// JSONC turns JSON with comments and trailing commas (Devin's config files)
// into plain JSON.
func JSONC(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	out := make([]byte, 0, len(data))
	inString, escaped := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			i++ // the closing '/'
			out = append(out, ' ')
		case c == ']' || c == '}':
			// Drop a comma before the closing bracket (only spaces between).
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

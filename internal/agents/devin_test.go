package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

func TestJSONC(t *testing.T) {
	in := "\uFEFF{\n  // the model\n  \"model\": \"opus\", /* inline */\n  \"url\": \"http://x//y /* not a comment */\",\n  \"list\": [1, 2,],\n  \"esc\": \"a\\\"//b\",\n}\n"
	var v map[string]any
	if err := json.Unmarshal(JSONC([]byte(in)), &v); err != nil {
		t.Fatalf("%v\n%s", err, JSONC([]byte(in)))
	}
	if v["model"] != "opus" || v["url"] != "http://x//y /* not a comment */" || v["esc"] != `a"//b` || len(v["list"].([]any)) != 2 {
		t.Fatalf("%v", v)
	}
}

// Devin's --config replaces the user's config.json: Squad passes a copy with
// its hooks added, and keeps what Devin saved into that copy.
func TestDevinConfigKeepsTheUsers(t *testing.T) {
	t.Setenv("SQUAD_HOME", t.TempDir())
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	user := filepath.Join(cfg, "devin", "config.json")
	_ = os.MkdirAll(filepath.Dir(user), 0o700)
	_ = os.WriteFile(user, []byte(`{
  // mine
  "agent": {"model": "opus"},
  "permissions": {"allow": ["Exec(git status)"], "deny": ["Write(.env*)"]},
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "notify-me"}]}]},
}`), 0o600)
	l, err := NewLauncher("/opt/squad/bin/squad")
	if err != nil {
		t.Fatal(err)
	}
	if l.DevinUserConfig != user {
		t.Fatalf("user config at %q, want %q", l.DevinUserConfig, user)
	}
	type hook struct{ Command string }
	type doc struct {
		Agent       struct{ Model string }
		Permissions struct{ Allow, Deny []string }
		Hooks       map[string][]struct{ Hooks []hook }
		MCPServers  map[string]any `json:"mcpServers"`
	}
	read := func() doc {
		var d doc
		data, err := os.ReadFile(l.DevinConfig)
		if err != nil || json.Unmarshal(data, &d) != nil {
			t.Fatalf("devin config: %v\n%s", err, data)
		}
		return d
	}
	prepare := func() {
		spec := terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
		if err := l.Prepare(store.Node{}, &spec); err != nil {
			t.Fatal(err)
		}
	}
	prepare()
	d := read()
	if d.Agent.Model != "opus" || !slices.Contains(d.Permissions.Allow, "Exec(git status)") || !slices.Contains(d.Permissions.Deny, "Write(.env*)") {
		t.Fatalf("user settings lost: %+v", d)
	}
	if !slices.Contains(d.Permissions.Allow, "mcp__squad__ask") || slices.Contains(d.Permissions.Allow, "mcp__squad__keys") ||
		slices.Contains(d.Permissions.Allow, "mcp__squad__floor_land") || d.MCPServers != nil {
		t.Fatalf("squad tools: %+v", d)
	}
	stop := d.Hooks["Stop"]
	if len(stop) != 2 || stop[0].Hooks[0].Command != "notify-me" || !strings.Contains(stop[1].Hooks[0].Command, "hook devin-stop") ||
		len(d.Hooks["SessionStart"]) != 1 || len(d.Hooks["UserPromptSubmit"]) != 1 {
		t.Fatalf("hooks: %+v", d.Hooks)
	}

	// Devin saves an approval into the copy; the next launch keeps it and
	// doesn't add Squad's hooks twice.
	var raw map[string]any
	data, _ := os.ReadFile(l.DevinConfig)
	_ = json.Unmarshal(data, &raw)
	perms := raw["permissions"].(map[string]any)
	perms["allow"] = append(perms["allow"].([]any), "Exec(npm test)")
	data, _ = json.Marshal(raw)
	_ = os.WriteFile(l.DevinConfig, data, 0o600)
	prepare()
	d = read()
	if !slices.Contains(d.Permissions.Allow, "Exec(npm test)") || len(d.Hooks["Stop"]) != 2 {
		t.Fatalf("second launch: %+v", d)
	}

	// An unreadable user config: Squad's part alone, as before.
	_ = os.WriteFile(user, []byte(`{ nope`), 0o600)
	_ = os.Remove(l.DevinConfig)
	prepare()
	if d = read(); len(d.Hooks["Stop"]) != 1 || d.Agent.Model != "" {
		t.Fatalf("unreadable user config: %+v", d)
	}
}

func TestAddDevinMCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devin", "mcp_config.json")
	if changed, err := AddDevinMCP(path, "/opt/squad"); err != nil || !changed {
		t.Fatalf("new file: %v %v", changed, err)
	}
	if changed, err := AddDevinMCP(path, "/opt/squad"); err != nil || changed {
		t.Fatalf("again: %v %v", changed, err)
	}
	_ = os.WriteFile(path, []byte("{\n // mine\n \"mcpServers\": {\"github\": {\"command\": \"gh-mcp\"}},\n}"), 0o600)
	if changed, err := AddDevinMCP(path, `C:\Users\n\go\bin\squad.exe`); err != nil || !changed {
		t.Fatalf("existing file: %v %v", changed, err)
	}
	var d struct {
		MCPServers map[string]struct {
			Command string
			Args    []string
		} `json:"mcpServers"`
	}
	data, _ := os.ReadFile(path)
	if json.Unmarshal(data, &d) != nil || d.MCPServers["github"].Command != "gh-mcp" ||
		d.MCPServers["squad"].Command != `C:\Users\n\go\bin\squad.exe` || strings.Join(d.MCPServers["squad"].Args, " ") != "mcp" {
		t.Fatalf("merged: %s", data)
	}
	if b, err := os.ReadFile(path + ".squad-backup"); err != nil || !strings.Contains(string(b), "// mine") {
		t.Fatalf("backup: %v %s", err, b)
	}
	_ = os.WriteFile(path, []byte("{ broken"), 0o600)
	if _, err := AddDevinMCP(path, "/opt/squad"); err == nil {
		t.Fatal("a broken file was overwritten")
	}
}

//go:build e2e

package agents

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// TestRealDevinConfig runs the real Devin CLI (no login needed) on the
// config Squad gives it: Devin must read the copy of the user's config
// (it writes its schema version into it) and list the squad MCP server
// registered by squad install-skill next to the user's own.
//
//	go test -tags e2e -run TestRealDevinConfig ./internal/agents/
func TestRealDevinConfig(t *testing.T) {
	devin, err := exec.LookPath("devin")
	if err != nil {
		t.Skip("devin not on PATH")
	}
	home := t.TempDir()
	cfg := filepath.Join(home, ".config")
	t.Setenv("SQUAD_HOME", filepath.Join(home, ".squad"))
	t.Setenv("XDG_CONFIG_HOME", cfg)
	user := filepath.Join(cfg, "devin", "config.json")
	_ = os.MkdirAll(filepath.Dir(user), 0o700)
	_ = os.WriteFile(user, []byte("{\n  // mine\n  \"permissions\": {\"allow\": [\"Exec(git status)\"]},\n}\n"), 0o600)
	_ = os.WriteFile(filepath.Join(cfg, "devin", "mcp_config.json"), []byte(`{"mcpServers": {"mine": {"command": "true"}}}`), 0o600)
	if _, err := AddDevinMCP(filepath.Join(cfg, "devin", "mcp_config.json"), "/opt/squad/bin/squad"); err != nil {
		t.Fatal(err)
	}

	l, err := NewLauncher("/opt/squad/bin/squad")
	if err != nil {
		t.Fatal(err)
	}
	spec := terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
	if err := l.Prepare(store.Node{}, &spec); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(devin, "--config", l.DevinConfig, "mcp", "list")
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Dir = home
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "squad") || !strings.Contains(string(out), "mine") {
		t.Fatalf("devin mcp list: %v\n%s", err, out)
	}
	var doc map[string]any
	data, _ := os.ReadFile(l.DevinConfig)
	if json.Unmarshal(data, &doc) != nil || doc["version"] == nil || doc["hooks"] == nil {
		t.Fatalf("Devin didn't read Squad's config copy:\n%s", data)
	}
}

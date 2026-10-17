//go:build !windows

package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// The user's command for an agent starts its terminals and is what Detect
// looks for; "" goes back to the agent's name.
func TestDefaultCommandAndDetect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TORRE_HOME", home)
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '\\033[1m9.9.9 (Claude Code)\\033[0m\\nmore\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "claude-beta"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	commands.Lock()
	commands.loaded = false // read the test's home
	commands.Unlock()

	if c := DefaultCommand("claude"); c != "claude" {
		t.Fatalf("default: %q", c)
	}
	if err := SetDefaultCommand("claude", "claude-beta --model haiku"); err != nil {
		t.Fatal(err)
	}
	if err := SetDefaultCommand("claude", "a\nb"); err != ErrBadCommand {
		t.Fatalf("two lines: %v", err)
	}
	if err := SetDefaultCommand("shell", "bash"); err == nil {
		t.Fatal("a shell's command set")
	}
	commands.Lock()
	commands.loaded = false // as a new server would: from agents.json
	commands.Unlock()
	if c := DefaultCommand("claude"); c != "claude-beta --model haiku" {
		t.Fatalf("saved: %q", c)
	}

	l := &Launcher{CLIPath: "/x/torre", ClaudeSettings: "/s.json", ClaudeMCP: "/m.json", RoleDir: t.TempDir()}
	n := &store.Node{ID: "N", Terminal: &store.TerminalData{Agent: "claude"}}
	spec := &terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	if _, err := l.Prepare(n, spec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(spec.Command, "claude-beta --model haiku --settings ") {
		t.Fatalf("launch line: %s", spec.Command)
	}

	d := Detect(true)
	var claude, devin Detected
	for _, a := range d.Agents {
		switch a.Agent {
		case "claude":
			claude = a
		case "devin":
			devin = a
		}
	}
	if !claude.Found || claude.Version != "9.9.9 (Claude Code)" || !claude.Custom || claude.Path != filepath.Join(bin, "claude-beta") {
		t.Fatalf("claude: %+v", claude)
	}
	if devin.Found || devin.InstallHint == "" || devin.DefaultCommand != "devin" {
		t.Fatalf("devin: %+v", devin)
	}

	if err := SetDefaultCommand("claude", ""); err != nil {
		t.Fatal(err)
	}
	if c := DefaultCommand("claude"); c != "claude" {
		t.Fatalf("reset: %q", c)
	}
}

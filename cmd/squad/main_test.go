package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/agentbus"
)

func TestRunDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "squad ") {
		t.Fatalf("version: code=%d out=%q", code, out.String())
	}
	out.Reset()
	if code := run(nil, &out, &errb); code != 0 || !strings.Contains(out.String(), "Usage: squad") {
		t.Fatalf("help: code=%d out=%q", code, out.String())
	}
	errb.Reset()
	if code := run([]string{"bogus"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("unknown: code=%d err=%q", code, errb.String())
	}
}

// Every command the help inside a terminal shows reaches the server.
func TestTerminalCommandsAreRouted(t *testing.T) {
	known := map[string]bool{}
	for _, c := range commands() {
		known[c.name] = true
	}
	for _, line := range strings.Split(agentbus.Usage, "\n") {
		f := strings.Fields(line)
		if len(f) > 1 && f[0] == "squad" && !known[f[1]] {
			t.Errorf("squad %s is in the terminal help but not a command of the squad binary", f[1])
		}
	}
	for _, args := range [][]string{{"ask", "A", "m"}, {"wait", "A"}} {
		if !blocking(args) {
			t.Errorf("%v should wait as long as the call takes", args)
		}
	}
	if blocking([]string{"ask", "A", "--raw", "x"}) || blocking([]string{"check", "A"}) {
		t.Error("raw keys and check don't block")
	}
}

func TestInstallSkill(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	if code := run([]string{"install-skill", "-dir", dir}, &out, &errb); code != 0 {
		t.Fatalf("install-skill: %d %s", code, errb.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "squad", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "name: squad") || !strings.Contains(string(data), "squad reply") {
		t.Fatalf("skill file: %v\n%s", err, data)
	}
}

func TestInstallSkillDevin(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	var out, errb bytes.Buffer
	if code := run([]string{"install-skill", "-agent", "devin"}, &out, &errb); code != 0 {
		t.Fatalf("install-skill devin: %d %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(cfg, "devin", "skills", "squad", "SKILL.md")); err != nil {
		t.Fatalf("devin skill: %v (%s)", err, out.String())
	}
	// Devin reads MCP servers only from its mcp_config.json.
	if data, err := os.ReadFile(filepath.Join(cfg, "devin", "mcp_config.json")); err != nil ||
		!strings.Contains(string(data), `"squad"`) || !strings.Contains(string(data), `"mcp"`) {
		t.Fatalf("devin mcp config: %v %s", err, data)
	}
	if code := run([]string{"install-skill", "-agent", "gemini"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "unknown agent") {
		t.Fatalf("unknown agent: %d %q", code, errb.String())
	}
}

func TestClientOutsideTerminal(t *testing.T) {
	t.Setenv("SQUAD_TERMINAL_ID", "")
	var out, errb bytes.Buffer
	for _, cmd := range [][]string{{"list"}, {"note", "list"}, {"portal", "snapshot"}} {
		errb.Reset()
		if code := run(cmd, &out, &errb); code != 1 || !strings.Contains(errb.String(), "only works inside a Squad terminal") {
			t.Fatalf("%s outside: %d %q", cmd[0], code, errb.String())
		}
	}
	// Hooks never fail loudly.
	if code := run([]string{"hook", "claude-stop"}, &out, &errb); code != 0 {
		t.Fatalf("hook outside: %d", code)
	}
}

// Without -agent, the skill goes to every agent found on the machine.
func TestInstallSkillFindsTheAgents(t *testing.T) {
	home, cfg := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	t.Setenv("PATH", t.TempDir()) // neither claude nor devin
	var out, errb bytes.Buffer
	if code := run([]string{"install-skill"}, &out, &errb); code != 0 {
		t.Fatalf("install-skill: %d %s", code, errb.String())
	}
	claude := filepath.Join(home, ".claude", "skills", "squad", "SKILL.md")
	devin := filepath.Join(cfg, "devin", "skills", "squad", "SKILL.md")
	if _, err := os.Stat(claude); err != nil {
		t.Fatalf("no agent found: Claude Code's skill expected: %v", err)
	}
	if _, err := os.Stat(devin); err == nil || !strings.Contains(out.String(), "squad install-skill -agent devin") {
		t.Fatalf("Devin's skill installed without Devin, or no hint:\n%s", out.String())
	}
	_ = os.MkdirAll(filepath.Join(cfg, "devin"), 0o700) // Devin is set up here
	out.Reset()
	if code := run([]string{"install-skill"}, &out, &errb); code != 0 {
		t.Fatalf("install-skill: %d %s", code, errb.String())
	}
	if _, err := os.Stat(devin); err != nil || !strings.Contains(out.String(), "Claude Code") || !strings.Contains(out.String(), "MCP") ||
		strings.Contains(out.String(), "não encontrado") || !strings.HasPrefix(out.String(), "squad ") {
		t.Fatalf("both agents: %v\n%s", err, out.String())
	}
	if code := run([]string{"install-skill", "-agent", "all", "-dir", t.TempDir()}, &out, &errb); code != 2 {
		t.Fatalf("-dir with two agents: %d", code)
	}
}

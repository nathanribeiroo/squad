package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

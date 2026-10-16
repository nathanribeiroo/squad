//go:build e2e && !windows

// End-to-end test with the real Claude Code CLI. It costs one tiny prompt.
// Run: go test -tags e2e -run TestRealClaude -v ./internal/app/
// SQUAD_E2E_CWD must be a folder Claude Code already trusts (default: repo root).

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/store"
)

func TestRealClaude(t *testing.T) {
	r, screen := askRealClaude(t, "", "Responda apenas com a palavra: pong")
	if r.Code != 200 || r.Source != "claude-stop" || !strings.Contains(strings.ToLower(r.Body), "pong") {
		t.Fatalf("unexpected reply: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
}

// The role's instructions reach Claude Code (--append-system-prompt-file).
func TestRealClaudeRole(t *testing.T) {
	r, screen := askRealClaude(t, "Responda sempre só com letras MAIÚSCULAS.", "Responda apenas com a palavra: pong")
	if r.Code != 200 || r.Source != "claude-stop" || !strings.Contains(r.Body, "PONG") {
		t.Fatalf("the role was not followed: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
}

// askRealClaude starts a Claude Code terminal (with a role, when prompt isn't
// empty) and asks it msg from a shell.
func askRealClaude(t *testing.T, rolePrompt, msg string) (agentbus.Result, string) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}
	cwd := os.Getenv("SQUAD_E2E_CWD")
	if cwd == "" {
		cwd, _ = filepath.Abs("../..")
	}
	limits := agentbus.Limits{Timeout: 3 * time.Minute}
	e := newEnv(t, limits)
	role := ""
	if rolePrompt != "" {
		r, err := e.roles.Create(roles.Role{Name: "Gritador", Prompt: rolePrompt})
		if err != nil {
			t.Fatal(err)
		}
		role = r.ID
	}
	st := e.app.Server.Store()
	shell, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Shell", Agent: "shell", Shell: "/bin/sh", Cwd: cwd}})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Claude", Agent: "claude", Role: role, Cwd: cwd, Cols: 120, Rows: 36}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(shell, claude)
	for _, n := range []store.Node{shell, claude} {
		if err := e.app.Server.StartTerminal(n.ID); err != nil {
			t.Fatal(err)
		}
	}
	term := e.app.Server.Terminals().Get(claude.ID)
	defer func() { _ = term.Raw([]byte("/exit\r")) }()

	// Wait for Claude Code's prompt, past its first-run notes on a new machine
	// and its question about trusting the folder (the repository itself).
	deadline := time.Now().Add(60 * time.Second)
	for screen := term.Snapshot(); !strings.Contains(screen, "❯") || strings.Contains(screen, "Enter to confirm"); screen = term.Snapshot() {
		if time.Now().After(deadline) {
			t.Fatalf("claude did not start:\n%s", screen)
		}
		switch {
		case strings.Contains(screen, "Press Enter to continue"):
			_ = term.Raw([]byte("\r"))
			time.Sleep(time.Second)
		case strings.Contains(screen, "❯ Yes, I trust this folder"):
			_ = term.Raw([]byte("\r"))
			time.Sleep(time.Second)
		case strings.Contains(screen, "Yes, I trust this folder"):
			_ = term.Raw([]byte("\x1b[B")) // down to "Yes"
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)

	start := time.Now()
	r := e.run(shell, "ask", "Claude", msg)
	t.Logf("ask took %v, source=%s\n%s", time.Since(start), r.Source, r.Body)
	return r, term.Snapshot()
}

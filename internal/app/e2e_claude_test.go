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
	"github.com/nathanribeiroo/squad/internal/store"
)

func TestRealClaude(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}
	cwd := os.Getenv("SQUAD_E2E_CWD")
	if cwd == "" {
		cwd, _ = filepath.Abs("../..")
	}
	limits := agentbus.Limits{Timeout: 3 * time.Minute}
	e := newEnv(t, limits)
	st := e.app.Server.Store()
	shell, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Shell", Agent: "shell", Shell: "/bin/sh", Cwd: cwd}})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Claude", Agent: "claude", Cwd: cwd, Cols: 120, Rows: 36}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(shell, claude)
	for _, n := range []store.Node{shell, claude} {
		if err := e.app.Server.StartTerminal(n.ID); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { _ = e.app.Server.Terminals().Get(claude.ID).Raw([]byte("/exit\r")) }()

	// Wait for Claude Code's prompt.
	deadline := time.Now().Add(60 * time.Second)
	for !strings.Contains(e.app.Server.Terminals().Get(claude.ID).Snapshot(), "❯") {
		if time.Now().After(deadline) {
			t.Fatalf("claude did not start:\n%s", e.app.Server.Terminals().Get(claude.ID).Snapshot())
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)

	start := time.Now()
	r := e.run(shell, "ask", "Claude", "Responda apenas com a palavra: pong")
	t.Logf("ask took %v, source=%s\n%s", time.Since(start), r.Source, r.Body)
	if r.Code != 200 || r.Source != "claude-stop" || !strings.Contains(strings.ToLower(r.Body), "pong") {
		t.Fatalf("unexpected reply: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body,
			e.app.Server.Terminals().Get(claude.ID).Snapshot())
	}
}

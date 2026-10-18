//go:build e2e && !windows

// End-to-end test with the real Claude Code CLI. It costs one tiny prompt.
// Run: go test -tags e2e -run TestRealClaude -v ./internal/app/
// TORRE_E2E_CWD must be a folder Claude Code already trusts (default: repo root).

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/radio"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/store"
)

func TestRealClaude(t *testing.T) {
	r, screen := askRealClaude(t, "", "Responda apenas com a palavra: pong", nil)
	if r.Code != 200 || r.Source != "claude-stop" || !strings.Contains(strings.ToLower(r.Body), "pong") {
		t.Fatalf("unexpected reply: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
}

// The Rádio keeps a real Claude's reply with the request, after the turn.
func TestRealClaudeRadio(t *testing.T) {
	e, shell, _ := startRealClaude(t, "")
	r := e.run(shell, "ask", "Claude", "Responda apenas com a palavra: pong")
	if r.Code != 200 {
		t.Fatalf("ask: %+v", r)
	}
	line := e.radioLine(func(it radio.Item) bool { return it.Outcome == "replied" && it.HasReply })
	if _, reply, _ := e.app.Radio.Texts(line.ID); !strings.Contains(strings.ToLower(reply), "pong") || strings.HasPrefix(reply, "[reply from") {
		t.Fatalf("reply kept: %q", reply)
	}
}

// The role's instructions reach Claude Code (--append-system-prompt-file).
func TestRealClaudeRole(t *testing.T) {
	r, screen := askRealClaude(t, "Responda sempre só com letras MAIÚSCULAS.", "Responda apenas com a palavra: pong", nil)
	if r.Code != 200 || r.Source != "claude-stop" || !strings.Contains(r.Body, "PONG") {
		t.Fatalf("the role was not followed: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
}

// PRIORIDADE with the real CLI: in the default permission mode, a command
// stops at Claude Code's prompt; the flight asks the user (the hook gives the
// command, the screen the answers), and answering from the interface runs it.
func TestRealClaudePermission(t *testing.T) {
	file := filepath.Join(t.TempDir(), "allowed.txt")
	var answered bool
	r, screen := askRealClaude(t, "", "Use the Bash tool to run exactly this command, nothing else: touch "+file, func(e *env, id string) {
		a := e.waitAttention(id, func(a *attention.Attention) bool { return a != nil && len(a.Choices) > 0 }, 2*time.Minute)
		if a.Tool != "Bash" || !strings.Contains(a.Detail, file) || a.Choices[0] != attention.Allow {
			t.Errorf("attention: %+v", a)
		}
		if code, body := e.respond(id, "allow"); code != 204 {
			t.Errorf("respond: %d %s", code, body)
		}
		answered = true
	}, "--permission-mode", "default")
	if !answered || r.Code != 200 || r.Source != "claude-stop" {
		t.Fatalf("reply: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the command didn't run: %v\n%s", err, screen)
	}
}

// A Claude wired only to the base of a stack of notes reads another note of
// the stack, by its own name.
func TestRealClaudeStack(t *testing.T) {
	e, shell, claude := startRealClaude(t, "")
	plano, dec := e.notam("Plano", 0), e.notam("Decisoes", 500)
	if err := e.app.Notes.Write(dec, "Codigo da decisao: TORRE-7341", "test"); err != nil {
		t.Fatal(err)
	}
	if w := e.call("POST", "/api/notes/"+dec.ID+"/stack", map[string]string{"onto": plano.ID}); w.Code != 200 {
		t.Fatalf("stack: %d %s", w.Code, w.Body)
	}
	e.connect(claude, plano)
	r := e.run(shell, "ask", "Claude", `Leia a NOTAM "Decisoes" e responda apenas com o codigo que esta nela.`)
	t.Logf("source=%s\n%s", r.Source, r.Body)
	if r.Code != 200 || r.Source != "claude-stop" || !strings.Contains(r.Body, "TORRE-7341") {
		t.Fatalf("the stack wasn't read: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, e.app.Server.Terminals().Get(claude.ID).Snapshot())
	}
}

// askRealClaude starts a Claude Code terminal (with a role, when prompt isn't
// empty, and extra arguments) and asks it msg from a shell; during, when
// set, runs while the ask waits.
func askRealClaude(t *testing.T, rolePrompt, msg string, during func(e *env, id string), args ...string) (agentbus.Result, string) {
	e, shell, claude := startRealClaude(t, rolePrompt, args...)
	term := e.app.Server.Terminals().Get(claude.ID)
	start := time.Now()
	if during != nil {
		done := make(chan agentbus.Result, 1)
		go func() { done <- e.run(shell, "ask", "Claude", msg) }()
		during(e, claude.ID)
		r := <-done
		t.Logf("ask took %v, source=%s\n%s", time.Since(start), r.Source, r.Body)
		return r, term.Snapshot()
	}
	r := e.run(shell, "ask", "Claude", msg)
	t.Logf("ask took %v, source=%s\n%s", time.Since(start), r.Source, r.Body)
	return r, term.Snapshot()
}

// startRealClaude starts a shell and a Claude Code terminal wired to it (with
// a role, when prompt isn't empty, and extra arguments), and waits for Claude
// Code's prompt.
func startRealClaude(t *testing.T, rolePrompt string, args ...string) (e *env, shell, claude store.Node) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}
	cwd := config.Getenv("E2E_CWD")
	if cwd == "" {
		cwd, _ = filepath.Abs("../..")
	}
	limits := agentbus.Limits{Timeout: 3 * time.Minute}
	e = newEnv(t, limits)
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
	command := ""
	if len(args) > 0 {
		command = "claude " + strings.Join(args, " ")
	}
	claude, err = st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Claude", Agent: "claude", Command: command, Role: role, Cwd: cwd, Cols: 120, Rows: 36}})
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
	t.Cleanup(func() { _ = term.Raw([]byte("/exit\r")) }) // before the env closes

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
	return e, shell, claude
}

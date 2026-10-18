//go:build e2e && !windows

// End-to-end tests with the real Devin CLI, logged in. They cost five tiny
// prompts. Run:
//
//	TORRE_E2E_DEVIN_MODEL=swe-1-6-slow go test -tags e2e -run TestRealDevin -v ./internal/app/
//
// Without devin on the PATH, or without a login, it is skipped.

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/store"
)

// PRIORIDADE with the real Devin: a command stops at its permission prompt;
// the hook says what is at stake, the screen gives the answers, and the
// interface answers allow, deny and always (this command in this project,
// saved in the project's .devin/config.local.json).
func TestRealDevinPermission(t *testing.T) {
	e, shell, devin, dir := startRealDevin(t)
	ask := func(file string, choice attention.Choice) (agentbus.Result, *attention.Attention) {
		done := make(chan agentbus.Result, 1)
		go func() {
			done <- e.run(shell, "ask", "Devin", "Run exactly this shell command and nothing else, then stop: touch "+file)
		}()
		a := e.waitAttention(devin.ID, func(a *attention.Attention) bool { return a != nil && len(a.Choices) > 0 }, 2*time.Minute)
		if code, body := e.respond(devin.ID, string(choice)); code != 204 {
			t.Fatalf("respond %s: %d %s", choice, code, body)
		}
		r := <-done
		t.Logf("%s: %+v\n%s", choice, a, r.Body)
		return r, a
	}

	_, a := ask("allowed.txt", attention.Allow)
	if a.Tool != "exec" || !strings.Contains(a.Detail, "touch allowed.txt") || a.Choices[0] != attention.Allow {
		t.Errorf("attention: %+v", a)
	}
	if _, err := os.Stat(filepath.Join(dir, "allowed.txt")); err != nil {
		t.Fatalf("allow: the command didn't run: %v\n%s", err, e.app.Server.Terminals().Get(devin.ID).Snapshot())
	}

	ask("denied.txt", attention.Deny)
	if _, err := os.Stat(filepath.Join(dir, "denied.txt")); err == nil {
		t.Fatal("deny: the command ran")
	}

	_, a = ask("always.txt", attention.Always)
	if !strings.Contains(a.Labels[attention.Always], "commands in") {
		t.Errorf("always label: %v", a.Labels)
	}
	if _, err := os.Stat(filepath.Join(dir, "always.txt")); err != nil {
		t.Fatalf("always: the command didn't run: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".devin", "config.local.json")); !strings.Contains(string(b), "Exec(touch)") {
		t.Fatalf("always: not saved in the project:\n%s", b)
	}
}

// The real Devin's question (ask_user_question, read on screen) is answered
// from the interface with the user's text, on its "Other" field.
func TestRealDevinQuestion(t *testing.T) {
	e, shell, devin, _ := startRealDevin(t)
	done := make(chan agentbus.Result, 1)
	go func() {
		done <- e.run(shell, "ask", "Devin", "Use your ask_user_question tool to ask me exactly one question: 'Qual cor?' with the options Azul and Verde. Then reply with only my answer.")
	}()
	a := e.waitAttention(devin.ID, func(a *attention.Attention) bool { return a != nil && len(a.Options) > 0 }, 2*time.Minute)
	field := slices.IndexFunc(a.Options, func(o attention.Option) bool { return o.Input })
	if a.Kind != attention.Question || a.Text != "Qual cor?" || field < 0 {
		t.Fatalf("attention: %+v", a)
	}
	if code, body := e.respondOption(devin.ID, a.Options[field].Key, "Roxo"); code != 204 {
		t.Fatalf("respond: %d %s", code, body)
	}
	if r := <-done; r.Code != 200 || !strings.Contains(r.Body, "Roxo") {
		t.Fatalf("reply: %+v\n%s", r, e.app.Server.Terminals().Get(devin.ID).Snapshot())
	}
}

// The real Devin in plan mode (alt+p): its plan comes whole in the hook,
// and the interface approves it (accepting edits); the file gets written.
func TestRealDevinPlan(t *testing.T) {
	e, shell, devin, dir := startRealDevin(t)
	term := e.app.Server.Terminals().Get(devin.ID)
	_ = term.Raw([]byte("\x1bp"))
	waitUntil(t, "plan mode", func() bool { return strings.Contains(term.Snapshot(), "Plan ·") })
	done := make(chan agentbus.Result, 1)
	go func() {
		done <- e.run(shell, "ask", "Devin", "Plan how to create the file hello.txt containing the word oi in this folder. Keep the plan to two short steps.")
	}()
	a := e.waitAttention(devin.ID, func(a *attention.Attention) bool { return a != nil && a.Kind == attention.Plan && len(a.Options) > 0 }, 3*time.Minute)
	accept := slices.IndexFunc(a.Options, func(o attention.Option) bool { return strings.Contains(o.Text, "accept edits") })
	if !strings.Contains(a.PlanText, "hello.txt") || accept < 0 || slices.ContainsFunc(a.Options, func(o attention.Option) bool { return strings.Contains(o.Text, "bypass") }) {
		t.Fatalf("plan: %+v", a)
	}
	if code, body := e.respondOption(devin.ID, a.Options[accept].Key, ""); code != 204 {
		t.Fatalf("approve: %d %s", code, body)
	}
	if r := <-done; r.Code != 200 {
		t.Fatalf("reply: %+v", r)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "hello.txt")); err != nil || !strings.Contains(string(b), "oi") {
		t.Fatalf("the file: %q %v\n%s", b, err, term.Snapshot())
	}
}

// startRealDevin starts a shell and a Devin CLI terminal wired to it, in a
// new git repository it trusts, and waits for Devin's prompt.
func startRealDevin(t *testing.T) (e *env, shell, devin store.Node, dir string) {
	if _, err := exec.LookPath("devin"); err != nil {
		t.Skip("devin CLI not installed")
	}
	if out, _ := exec.Command("devin", "auth", "status").CombinedOutput(); !strings.Contains(string(out), "Logged in") {
		t.Skip("devin CLI not logged in")
	}
	dir = t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	e = newEnv(t, agentbus.Limits{Timeout: 3 * time.Minute})
	command := ""
	if m := config.Getenv("E2E_DEVIN_MODEL"); m != "" {
		command = "devin --model " + m
	}
	st := e.app.Server.Store()
	shell, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Shell", Agent: "shell", Shell: "/bin/sh", Cwd: dir}})
	if err != nil {
		t.Fatal(err)
	}
	devin, err = st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Devin", Agent: "devin", Command: command, Cwd: dir, Cols: 120, Rows: 40}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(shell, devin)
	for _, n := range []store.Node{shell, devin} {
		if err := e.app.Server.StartTerminal(n.ID); err != nil {
			t.Fatal(err)
		}
	}
	term := e.app.Server.Terminals().Get(devin.ID)
	t.Cleanup(func() { _ = term.Raw([]byte("/exit\r")) }) // before the env closes

	// Past the question about trusting the folder, up to Devin's prompt.
	deadline := time.Now().Add(90 * time.Second)
	for screen := term.Snapshot(); !strings.Contains(screen, "Ask Devin"); screen = term.Snapshot() {
		if time.Now().After(deadline) {
			t.Fatalf("devin did not start:\n%s", screen)
		}
		if strings.Contains(screen, "Yes, trust") {
			_ = term.Raw([]byte("1"))
			time.Sleep(time.Second)
		}
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)
	return e, shell, devin, dir
}

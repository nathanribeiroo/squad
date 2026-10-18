//go:build e2e && !windows

// End-to-end test with the real Claude Code CLI. It costs one tiny prompt.
// Run: go test -tags e2e -run TestRealClaude -v ./internal/app/
// TORRE_E2E_CWD must be a folder Claude Code already trusts (default: repo root).

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

// A real Claude's question (AskUserQuestion) is answered from the interface
// with the user's own text, on its text field.
func TestRealClaudeQuestion(t *testing.T) {
	var answered bool
	r, screen := askRealClaude(t, "", "Use the AskUserQuestion tool to ask me exactly one question: 'Qual cor?' (header 'Cor'), options 'Azul' and 'Verde', single select. Then reply with only my answer.", func(e *env, id string) {
		a := e.waitAttention(id, func(a *attention.Attention) bool { return a != nil && len(a.Options) > 0 }, 2*time.Minute)
		field := slices.IndexFunc(a.Options, func(o attention.Option) bool { return o.Input })
		if a.Kind != attention.Question || a.Text != "Qual cor?" || a.Options[0].Text != "Azul" || field < 0 {
			t.Errorf("attention: %+v", a)
			return
		}
		if code, body := e.respondOption(id, a.Options[field].Key, "Roxo"); code != 204 {
			t.Errorf("respond: %d %s", code, body)
		}
		answered = true
	})
	if !answered || r.Code != 200 || !strings.Contains(r.Body, "Roxo") {
		t.Fatalf("reply: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
}

// A real Claude in plan mode: its plan comes whole in the hook, an option
// approves it from the interface, and the edit it then asks for is allowed
// from there too.
func TestRealClaudePlan(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plano.txt")
	var approved, allowed bool
	r, screen := askRealClaude(t, "", "Plan how to create the file "+file+" containing the word oi. Keep the plan to two short steps and ask me to approve it.", func(e *env, id string) {
		a := e.waitAttention(id, func(a *attention.Attention) bool { return a != nil && a.Kind == attention.Plan && len(a.Options) > 0 }, 3*time.Minute)
		manual := slices.IndexFunc(a.Options, func(o attention.Option) bool { return strings.Contains(o.Text, "manually approve") })
		if !strings.Contains(a.PlanText, "plano.txt") || a.Detail == "" || manual < 0 {
			t.Errorf("plan: %+v", a)
			return
		}
		if code, body := e.respondOption(id, a.Options[manual].Key, ""); code != 204 {
			t.Errorf("approve: %d %s", code, body)
			return
		}
		approved = true
		// Then the edits it asks for (and a check it may run), until it rests.
		var last, quiet time.Time
		for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
			a := e.app.Monitor.Attention(id)
			if a == nil || a.Kind != attention.Permission || len(a.Choices) == 0 || a.Since.Equal(last) {
				if _, err := os.Stat(file); err == nil && allowed && a == nil {
					if quiet.IsZero() {
						quiet = time.Now()
					} else if time.Since(quiet) > 5*time.Second {
						return
					}
				}
				continue
			}
			quiet, last = time.Time{}, a.Since
			if code, body := e.respond(id, "allow"); code != 204 {
				t.Errorf("allow %s: %d %s", a.Tool, code, body)
				return
			}
			allowed = true
		}
	}, "--permission-mode", "plan")
	if !approved || !allowed || r.Code != 200 {
		t.Fatalf("reply: code=%d source=%s\n%s\n--- screen ---\n%s", r.Code, r.Source, r.Body, screen)
	}
	if b, err := os.ReadFile(file); err != nil || !strings.Contains(string(b), "oi") {
		t.Fatalf("the file: %q %v\n%s", b, err, screen)
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

// A Claude on a floor runs the floor's tests and marks the result for the
// landing queue (the floors tool needs no approval; the user allows the
// test command from the interface).
func TestRealClaudeFloorMark(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}
	e := newEnv(t, agentbus.Limits{Timeout: 3 * time.Minute})
	repo := e.gitRepo()
	if err := os.WriteFile(filepath.Join(repo, "test.sh"), []byte("echo '3 testes passaram'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.git(repo, "add", "test.sh")
	e.git(repo, "commit", "-q", "-m", "tests")
	code, f := e.api("POST", "/api/floors", `{"name":"Login"}`)
	if code != 201 {
		t.Fatalf("floor: %d %v", code, f)
	}
	id := f["id"].(string)
	st := e.app.Server.Store()
	shell, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Shell", Agent: "shell", Shell: "/bin/sh"}})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := st.AddNode(store.Node{Kind: store.KindTerminal, Floor: id,
		Terminal: &store.TerminalData{Name: "Testador", Agent: "claude", Command: "claude --permission-mode default", Cols: 120, Rows: 36}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(shell, claude)
	for _, n := range []store.Node{shell, claude} {
		if err := e.app.Server.StartTerminal(n.ID); err != nil {
			t.Fatal(err)
		}
	}
	waitRealClaude(t, e, claude.ID)
	done := e.askAsync(shell, "Testador", "Rode `sh test.sh` (são os testes deste nível) e registre o resultado com a ferramenta floors do MCP torre (action mark, check tests). Depois responda só OK.")
	// Allow what it asks for (the test command), as the user would.
	var r agentbus.Result
	for answered := 0; ; {
		select {
		case r = <-done:
		case <-time.After(200 * time.Millisecond):
			if a := e.app.Monitor.Attention(claude.ID); a != nil && slices.Contains(a.Choices, attention.Allow) && answered < 3 {
				t.Logf("allowing %s %q", a.Tool, a.Detail)
				if code, body := e.respond(claude.ID, "allow"); code != 204 {
					t.Fatalf("respond: %d %s", code, body)
				}
				answered++
				time.Sleep(time.Second)
			}
			continue
		}
		break
	}
	fl, _ := st.Floor(id)
	c, ok := fl.Checks[store.CheckTests]
	head := e.git(fl.Path, "rev-parse", "HEAD")
	t.Logf("reply: %s · check: %+v", r.Source, c)
	if r.Code != 200 || !ok || c.Result != store.CheckPass || c.By != claude.ID || c.Commit != head {
		t.Fatalf("check: %+v %v\nreply: %+v\n%s", c, ok, r, e.app.Server.Terminals().Get(claude.ID).Snapshot())
	}
}

// Two flights ask the same Claude, the second while the first's turn runs:
// the second waits in Claude's line, and each gets the answer to its own
// message (before, the second got the first's).
func TestRealClaudeQueue(t *testing.T) {
	e, shell, claude := startRealClaude(t, "")
	other, err := e.app.Server.Store().AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Outro", Agent: "shell", Shell: "/bin/sh"}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(other, claude)
	if err := e.app.Server.StartTerminal(other.ID); err != nil {
		t.Fatal(err)
	}
	l := e.requestLog()
	first := e.askAsync(shell, "Claude", "Escreva os numeros de 1 a 120 por extenso, um por linha, e na ultima linha escreva PRIMEIRO-OK.")
	e.waitWorking(claude.ID)
	time.Sleep(3 * time.Second) // well into the first turn
	second := e.run(other, "ask", "Claude", "Responda apenas com a palavra: SEGUNDO-OK")
	r := <-first
	t.Logf("first: %s · second: %s", r.Source, second.Source)
	if r.Code != 200 || !strings.Contains(r.Body, "PRIMEIRO-OK") || strings.Contains(r.Body, "SEGUNDO-OK") {
		t.Fatalf("first: %+v", r)
	}
	if second.Code != 200 || second.Source != "claude-stop" || !strings.Contains(second.Body, "SEGUNDO-OK") || strings.Contains(second.Body, "PRIMEIRO-OK") {
		t.Fatalf("second: %+v\n%s", second, e.app.Server.Terminals().Get(claude.ID).Snapshot())
	}
	if queued, _ := l.snapshot(); queued[l.sent(other)] != 1 {
		t.Fatalf("queued: %v", queued)
	}
}

// A Claude on a floor gets the ground floor's permissions (when the floor is
// made and as it starts), "always" from the interface works there (Claude's
// "don’t ask again", with its typographic apostrophe) and what it saves is
// the ground floor's at once; the floor's copy of the file blocks nothing.
func TestRealClaudeFloorPermissions(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not installed")
	}
	e := newEnv(t, agentbus.Limits{Timeout: 3 * time.Minute})
	repo := e.gitRepo()
	const file = ".claude/settings.local.json"
	write := func(dir, text string) {
		path := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(dir string) string {
		b, _ := os.ReadFile(filepath.Join(dir, file))
		return string(b)
	}
	write(repo, `{"permissions": {"allow": ["Bash(npm test *)"]}}`)
	code, f := e.api("POST", "/api/floors", `{"name":"Regras"}`)
	if code != 201 {
		t.Fatalf("floor: %d %v", code, f)
	}
	id, cwd := f["id"].(string), f["cwd"].(string)
	write(repo, `{"permissions": {"allow": ["Bash(npm test *)", "Bash(ls *)"]}}`)

	st := e.app.Server.Store()
	shell, err := st.AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Shell", Agent: "shell", Shell: "/bin/sh"}})
	if err != nil {
		t.Fatal(err)
	}
	claude, err := st.AddNode(store.Node{Kind: store.KindTerminal, Floor: id,
		Terminal: &store.TerminalData{Name: "Claude", Agent: "claude", Command: "claude --permission-mode default", Cols: 120, Rows: 36}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(shell, claude)
	for _, n := range []store.Node{shell, claude} {
		if err := e.app.Server.StartTerminal(n.ID); err != nil {
			t.Fatal(err)
		}
	}
	waitRealClaude(t, e, claude.ID)
	if got := read(cwd); !strings.Contains(got, "Bash(npm test *)") || !strings.Contains(got, "Bash(ls *)") {
		t.Fatalf("the floor's file: %s", got)
	}

	done := make(chan agentbus.Result, 1)
	go func() {
		done <- e.run(shell, "ask", "Claude", "Use the Bash tool to run exactly this command, nothing else: git tag teste-torre")
	}()
	a := e.waitAttention(claude.ID, func(a *attention.Attention) bool { return a != nil && slices.Contains(a.Choices, attention.Always) }, 2*time.Minute)
	t.Logf("always: %q", a.Labels[attention.Always])
	if code, body := e.respond(claude.ID, "always"); code != 204 {
		t.Fatalf("respond: %d %s", code, body)
	}
	screen := e.app.Server.Terminals().Get(claude.ID).Snapshot
	if r := <-done; r.Code != 200 || r.Source != "claude-stop" {
		t.Fatalf("reply: %+v\n%s", r, screen())
	}
	// Claude Code (2.1.302) saves it in the main repository's file even in a
	// worktree, and reads both: it is already the ground floor's.
	got := read(repo)
	t.Logf("ground floor's file:\n%s", got)
	if !strings.Contains(got, "Bash(git tag *)") || !strings.Contains(got, "Bash(ls *)") || !strings.Contains(got, "Bash(npm test *)") {
		t.Fatalf("ground floor's file: %s\nfloor's: %s\n%s", got, read(cwd), screen())
	}

	// So landing has nothing of Claude's to take, and the floor's copy of
	// the file doesn't block it.
	code, ch := e.api("GET", "/api/floors/"+id+"/changes", "")
	if rules, _ := ch["rules"].([]any); code != 200 || len(ch["files"].([]any)) != 0 || len(rules) != 0 {
		t.Fatalf("changes: %d %v", code, ch)
	}
	if code, res := e.api("POST", "/api/floors/"+id+"/land", `{"permissions":true}`); code != 200 || res["permissions"] != nil {
		t.Fatalf("land: %d %v", code, res)
	}
	if got2 := read(repo); got2 != got {
		t.Fatalf("ground floor's file changed on landing: %s", got2)
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
	waitRealClaude(t, e, claude.ID)
	return e, shell, claude
}

// waitRealClaude waits for Claude Code's prompt on terminal id, past its
// first-run notes on a new machine and its question about trusting the
// folder.
func waitRealClaude(t *testing.T, e *env, id string) {
	t.Helper()
	term := e.app.Server.Terminals().Get(id)
	t.Cleanup(func() { _ = term.Raw([]byte("/exit\r")) }) // before the env closes
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
}

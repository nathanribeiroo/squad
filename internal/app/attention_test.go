//go:build !windows

package app

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/store"
)

// respond answers a flight's prompt the way the interface does.
func (e *env) respond(id, choice string) (int, string) {
	b, _ := json.Marshal(map[string]string{"choice": choice})
	r := httptest.NewRequest("POST", "/api/nodes/"+id+"/attention/respond", bytes.NewReader(b))
	r.Host = strings.TrimPrefix(e.app.Server.URL(), "http://")
	r.Header.Set("Authorization", "Bearer tok")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.app.Server.Handler().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// waitAttention waits until ok(attention) holds for terminal id.
func (e *env) waitAttention(id string, ok func(*attention.Attention) bool, timeout time.Duration) *attention.Attention {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if a := e.app.Monitor.Attention(id); ok(a) {
			return a
		}
		time.Sleep(50 * time.Millisecond)
	}
	st, _ := e.app.Monitor.State(id)
	e.t.Fatalf("attention of %s: %+v (state %s)\n%s", id, e.app.Monitor.Attention(id), st, e.app.Server.Terminals().Get(id).Snapshot())
	return nil
}

// A Claude at a permission prompt asks the user (PRIORIDADE): the hook says
// what is at stake, the prompt on screen gives the answers, the interface
// answers with the key, and the flight goes back to work. A message for it
// meanwhile waits: typed into the prompt, it would answer it.
func TestPermissionAskAndRespond(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	other := e.agent("Outro")
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	e.connect(plan, c)
	e.connect(other, c)

	first := make(chan agentbus.Result, 1)
	go func() { first <- e.run(plan, "ask", "Claude", "ASKPERM:make test") }()
	a := e.waitAttention(c.ID, func(a *attention.Attention) bool { return a != nil && len(a.Choices) == 3 }, 10*time.Second)
	if a.Kind != attention.Permission || a.Tool != "Bash" || a.Detail != "make test" || a.Source != "hook" || a.Text != "Do you want to proceed?" {
		t.Fatalf("attention: %+v", a)
	}
	if !reflect.DeepEqual(a.Choices, []attention.Choice{attention.Allow, attention.Always, attention.Deny}) {
		t.Fatalf("choices: %v", a.Choices)
	}
	if st, _ := e.app.Monitor.State(c.ID); st != activity.Ask {
		t.Fatalf("state: %s", st)
	}
	// Still asking after the quiet that would end a turn: no turn end.
	time.Sleep(time.Second)
	select {
	case r := <-first:
		t.Fatalf("the ask ended while the flight waited for the user: %+v", r)
	default:
	}

	// Another request waits for the answer, out of the prompt.
	second := make(chan agentbus.Result, 1)
	go func() { second <- e.run(other, "ask", "Claude", "oi depois") }()
	time.Sleep(800 * time.Millisecond)
	if log := e.log("Claude"); strings.Contains(log, `SUBMIT "[de: Outro`) || strings.Contains(log, "ASK-KEY 1b5b3230307e") {
		t.Fatalf("message typed into the prompt:\n%s", log)
	}

	// Garbage and a shell are refused; a prompt that changed too.
	if code, body := e.respond(c.ID, "maybe"); code != 400 {
		t.Fatalf("bad choice: %d %s", code, body)
	}
	if code, body := e.respond(c.ID, "always"); code != 204 {
		t.Fatalf("respond: %d %s", code, body)
	}
	for deadline := time.Now().Add(3 * time.Second); !strings.Contains(e.log("Claude"), "ASK-CHOSE 2"); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("key not typed:\n%s", e.log("Claude"))
		}
	}
	e.waitAttention(c.ID, func(a *attention.Attention) bool { return a == nil }, 5*time.Second)
	if code, body := e.respond(c.ID, "allow"); code != 409 || !strings.Contains(body, `"code":"attention.none"`) {
		t.Fatalf("nothing to answer: %d %s", code, body)
	}
	if r := <-first; r.Code != 200 || !strings.Contains(r.Body, "final: ASKPERM:make test") && !strings.Contains(r.Body, "final: [de: Plano") {
		t.Fatalf("first ask: %+v", r)
	}
	if r := <-second; r.Code != 200 || !strings.Contains(e.log("Claude"), `SUBMIT "[de: Outro`) {
		t.Fatalf("second ask: %+v\n%s", r, e.log("Claude"))
	}
}

// Agents whose keys aren't verified get the state from their screen, and
// only "open the terminal"; answering in the terminal clears it. Shells and
// own commands are never read.
func TestAskFromScreenOnly(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	codex := e.agentOfType("Codex", "codex")
	own := e.agent("Script") // agent "custom": an own command
	e.connect(plan, codex)
	e.connect(plan, own)
	go e.run(plan, "ask", "Codex", "ASKPERM:rm -rf build")
	a := e.waitAttention(codex.ID, func(a *attention.Attention) bool { return a != nil }, 10*time.Second)
	if a.Source != "screen" || a.Kind != attention.Permission || a.Text != "Do you want to proceed?" || len(a.Choices) != 0 {
		t.Fatalf("attention: %+v", a)
	}
	if code, body := e.respond(codex.ID, "allow"); code != 400 || !strings.Contains(body, "attention.no_keys") {
		t.Fatalf("unverified keys: %d %s", code, body)
	}
	if err := e.app.Server.Terminals().Get(codex.ID).Input([]byte("4")); err != nil {
		t.Fatal(err)
	}
	e.waitAttention(codex.ID, func(a *attention.Attention) bool { return a == nil }, 5*time.Second)

	go e.run(plan, "ask", "Script", "ASKPERM:make deploy")
	e.waitScreen(own.ID, "Do you want to proceed?", 10*time.Second)
	time.Sleep(2500 * time.Millisecond)
	if a := e.app.Monitor.Attention(own.ID); a != nil {
		t.Fatalf("own command read as asking: %+v", a)
	}
	if code, body := e.respond(own.ID, "allow"); code != 400 || !strings.Contains(body, "attention.not_agent") {
		t.Fatalf("own command answered: %d %s", code, body)
	}
	_ = e.app.Server.Terminals().Get(own.ID).Input([]byte("4"))
}

// The process going away ends the ask; so does a hook's turn end.
func TestAskClearsWhenProcessExits(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	e.connect(plan, c)
	go e.run(plan, "ask", "Claude", "ASKPERM:make test")
	e.waitAttention(c.ID, func(a *attention.Attention) bool { return a != nil }, 10*time.Second)
	if n, ok := e.app.Server.Store().Node(c.ID); !ok || n.Kind != store.KindTerminal {
		t.Fatal("node")
	}
	if err := e.app.Server.Terminals().Get(c.ID).Input([]byte{0x03}); err != nil { // Ctrl-C: exit 130
		t.Fatal(err)
	}
	e.waitAttention(c.ID, func(a *attention.Attention) bool { return a == nil }, 5*time.Second)
	if st, _ := e.app.Monitor.State(c.ID); st == activity.Ask {
		t.Fatalf("state: %s", st)
	}
}

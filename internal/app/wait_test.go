//go:build !windows

package app

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
)

// waitEnv times out an ask after 2 s without activity, and ends a hooked
// agent's "working" after 0.5 s of silence (as at a permission prompt).
func waitEnv(t *testing.T) *env {
	limits := fastLimits
	limits.Timeout, limits.ActiveWindow = 2*time.Second, 500*time.Millisecond
	act := fastActivity
	act.HookTimeout = 500 * time.Millisecond
	return newEnvWith(t, limits, act)
}

// An ask that times out while the agent is still in its turn says so;
// squad wait then returns the agent's final answer when it finishes, and
// its last answer once it's done.
func TestWaitAfterTimeout(t *testing.T) {
	e := waitEnv(t)
	a := e.agent("A")
	arq := e.agentOfType("Arq", "claude", "FAKEAGENT_HOOK=claude")
	e.connect(a, arq)

	r := e.run(a, "ask", "Arq", "SLEEP:3")
	if r.Source != "timeout" || !strings.Contains(r.Body, `squad wait "Arq"`) {
		t.Fatalf("ask: source=%s body=%s", r.Source, r.Body)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		r = e.run(a, "wait", "Arq")
	}()
	// The waiting terminal shows as waiting on the canvas.
	deadline := time.Now().Add(time.Second)
	for st, _ := e.app.Monitor.State(a.ID); st != activity.Wait; st, _ = e.app.Monitor.State(a.ID) {
		if time.Now().After(deadline) {
			t.Fatalf("state while waiting: %s", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
	<-done
	if r.Code != 200 || r.Source != "claude-stop" || !strings.HasPrefix(r.Body, "[final answer from Arq · end of its turn]\nfinal: [de: A · pedido ") {
		t.Fatalf("wait: code=%d source=%s body=%s", r.Code, r.Source, r.Body)
	}
	if st, _ := e.app.Monitor.State(a.ID); st == activity.Wait {
		t.Fatal("still waiting after the wait")
	}

	r = e.run(a, "wait", "Arq")
	if r.Source != "last" || !strings.Contains(r.Body, "[last answer from Arq · its turn ended ") ||
		!strings.Contains(r.Body, "it began with: [de: A · pedido ") || !strings.Contains(r.Body, "\nfinal: ") {
		t.Fatalf("wait after the turn: source=%s body=%s", r.Source, r.Body)
	}
}

// A message the awaited agent sends the waiting one ends the wait (it
// couldn't read it otherwise); the reply goes back, and the wait resumes.
func TestWaitReceivesMessages(t *testing.T) {
	e := waitEnv(t)
	a := e.agentOfType("A", "claude", "FAKEAGENT_HOOK=claude")
	b := e.agentOfType("B", "claude", "FAKEAGENT_HOOK=claude")
	e.connect(a, b)

	if r := e.run(a, "ask", "B", "RUN:sleep 3; squad ask A pergunta-de-b"); r.Source != "timeout" {
		t.Fatalf("ask: source=%s body=%s", r.Source, r.Body)
	}
	r := e.run(a, "wait", "B")
	m := regexp.MustCompile(`^\[de: B · pedido ([0-9a-f]+)\] pergunta-de-b\n\[squad: "B" sent this while you were waiting for its answer`).FindStringSubmatch(r.Body)
	if r.Source != "message" || m == nil {
		t.Fatalf("wait: source=%s body=%s", r.Source, r.Body)
	}
	if strings.Contains(e.app.Server.Terminals().Get(a.ID).Snapshot(), "pergunta-de-b") {
		t.Fatal("the message was also typed into A")
	}
	if r := e.run(a, "reply", m[1], "resposta-de-a"); r.Code != 200 {
		t.Fatalf("reply: %d %s", r.Code, r.Body)
	}
	e.waitScreen(b.ID, "resposta-de-a", 5*time.Second)
	if r := e.run(a, "wait", "B"); !strings.Contains(r.Body, "final: [de: A · pedido ") {
		t.Fatalf("wait again: source=%s body=%s", r.Source, r.Body)
	}
}

// Waiting for someone who waits for you would never end: it's refused.
func TestWaitRefusesCycle(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	c := e.agent("C")
	e.connect(a, b)
	r := e.run(a, "ask", "B", "RUN:squad wait A")
	if !strings.Contains(r.Body, `"A" is waiting for you (its request `) {
		t.Fatalf("cycle: %s", r.Body)
	}
	for args, want := range map[string]string{"wait C": "not connected", "wait": "Usage: squad wait", "wait --x": "Usage: squad wait"} {
		if r := e.run(a, strings.Fields(args)...); r.Code < 400 || !strings.Contains(r.Body, want) {
			t.Fatalf("%s: %d %s", args, r.Code, r.Body)
		}
	}
	_ = c
}

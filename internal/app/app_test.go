//go:build !windows

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

var binDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "squad-app-test")
	if err != nil {
		panic(err)
	}
	binDir = dir
	for name, pkg := range map[string]string{
		"squad":     "github.com/nathanribeiroo/squad/cmd/squad",
		"fakeagent": "github.com/nathanribeiroo/squad/internal/testutil/fakeagent",
	} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, name), pkg)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			panic("build " + name + ": " + err.Error())
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fast timings so tests take seconds, not minutes.
var fastActivity = activity.Options{Tick: 250 * time.Millisecond, ByteThresh: 200, ActiveStreak: 2, IdleAfter: 600 * time.Millisecond}
var fastLimits = agentbus.Limits{QuickAfter: time.Second, QuickEvery: 200 * time.Millisecond, Stable: time.Second,
	Timeout: 15 * time.Second, ActiveWindow: 2 * time.Second, RawDelay: 200 * time.Millisecond}

type env struct {
	t   *testing.T
	app *App
	dir string
}

func newEnv(t *testing.T, limits agentbus.Limits) *env {
	t.Helper()
	ln, port, err := server.ListenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "workspace.json"), store.Workspace{ID: "w", Name: "test", Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{
		Server:   server.Options{Port: port, Token: "tok", Store: st, CLIPath: filepath.Join(binDir, "squad")},
		Activity: fastActivity, Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewUnstartedServer(a.Server.Handler())
	hs.Listener = ln
	hs.Start()
	a.Monitor.Start()
	t.Cleanup(func() { hs.Close(); a.Close() })
	return &env{t: t, app: a, dir: dir}
}

// agent adds a fakeagent terminal; extra are KEY=VALUE settings.
func (e *env) agent(name string, extra ...string) store.Node {
	e.t.Helper()
	log := filepath.Join(e.dir, name+".log")
	vars := append([]string{"FAKEAGENT_LOG=" + log, "FAKEAGENT_WORK=0.4"}, extra...)
	cmd := strings.Join(vars, " ") + " " + filepath.Join(binDir, "fakeagent")
	n, err := e.app.Server.Store().AddNode(store.Node{Kind: store.KindTerminal,
		Terminal: &store.TerminalData{Name: name, Agent: "custom", Command: cmd, Shell: "/bin/sh"}})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.app.Server.StartTerminal(n.ID); err != nil {
		e.t.Fatal(err)
	}
	e.waitScreen(n.ID, "fakeagent ready", 10*time.Second)
	return n
}

func (e *env) connect(a, b store.Node) store.Edge {
	e.t.Helper()
	edge, err := e.app.Server.Store().AddEdge(a.ID, b.ID, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return edge
}

func (e *env) waitScreen(id, text string, timeout time.Duration) {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if t := e.app.Server.Terminals().Get(id); t != nil && strings.Contains(t.Snapshot(), text) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t := e.app.Server.Terminals().Get(id)
	e.t.Fatalf("timeout waiting for %q on screen:\n%s", text, t.Snapshot())
}

func (e *env) log(name string) string {
	b, _ := os.ReadFile(filepath.Join(e.dir, name+".log"))
	return string(b)
}

func (e *env) run(caller store.Node, args ...string) agentbus.Result {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return e.app.Run(ctx, caller.ID, args...)
}

func TestAskRepliesAtTurnEnd(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("Planejador")
	b := e.agent("Implementador", "FAKEAGENT_WORK=1.2", "FAKEAGENT_WIDTH=1500")
	e.connect(a, b)
	start := time.Now()
	r := e.run(a, "ask", "Implementador", "implemente o CSV")
	if r.Code != 200 || r.Source != "idle" {
		t.Fatalf("ask: code=%d source=%s body=%s", r.Code, r.Source, r.Body)
	}
	if !strings.Contains(r.Body, "[screen of Implementador after its turn · request ") || !strings.Contains(r.Body, "answer: [DE: PLANEJADOR · PEDIDO") {
		t.Fatalf("body:\n%s", r.Body)
	}
	if !strings.Contains(e.log("Implementador"), `SUBMIT "[de: Planejador · pedido `) {
		t.Fatalf("envelope not delivered:\n%s", e.log("Implementador"))
	}
	if el := time.Since(start); el > 6*time.Second {
		t.Fatalf("too slow: %v", el)
	}
}

func TestAskFallsBackToSettledScreen(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B") // little output: never "working", reply by settled screen
	e.connect(a, b)
	r := e.run(a, "ask", "B", "oi")
	if r.Code != 200 || r.Source != "screen" || !strings.Contains(r.Body, "DONE") {
		t.Fatalf("ask: code=%d source=%s body=%s", r.Code, r.Source, r.Body)
	}
}

func TestAskErrors(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	c := e.agent("C")
	e.connect(a, b)
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"ask", "C", "x"}, 403},
		{[]string{"ask", "Nobody", "x"}, 404},
		{[]string{"ask", "A", "x"}, 400},
		{[]string{"ask", "B"}, 400},
		{[]string{"check", "C"}, 403},
		{[]string{"reply", "ffff", "x"}, 404},
		{[]string{"frobnicate"}, 400},
	}
	for _, c := range cases {
		if r := e.run(a, c.args...); r.Code != c.code {
			t.Errorf("%v: code %d (want %d): %s", c.args, r.Code, c.code, r.Body)
		}
	}
	e.app.Server.Terminals().Get(b.ID).Stop()
	if r := e.run(a, "ask", "B", "x"); r.Code != 503 {
		t.Errorf("stopped target: %d %s", r.Code, r.Body)
	}
	_ = c
	if r := e.run(a, "list"); r.Code != 200 || !strings.Contains(r.Body, `name: "B"`) || strings.Contains(r.Body, `name: "C"`) {
		t.Errorf("list:\n%s", r.Body)
	}
}

func TestAskBackIsDirectReply(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	r := e.run(a, "ask", "B", "RUN:squad ask A 'resultado final'")
	if r.Code != 200 || r.Source != "ask-back" || !strings.Contains(r.Body, "resultado final") || !strings.Contains(r.Body, "[reply from B · request ") {
		t.Fatalf("ask-back: code=%d source=%s body=%s", r.Code, r.Source, r.Body)
	}
	e.waitScreen(b.ID, "OK: delivered to \"A\" as the reply to request", 5*time.Second)
}

func TestReplyByID(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	r := e.run(a, "ask", "B", "RUN:squad reply {id} feito-via-reply")
	if r.Code != 200 || r.Source != "reply" || !strings.Contains(r.Body, "feito-via-reply") {
		t.Fatalf("reply: code=%d source=%s body=%s", r.Code, r.Source, r.Body)
	}
}

func TestCycleIsReleased(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	c := e.agent("C")
	e.connect(a, b)
	e.connect(b, c)
	e.connect(c, a)
	// A asks B; B asks C; C asks A while A waits on B → A's wait is released.
	r := e.run(a, "ask", "B", "RUN:squad ask C 'RUN:squad ask A ping-de-c'")
	if r.Code != 200 || r.Source != "ring-break" || !strings.Contains(r.Body, "deadlock") {
		t.Fatalf("cycle: code=%d source=%s body=%s", r.Code, r.Source, r.Body)
	}
	// C's message reaches A as a normal prompt and A answers it.
	e.waitScreen(a.ID, "received: [de: C · pedido", 10*time.Second)
}

func TestRawCheckAndBatch(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	c := e.agent("C")
	e.connect(a, b)
	e.connect(a, c)
	r := e.run(a, "ask", "B", "--raw", `abc\x03`)
	if r.Code != 200 || !strings.Contains(r.Body, "^C") {
		t.Fatalf("raw: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "check", "B"); r.Code != 200 || !strings.Contains(r.Body, "^C") {
		t.Fatalf("check: %d %s", r.Code, r.Body)
	}
	r = e.run(a, "ask", "--batch", `{"C": "um", "B": "dois"}`)
	var items []agentbus.BatchItem
	if err := json.Unmarshal([]byte(r.Body), &items); err != nil || len(items) != 2 {
		t.Fatalf("batch: %v %s", err, r.Body)
	}
	// B exited on ^C (its shell is back), C answers normally.
	if items[0].Name != "B" || items[1].Name != "C" || !strings.Contains(items[1].Output, "answer: [DE: A") {
		t.Fatalf("batch items: %+v", items)
	}
}

func TestHopLimitPausesConversations(t *testing.T) {
	limits := fastLimits
	limits.MaxHops = 1
	e := newEnv(t, limits)
	a := e.agent("A")
	b := e.agent("B")
	c := e.agent("C")
	e.connect(a, b)
	e.connect(b, c)
	r := e.run(a, "ask", "B", "RUN:squad ask C hi")
	if r.Code != 200 || !strings.Contains(r.Body, "hop limit (1) reached") {
		t.Fatalf("hop: code=%d body=%s", r.Code, r.Body)
	}
	if !e.app.Bus.Paused() {
		t.Fatal("conversations should be paused")
	}
}

func TestPauseHoldsUntilResume(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	e.app.Bus.SetPaused(true, "test")
	done := make(chan agentbus.Result, 1)
	go func() { done <- e.run(a, "ask", "B", "segura") }()
	time.Sleep(700 * time.Millisecond)
	if len(e.app.Bus.Pending()) != 0 || strings.Contains(e.log("B"), "segura") {
		t.Fatal("request should be held while paused")
	}
	e.app.Bus.SetPaused(false, "test")
	select {
	case r := <-done:
		if r.Code != 200 || !strings.Contains(r.Body, "SEGURA") {
			t.Fatalf("after resume: %d %s", r.Code, r.Body)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("not delivered after resume")
	}
}

func TestEdgeRemovalCancelsPending(t *testing.T) {
	e := newEnv(t, fastLimits)
	e.app.Bus.SetHookedAgents(func(string) bool { return true }) // no screen fallback
	a := e.agent("A")
	b := e.agent("B")
	edge := e.connect(a, b)
	done := make(chan agentbus.Result, 1)
	go func() { done <- e.run(a, "ask", "B", "SLEEP:30") }()
	deadline := time.Now().Add(5 * time.Second)
	for len(e.app.Bus.Pending()) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !e.app.Bus.IsWaiting(a.ID) {
		t.Fatal("A should be waiting")
	}
	if st, _ := e.app.Monitor.State(a.ID); st != activity.Wait {
		t.Fatalf("A state %s, want wait", st)
	}
	if _, err := e.app.Server.Store().RemoveEdge(edge.ID); err != nil {
		t.Fatal(err)
	}
	e.app.Bus.EdgeRemoved(edge)
	select {
	case r := <-done:
		if r.Code != 409 || r.Source != "cancelled" {
			t.Fatalf("cancel: %d %s %s", r.Code, r.Source, r.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("not cancelled")
	}
}

// The real CLI binary talks to the server from inside a terminal.
func TestCLIBinaryFromTerminal(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	r := e.run(a, "ask", "B", "RUN:squad list")
	if r.Code != 200 || !strings.Contains(r.Body, `name: "B"`) {
		t.Fatalf("cli list from B: %d %s", r.Code, r.Body)
	}
	_ = fmt.Sprint
	_ = http.StatusOK
}

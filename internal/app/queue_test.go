//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/radio"
	"github.com/nathanribeiroo/squad/internal/store"
)

// requestLog records the requests' events: who sent each one, and the order
// in which they were queued and delivered.
type requestLog struct {
	mu        sync.Mutex
	from      map[string]string // request id → sender id
	queued    map[string]int    // request id → requests ahead of it
	delivered []string
}

func (e *env) requestLog() *requestLog {
	l := &requestLog{from: map[string]string{}, queued: map[string]int{}}
	ch, cancel := e.app.Server.Bus().Subscribe()
	e.t.Cleanup(cancel)
	go func() {
		for ev := range ch {
			d, _ := ev.Data.(map[string]any)
			id, _ := d["id"].(string)
			l.mu.Lock()
			switch ev.Type {
			case events.RequestSent:
				l.from[id], _ = d["from"].(string)
			case events.RequestQueued:
				l.queued[id], _ = d["ahead"].(int)
			case events.RequestDelivered:
				l.delivered = append(l.delivered, id)
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// sent returns the id of the request n sent.
func (l *requestLog) sent(n store.Node) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, from := range l.from {
		if from == n.ID {
			return id
		}
	}
	return ""
}

func (l *requestLog) snapshot() (map[string]int, []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	q := map[string]int{}
	for k, v := range l.queued {
		q[k] = v
	}
	return q, slices.Clone(l.delivered)
}

// waitWorking waits until terminal id is in a turn.
func (e *env) waitWorking(id string) {
	e.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !e.app.Monitor.Working(id); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			e.t.Fatalf("not working:\n%s", e.app.Server.Terminals().Get(id).Snapshot())
		}
	}
}

// askAsync asks from caller in the background.
func (e *env) askAsync(caller store.Node, target, msg string) <-chan agentbus.Result {
	ch := make(chan agentbus.Result, 1)
	go func() { ch <- e.run(caller, "ask", target, msg) }()
	return ch
}

// Three flights ask the same busy flight: each request waits in its line
// for the turn before it to end, and gets the answer to its own message,
// in the order they were sent. The Rádio says it is in the line meanwhile.
func TestBusyFlightQueue(t *testing.T) {
	for _, agent := range []string{"claude", "devin"} {
		t.Run(agent, func(t *testing.T) { busyFlightQueue(t, agent) })
	}
}

func busyFlightQueue(t *testing.T, agent string) {
	e := newEnv(t, fastLimits)
	plano, revisor, testador := e.agent("Plano"), e.agent("Revisor"), e.agent("Testador")
	impl := e.agentOfType("Impl", agent, "FAKEAGENT_HOOK="+agent, "FAKEAGENT_WORK=1.5")
	for _, n := range []store.Node{plano, revisor, testador} {
		e.connect(n, impl)
	}
	l := e.requestLog()

	um := e.askAsync(plano, "Impl", "um")
	e.waitWorking(impl.ID)
	dois := e.askAsync(revisor, "Impl", "dois")
	time.Sleep(300 * time.Millisecond)
	tres := e.askAsync(testador, "Impl", "tres")
	line := e.radioLine(func(it radio.Item) bool { return it.FromName == "Revisor" && it.Queued })

	for _, c := range []struct {
		from store.Node
		ch   <-chan agentbus.Result
		word string
	}{{plano, um, "um"}, {revisor, dois, "dois"}, {testador, tres, "tres"}} {
		r := <-c.ch
		id := l.sent(c.from)
		if r.Code != 200 || r.Source != agent+"-stop" || !strings.Contains(r.Body, "pedido "+id+"] "+c.word) {
			t.Fatalf("%s's answer (request %s): %+v", c.word, id, r)
		}
	}
	queued, delivered := l.snapshot()
	um1, dois1, tres1 := l.sent(plano), l.sent(revisor), l.sent(testador)
	if _, ok := queued[um1]; ok || queued[dois1] != 1 || queued[tres1] != 2 {
		t.Fatalf("queued: %v (%s %s %s)", queued, um1, dois1, tres1)
	}
	if !slices.Equal(delivered, []string{um1, dois1, tres1}) {
		t.Fatalf("delivered: %v, want %s %s %s", delivered, um1, dois1, tres1)
	}
	if line.ID != dois1 {
		t.Fatalf("Rádio line: %+v", line)
	}
	if it := e.radioLine(func(it radio.Item) bool { return it.ID == dois1 && it.Outcome == "replied" }); it.Queued {
		t.Fatalf("still queued: %+v", it)
	}
}

// A busy flight that asks (or waits for) someone whose request waits in its
// line gets that request instead, answers it, and nothing waits forever.
func TestQueueHandOver(t *testing.T) {
	for _, how := range []string{"ask", "wait"} {
		t.Run(how, func(t *testing.T) {
			e := newEnv(t, fastLimits)
			plano, revisor := e.agent("Plano"), e.agent("Revisor")
			impl := e.agentOfType("Impl", "claude", "FAKEAGENT_HOOK=claude")
			e.connect(plano, impl)
			e.connect(revisor, impl)
			l := e.requestLog()
			handed := filepath.Join(e.dir, "handed.txt")
			cmd := "torre ask Revisor 'pergunta do impl'"
			if how == "wait" {
				cmd = "torre wait Revisor"
			}
			first := e.askAsync(plano, "Impl", "RUN:sleep 1.5; out=$("+cmd+"); echo \"$out\" > "+handed+
				"; torre reply $(echo \"$out\" | grep -o 'torre reply [0-9a-f]*' | head -1 | cut -d' ' -f3) pronto-revisor")
			e.waitWorking(impl.ID)
			second := e.askAsync(revisor, "Impl", "pergunta do revisor")

			r := <-second
			if r.Code != 200 || r.Source != "reply" || !strings.Contains(r.Body, "pronto-revisor") {
				t.Fatalf("Revisor's answer: %+v", r)
			}
			if r := <-first; r.Code != 200 || r.Source != "claude-stop" {
				t.Fatalf("Plano's answer: %+v", r)
			}
			b, _ := os.ReadFile(handed)
			if !strings.Contains(string(b), "pergunta do revisor") || !strings.Contains(string(b), "sent this while you were busy") {
				t.Fatalf("handed over:\n%s", b)
			}
			if strings.Contains(e.log("Impl"), "received: [de: Revisor") {
				t.Fatalf("pasted too:\n%s", e.log("Impl"))
			}
			if queued, _ := l.snapshot(); len(queued) != 1 {
				t.Fatalf("queued: %v", queued)
			}
		})
	}
}

// A request that waits in the line past its limit, or that the user
// cancels there, never reaches the flight.
func TestQueueLimitAndCancel(t *testing.T) {
	limits := fastLimits
	limits.MaxWait = 2 * time.Second
	e := newEnv(t, limits)
	plano, revisor, testador := e.agent("Plano"), e.agent("Revisor"), e.agent("Testador")
	impl := e.agentOfType("Impl", "claude", "FAKEAGENT_HOOK=claude", "FAKEAGENT_WORK=3.5")
	for _, n := range []store.Node{plano, revisor, testador} {
		e.connect(n, impl)
	}
	l := e.requestLog()
	um := e.askAsync(plano, "Impl", "um")
	e.waitWorking(impl.ID)
	dois := e.askAsync(revisor, "Impl", "dois")
	tres := e.askAsync(testador, "Impl", "tres")
	for deadline := time.Now().Add(2 * time.Second); l.sent(testador) == ""; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no request from Testador")
		}
	}
	if !e.app.Bus.Cancel(l.sent(testador)) {
		t.Fatal("cancel")
	}
	if r := <-tres; r.Code != 409 || r.Source != "cancelled" {
		t.Fatalf("cancelled: %+v", r)
	}
	if r := <-dois; r.Source != "timeout" || !strings.Contains(r.Body, `"Impl" was busy the whole time`) || !strings.Contains(r.Body, "not delivered") {
		t.Fatalf("limit: %+v", r)
	}
	if r := <-um; r.Source != "claude-stop" {
		t.Fatalf("first: %+v", r)
	}
	time.Sleep(3 * time.Second) // past the settle after the first turn
	if log := e.log("Impl"); strings.Contains(log, "dois") || strings.Contains(log, "tres") {
		t.Fatalf("delivered anyway:\n%s", log)
	}
}

// A flight without hooks (its turns end when its screen settles) has a
// line too.
func TestQueueScreenOnly(t *testing.T) {
	e := newEnv(t, fastLimits)
	plano, revisor := e.agent("Plano"), e.agent("Revisor")
	gem := e.agentOfType("Gem", "gemini", "FAKEAGENT_WORK=1.5", "FAKEAGENT_WIDTH=400")
	e.connect(plano, gem)
	e.connect(revisor, gem)
	l := e.requestLog()
	um := e.askAsync(plano, "Gem", "um")
	e.waitWorking(gem.ID)
	dois := e.askAsync(revisor, "Gem", "dois")
	r1, r2 := <-um, <-dois
	if !strings.Contains(r1.Body, "answer: [DE: PLANO") {
		t.Fatalf("first: %+v", r1)
	}
	if !strings.Contains(r2.Body, "answer: [DE: REVISOR") {
		t.Fatalf("second: %+v", r2)
	}
	if queued, delivered := l.snapshot(); queued[l.sent(revisor)] != 1 || !slices.Equal(delivered, []string{l.sent(plano), l.sent(revisor)}) {
		t.Fatalf("queued %v, delivered %v", queued, delivered)
	}
}

// A shell has no line: a message goes at once, even while it prints.
func TestShellNotQueued(t *testing.T) {
	e := newEnv(t, fastLimits)
	plano := e.agent("Plano")
	sh, err := e.app.Server.Store().AddNode(store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Sh", Agent: "shell", Shell: "/bin/sh"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.app.Server.StartTerminal(sh.ID); err != nil {
		t.Fatal(err)
	}
	e.connect(plano, sh)
	l := e.requestLog()
	term := e.app.Server.Terminals().Get(sh.ID)
	time.Sleep(500 * time.Millisecond)
	_ = term.Raw([]byte("for i in $(seq 40); do echo line $i $(printf '%0300d' 0); sleep 0.1; done\r"))
	e.waitWorking(sh.ID)
	e.askAsync(plano, "Sh", "echo oi")
	for deadline := time.Now().Add(time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, delivered := l.snapshot(); len(delivered) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not delivered at once")
		}
	}
	if queued, _ := l.snapshot(); len(queued) != 0 {
		t.Fatalf("queued: %v", queued)
	}
}

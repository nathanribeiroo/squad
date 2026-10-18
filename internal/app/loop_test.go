//go:build !windows

package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/radio"
	"github.com/nathanribeiroo/squad/internal/store"
)

// A chain of requests past the hop limit holds every conversation and says
// which flights made the loop: in the event, in the state a reload reads, in
// the Rádio and to the flight that asked. Releasing clears it; the user's own
// Espera geral has no chain.
func TestLoopChain(t *testing.T) {
	limits := fastLimits
	limits.MaxHops = 2
	e := newEnv(t, limits)
	wide := "FAKEAGENT_WIDTH=400"
	plan, impl, rev, tst := e.agent("Plano", wide), e.agent("Impl", wide), e.agent("Revisor", wide), e.agent("Testador", wide)
	for i, n := range []store.Node{plan, impl, rev, tst} {
		if _, err := e.app.Server.Store().AssignCallsign(n.ID, []string{"PLN", "IMP", "REV", "TST"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	e.connect(plan, impl)
	e.connect(impl, rev)
	e.connect(rev, tst)
	evs, cancel := e.app.Server.Bus().Subscribe()
	defer cancel()
	pauses := make(chan map[string]any, 8)
	go func() {
		for ev := range evs {
			if ev.Type == events.ConversationsPaused {
				pauses <- ev.Data.(map[string]any)
			}
		}
	}()
	next := func() map[string]any {
		t.Helper()
		select {
		case d := <-pauses:
			return d
		case <-time.After(10 * time.Second):
			t.Fatal("no conversations.paused event")
			return nil
		}
	}

	// Plano → Impl → Revisor are 2 hops; Revisor asking Testador is the 3rd.
	r := e.run(plan, "ask", "Impl", `RUN:torre ask Revisor "RUN:torre ask Testador oi"`)
	if r.Code != 200 {
		t.Fatalf("ask: %d %s", r.Code, r.Body)
	}
	want := []string{plan.ID, impl.ID, rev.ID, tst.ID}
	d := next()
	p, _ := d["pause"].(agentbus.Pause)
	ids := func(c []agentbus.LoopStop) []string {
		var out []string
		for _, s := range c {
			out = append(out, s.ID)
		}
		return out
	}
	if d["paused"] != true || p.By != "loop" || !slices.Equal(ids(p.Chain), want) || p.Hops != 3 || p.Limit != 2 ||
		p.Chain[1].Callsign != "IMP" || p.Chain[1].Name != "Impl" {
		t.Fatalf("event: %+v", d)
	}
	if d["reason"] != "hop limit 2 reached PLN → IMP → REV → TST" {
		t.Fatalf("reason: %q", d["reason"])
	}
	// The flight that asked is told the chain, itself as "you".
	if !strings.Contains(e.log("Revisor"), "torre: hop limit (2) reached: PLN → IMP → you → TST. Conversations are paused until the user releases them") {
		t.Fatalf("asker's answer:\n%s", e.log("Revisor"))
	}
	if strings.Contains(e.log("Testador"), "oi") {
		t.Fatal("the request past the limit was delivered")
	}
	st, _ := e.app.Bus.StateExtra()["pause"].(*agentbus.Pause)
	if !e.app.Bus.Paused() || st == nil || st.By != "loop" || !slices.Equal(ids(st.Chain), want) {
		t.Fatalf("state: %+v", st)
	}
	line := e.radioLine(func(it radio.Item) bool { return it.Hold == "loop" })
	if line.Outcome != "held" || line.Text != "hop limit 2 reached PLN → IMP → REV → TST" {
		t.Fatalf("Rádio: %+v", line)
	}
	// The user's Espera geral on top of it changes nothing.
	e.app.Bus.SetPaused(true, "user")
	if st, _ := e.app.Bus.StateExtra()["pause"].(*agentbus.Pause); st == nil || st.By != "loop" {
		t.Fatalf("after Espera geral: %+v", st)
	}

	e.app.Bus.SetPaused(false, "user")
	if d := next(); d["paused"] != false || d["pause"] != nil {
		t.Fatalf("release event: %+v", d)
	}
	if st, _ := e.app.Bus.StateExtra()["pause"].(*agentbus.Pause); e.app.Bus.Paused() || st != nil {
		t.Fatalf("state after release: %+v", st)
	}

	e.app.Bus.SetPaused(true, "user")
	if d := next(); d["pause"].(agentbus.Pause).By != "user" || d["pause"].(agentbus.Pause).Chain != nil {
		t.Fatalf("user pause event: %+v", d)
	}
	if st, _ := e.app.Bus.StateExtra()["pause"].(*agentbus.Pause); st == nil || st.By != "user" || st.Chain != nil {
		t.Fatalf("user pause state: %+v", st)
	}
	items, _ := e.app.Radio.Page(time.Time{}, radio.Limit)
	loops := 0
	for _, it := range items {
		if it.Hold == "loop" {
			loops++
		}
	}
	if loops != 1 {
		t.Fatalf("Rádio loop lines: %d", loops)
	}
	e.app.Bus.SetPaused(false, "user")
}

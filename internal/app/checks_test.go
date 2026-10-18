//go:build !windows

package app

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/radio"
	"github.com/nathanribeiroo/squad/internal/store"
)

// A Testador on a floor marks its tests through the MCP tool: the Rádio
// says so, torre floor list shows it, and the manager's land reports it.
func TestFloorChecks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := newEnv(t, fastLimits)
	e.gitRepo()
	plan := e.agent("Plano")
	e.setRole(plan, "plan")
	if r := e.run(plan, "floor", "create", "Login"); r.Code != 200 {
		t.Fatalf("create: %+v", r)
	}
	if r := e.run(plan, "new", "Tst", "--floor", "Login", "--agent", "custom", "--command", e.fakeCommand("Tst")); r.Code != 200 {
		t.Fatalf("new: %+v", r)
	}
	tst, _ := e.node("Tst")
	s := e.mcpSession(tst)
	if out, failed := call(t, s, "floors", map[string]any{"action": "mark", "check": "tests", "result": "pass", "note": "12 testes passaram"}); failed || !strings.Contains(out, `Marked tests as passed on floor "Login"`) {
		t.Fatalf("mark: %v %s", failed, out)
	}
	if out, failed := call(t, s, "floors", map[string]any{"action": "mark", "check": "tests"}); !failed || !strings.Contains(out, "result") {
		t.Fatalf("no result: %v %s", failed, out)
	}
	line := e.radioLine(func(it radio.Item) bool { return it.Outcome == "check" })
	if line.Check != "tests" || line.Result != "pass" || line.FromName != "Tst" || line.ToName != "Login" || line.Number != 1 || line.Text != "12 testes passaram" {
		t.Fatalf("Rádio: %+v", line)
	}
	if r := e.run(plan, "floor", "list"); !strings.Contains(r.Body, `checks: tests passed (TST: "12 testes passaram") on the latest commit; review not marked`) {
		t.Fatalf("list:\n%s", r.Body)
	}
	r := e.run(plan, "floor", "land", "Login")
	if r.Code != 200 || !strings.Contains(r.Body, `Checks when it landed: tests passed (TST: "12 testes passaram") on the latest commit; review not marked.`) {
		t.Fatalf("land: %+v", r)
	}
}

// torre list says who is busy: the request each flight answers, how many
// wait in its line, who waits for whom, and, to a busy flight, who waits
// for its turn.
func TestListOccupancy(t *testing.T) {
	e := newEnv(t, fastLimits)
	plano, revisor, obs := e.agent("Plano"), e.agent("Revisor"), e.agent("Obs")
	impl := e.agentOfType("Impl", "claude", "FAKEAGENT_HOOK=claude", "FAKEAGENT_WORK=3")
	for _, n := range []store.Node{plano, revisor, obs} {
		e.connect(n, impl)
	}
	e.connect(obs, revisor)
	e.connect(obs, plano)
	l := e.requestLog()
	um := e.askAsync(plano, "Impl", "um")
	e.waitWorking(impl.ID)
	dois := e.askAsync(revisor, "Impl", "dois")
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if q, _ := l.snapshot(); len(q) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not queued")
		}
	}
	list := e.run(obs, "list").Body
	for _, want := range []string{
		`busy: answering request ` + l.sent(plano) + ` from "Plano"; 1 more in its line (from "Revisor"): an ask now waits for those first`,
		`waiting for "Impl" (#2 in its line)`,
		`waiting for "Impl" (request ` + l.sent(plano) + `)`,
		"States: idle/done free · work in a turn",
	} {
		if !strings.Contains(list, want) {
			t.Fatalf("missing %q in:\n%s", want, list)
		}
	}
	if own := e.run(impl, "list").Body; !strings.Contains(own, `1 request(s) wait for your turn to end (from "Revisor"); they arrive one at a time after it.`) || strings.Contains(own, l.sent(revisor)) {
		t.Fatalf("own list:\n%s", own)
	}
	// Nor can it answer one it hasn't read.
	if r := e.run(impl, "reply", l.sent(revisor), "pronto"); r.Code != 409 || !strings.Contains(r.Body, "hasn't reached you yet") {
		t.Fatalf("reply to a queued request: %+v", r)
	}
	<-um
	<-dois
	if after := e.run(obs, "list").Body; strings.Contains(after, "busy:") || strings.Contains(after, `waiting for "`) {
		t.Fatalf("after:\n%s", after)
	}
}

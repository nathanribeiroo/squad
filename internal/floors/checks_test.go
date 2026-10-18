package floors

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// A floor's agents and the one that created it mark its tests and review on
// its latest commit; a later commit makes a mark stale; others can't mark.
func TestMarkChecks(t *testing.T) {
	_, svc, st := newRepo(t)
	ctx := context.Background()
	term := func(name, callsign, floor string) store.Node {
		n, err := st.AddNode(store.Node{Kind: store.KindTerminal, Floor: floor, Terminal: &store.TerminalData{Name: name, Agent: "claude", Callsign: callsign}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	plan := term("Plano", "PLN", "")
	f, err := svc.Create(ctx, CreateRequest{Name: "Login", By: plan.ID})
	if err != nil {
		t.Fatal(err)
	}
	tst, other := term("Testador", "TST", f.ID), term("Outro", "OUT", "")
	ch, cancel := svc.o.Canvas.Bus().Subscribe()
	defer cancel()
	cli := func(n store.Node, args ...string) (int, string) {
		r, _ := svc.Extension(ctx, n.ID, append([]string{"floor"}, args...))
		return r.Code, r.Body
	}

	if code, body := cli(tst, "mark", "tests", "pass", "42 passaram"); code != 200 || !strings.Contains(body, `Marked tests as passed on floor "Login"`) || strings.Contains(body, "uncommitted") {
		t.Fatalf("mark: %d %s", code, body)
	}
	got, _ := st.Floor(f.ID)
	c := got.Checks[store.CheckTests]
	head := sh(t, f.Path, "rev-parse", "HEAD")
	if c.Result != store.CheckPass || c.Note != "42 passaram" || c.Commit != head || c.By != tst.ID || c.Callsign != "TST" {
		t.Fatalf("check: %+v", c)
	}
	for seen := map[string]bool{}; !seen[events.FloorChecked] || !seen[events.FloorsUpdated]; {
		ev := <-ch
		seen[ev.Type] = true
		if ev.Type == events.FloorChecked {
			if d := ev.Data.(map[string]any); d["check"] != "tests" || d["result"] != "pass" || d["fromName"] != "Testador" {
				t.Fatalf("event: %+v", d)
			}
		}
	}
	if code, body := cli(plan, "mark", "review", "FAIL", "falta tratar o erro", "--floor", "Login"); code != 200 {
		t.Fatalf("creator: %d %s", code, body)
	}

	// Not its floor, nor its creator; no floor; bad words.
	for _, c := range []struct {
		n    store.Node
		args []string
		code int
	}{
		{other, []string{"mark", "tests", "pass", "--floor", "Login"}, 403},
		{other, []string{"mark", "tests", "pass"}, 400},
		{tst, []string{"mark", "lint", "pass"}, 400},
		{tst, []string{"mark", "tests", "ok"}, 400},
		{tst, []string{"mark", "tests"}, 400},
	} {
		if code, body := cli(c.n, c.args...); code != c.code {
			t.Fatalf("%v: %d %s", c.args, code, body)
		}
	}

	_, list := cli(plan, "list")
	if !strings.Contains(list, `checks: tests passed (TST: "42 passaram") on the latest commit; review failed (PLN: "falta tratar o erro") on the latest commit`) {
		t.Fatalf("list:\n%s", list)
	}
	if s := svc.Status(ctx); s.Floors[f.ID].Head != head {
		t.Fatalf("status head: %+v", s.Floors[f.ID])
	}

	// A commit after the mark: it is stale. Uncommitted work isn't part of a mark.
	write(t, filepath.Join(f.Cwd, "login.js"), "x\n")
	sh(t, f.Path, "add", "login.js")
	sh(t, f.Path, "commit", "-q", "-m", "login")
	if _, list = cli(plan, "list"); !strings.Contains(list, "tests passed (TST: \"42 passaram\") on an older commit, "+head[:7]+", 1 commit(s) ago: mark again") {
		t.Fatalf("stale:\n%s", list)
	}
	write(t, filepath.Join(f.Cwd, "wip.js"), "y\n")
	if code, body := cli(tst, "mark", "tests", "fail", "1 falhou"); code != 200 || !strings.Contains(body, "1 uncommitted file(s) aren't part of it") {
		t.Fatalf("dirty: %d %s", code, body)
	}
	if _, list = cli(tst, "list"); !strings.Contains(list, `tests failed (TST: "1 falhou") on the latest commit`) {
		t.Fatalf("marked again:\n%s", list)
	}
}

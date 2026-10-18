//go:build !windows

package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/plans"
	"github.com/nathanribeiroo/squad/internal/store"
)

func (e *env) notam(title string, x float64) store.Node {
	e.t.Helper()
	n, err := e.app.Server.AddNode(store.Node{Kind: store.KindNote, X: x, Y: 600, Note: &store.NoteData{Title: title}})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

// A wire to a stack reaches every note in it, for the CLI (and so the MCP),
// torre list and the mentions; leaving the stack keeps the wire, and so
// does removing its base.
func TestStackReachesEveryNote(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	plano, arq, dec := e.notam("Plano", 0), e.notam("Arquitetura", 500), e.notam("Decisoes", 1000)
	e.connect(a, plano)

	if r := e.run(a, "note", "read", "Decisoes"); r.Code != 404 {
		t.Fatalf("before stacking: %d %s", r.Code, r.Body)
	}
	for _, id := range []string{arq.ID, dec.ID} {
		if w := e.call("POST", "/api/notes/"+id+"/stack", map[string]string{"onto": plano.ID}); w.Code != 200 {
			t.Fatalf("stack: %d %s", w.Code, w.Body)
		}
	}
	if w := e.call("POST", "/api/notes/"+plano.ID+"/stack", map[string]string{"onto": a.ID}); w.Code != 400 || !strings.Contains(w.Body.String(), "notam.stack_kind") {
		t.Fatalf("onto a flight: %d %s", w.Code, w.Body)
	}

	if r := e.run(a, "note", "write", "Decisoes", "usar SQLite"); r.Code != 200 {
		t.Fatalf("write in the stack: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "note", "read", "decisoes"); r.Code != 200 || !strings.Contains(r.Body, "usar SQLite") {
		t.Fatalf("read in the stack: %d %s", r.Code, r.Body)
	}
	if r := e.run(b, "note", "read", "Decisoes"); r.Code != 404 {
		t.Fatalf("not wired: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "note", "list"); !strings.Contains(r.Body, `- name: "Arquitetura", file: arquitetura.md, stack: "Plano"`) {
		t.Fatalf("note list:\n%s", r.Body)
	}
	if r := e.run(a, "list"); !strings.Contains(r.Body, `name: "Decisoes", file: decisoes.md, stack: "Plano"`) || !strings.Contains(r.Body, `name: "Plano", file: plano.md, stack: "Plano"`) {
		t.Fatalf("list:\n%s", r.Body)
	}
	if err := e.app.Send(a.ID, ComposeRequest{Text: "veja @Decisoes", Mentions: []string{dec.ID}}); err != nil {
		t.Fatalf("mention in the stack: %v", err)
	}

	// Out of the stack, with the stack's wire.
	if w := e.call("POST", "/api/notes/"+dec.ID+"/unstack", nil); w.Code != 200 {
		t.Fatalf("unstack: %d %s", w.Code, w.Body)
	}
	st := e.app.Server.Store()
	if n, _ := st.Node(dec.ID); n.Note.Stack != "" || !st.Connected(a.ID, dec.ID) {
		t.Fatalf("out: %+v", n.Note)
	}
	// The base removed: the next note holds the wire (and alone, isn't a stack).
	if w := e.call("DELETE", "/api/nodes/"+plano.ID, nil); w.Code != http.StatusNoContent {
		t.Fatalf("remove the base: %d %s", w.Code, w.Body)
	}
	if n, _ := st.Node(arq.ID); n.Note.Stack != "" || !st.Connected(a.ID, arq.ID) {
		t.Fatalf("after the base: %+v", n.Note)
	}
	if r := e.run(a, "note", "read", "Arquitetura"); r.Code != 200 {
		t.Fatalf("read after the base: %d %s", r.Code, r.Body)
	}
}

// A plan keeps its stacks: saved from a selected stack, they fly in order.
func TestPlanKeepsStacks(t *testing.T) {
	e := newEnv(t, fastLimits)
	plano, arq := e.notam("Plano", 0), e.notam("Arquitetura", 500)
	if w := e.call("POST", "/api/notes/"+arq.ID+"/stack", map[string]string{"onto": plano.ID}); w.Code != 200 {
		t.Fatalf("stack: %d %s", w.Code, w.Body)
	}
	w := e.call("POST", "/api/plans", map[string]any{"name": "Docs", "scope": "selection", "nodes": []string{plano.ID}})
	var p plans.Plan
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != 201 || len(p.Notes) != 2 || p.Notes[1].Stack != p.Notes[0].Key || p.Notes[1].StackPos != 1 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w = e.call("POST", "/api/plans/docs/apply", map[string]any{})
	var res PlanResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || len(res.Nodes) != 2 || len(res.Warnings) != 0 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	var titles []string
	for _, n := range e.app.Server.Store().StackMembers(res.Nodes[0]) {
		titles = append(titles, n.Note.Title)
	}
	if strings.Join(titles, ",") != "Plano 2,Arquitetura 2" {
		t.Fatalf("stack flew as %v", titles)
	}
}

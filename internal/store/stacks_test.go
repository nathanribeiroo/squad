package store

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

type stackEnv struct {
	t *testing.T
	s *Store
}

func newStackEnv(t *testing.T) *stackEnv {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.json"), Workspace{ID: "w", Name: "w"})
	if err != nil {
		t.Fatal(err)
	}
	return &stackEnv{t, s}
}

func (e *stackEnv) note(title string, x float64) Node {
	e.t.Helper()
	n, err := e.s.AddNode(Node{Kind: KindNote, X: x, Y: 10, W: 400, H: 350, Note: &NoteData{Title: title, File: title + ".md"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *stackEnv) term(name string) Node {
	e.t.Helper()
	n, err := e.s.AddNode(Node{Kind: KindTerminal, Terminal: &TerminalData{Name: name, Agent: "claude"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *stackEnv) wire(a, b Node) {
	e.t.Helper()
	if _, err := e.s.AddEdge(a.ID, b.ID, nil); err != nil {
		e.t.Fatal(err)
	}
}

// wires lists who each node is wired to, by name.
func (e *stackEnv) wires(n Node) []string {
	var out []string
	for _, o := range e.s.Neighbors(n.ID) {
		out = append(out, o.Label())
	}
	slices.Sort(out)
	return out
}

func (e *stackEnv) stack(id string) []string {
	var out []string
	for _, n := range e.s.StackMembers(id) {
		out = append(out, n.Note.Title)
	}
	return out
}

// Stacking moves the wires to the base without repeating them, drops the
// wire between the two notes, and gives the notes the base's place.
func TestStackWiresAndPlace(t *testing.T) {
	e := newStackEnv(t)
	plan, arch, dec := e.note("Plano", 0), e.note("Arquitetura", 500), e.note("Decisoes", 1000)
	imp, rev := e.term("IMP"), e.term("REV")
	e.wire(imp, plan)
	e.wire(imp, arch) // the same flight on both: one wire after stacking
	e.wire(rev, arch)
	e.wire(plan, arch) // between the two: gone

	ch, err := e.s.Stack(arch.ID, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.stack(plan.ID); !slices.Equal(got, []string{"Plano", "Arquitetura"}) {
		t.Fatalf("stack: %v", got)
	}
	if got := e.wires(plan); !slices.Equal(got, []string{"IMP", "REV"}) {
		t.Fatalf("base wires: %v", got)
	}
	if got := e.wires(arch); len(got) != 0 {
		t.Fatalf("the other note keeps wires: %v", got)
	}
	if len(ch.Nodes) != 2 || len(ch.Removed) != 3 || len(ch.Added) != 1 {
		t.Fatalf("change: %d nodes, %d removed, %d added", len(ch.Nodes), len(ch.Removed), len(ch.Added))
	}
	if a, _ := e.s.Node(arch.ID); a.X != 0 || a.Note.Stack != plan.ID || a.Note.StackPos != 1 {
		t.Fatalf("arch: %+v %+v", a, a.Note)
	}

	// A stack on a stack: all of it goes on top, in order.
	e.wire(dec, rev)
	other := e.note("Riscos", 1500)
	if _, err := e.s.Stack(other.ID, dec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Stack(dec.ID, plan.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.stack(arch.ID); !slices.Equal(got, []string{"Plano", "Arquitetura", "Decisoes", "Riscos"}) {
		t.Fatalf("merged: %v", got)
	}
	if got := e.wires(plan); !slices.Equal(got, []string{"IMP", "REV"}) {
		t.Fatalf("merged wires: %v", got)
	}

	// A new wire to a note in the stack ends at the base.
	qa := e.term("TST")
	edge, err := e.s.AddEdge(qa.ID, dec.ID, nil)
	if err != nil || edge.Other(qa.ID) != plan.ID {
		t.Fatalf("wire to the stack: %+v %v", edge, err)
	}
	if _, err := e.s.AddEdge(arch.ID, dec.ID, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wire inside the stack: %v", err)
	}

	// Moving or resizing the base moves the stack.
	if _, err := e.s.UpdateNode(plan.ID, func(n *Node) error { n.X, n.W = 77, 480; n.Note.Stack = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.s.Node(dec.ID); d.X != 77 || d.W != 480 {
		t.Fatalf("the stack didn't follow: %+v", d)
	}
	if p, _ := e.s.Node(plan.ID); p.Note.Stack != plan.ID {
		t.Fatal("an update changed the stack")
	}

	// Refused: itself, its own stack, another kind.
	for _, c := range [][2]string{{plan.ID, plan.ID}, {dec.ID, arch.ID}, {imp.ID, plan.ID}} {
		if _, err := e.s.Stack(c[0], c[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("stack %v: %v", c, err)
		}
	}
}

// Leaving a stack keeps its wires (copies); the base leaving hands them to
// the next note; a stack of one is no stack.
func TestUnstack(t *testing.T) {
	e := newStackEnv(t)
	a, b, c := e.note("A", 0), e.note("B", 500), e.note("C", 1000)
	imp := e.term("IMP")
	e.wire(imp, a)
	if _, err := e.s.Stack(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Stack(c.ID, a.ID); err != nil {
		t.Fatal(err)
	}

	ch, err := e.s.Unstack(b.ID, false, true)
	if err != nil {
		t.Fatal(err)
	}
	nb, _ := e.s.Node(b.ID)
	if nb.Note.Stack != "" || nb.X != 440 || !slices.Equal(e.wires(nb), []string{"IMP"}) || len(ch.Added) != 1 {
		t.Fatalf("B out: %+v wires %v", nb, e.wires(nb))
	}
	if got := e.stack(a.ID); !slices.Equal(got, []string{"A", "C"}) {
		t.Fatalf("left: %v", got)
	}
	if nc, _ := e.s.Node(c.ID); nc.Note.StackPos != 1 {
		t.Fatalf("C renumbered: %+v", nc.Note)
	}

	// The base leaves: C holds the wires, and alone it is no stack.
	if _, err := e.s.Unstack(a.ID, false, true); err != nil {
		t.Fatal(err)
	}
	na, _ := e.s.Node(a.ID)
	nc, _ := e.s.Node(c.ID)
	if na.Note.Stack != "" || nc.Note.Stack != "" || !slices.Equal(e.wires(nc), []string{"IMP"}) || !slices.Equal(e.wires(na), []string{"IMP"}) {
		t.Fatalf("A %+v, C %+v, wires %v %v", na.Note, nc.Note, e.wires(na), e.wires(nc))
	}
	if _, err := e.s.Unstack(a.ID, false, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("not stacked: %v", err)
	}

	// All at once; then removing the base of another stack.
	if _, err := e.s.Stack(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Stack(c.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Unstack(c.ID, true, true); err != nil {
		t.Fatal(err)
	}
	for _, n := range []Node{a, b, c} {
		if m, _ := e.s.Node(n.ID); m.Note.Stack != "" || !slices.Equal(e.wires(m), []string{"IMP"}) {
			t.Fatalf("%s after all: %+v %v", n.Note.Title, m.Note, e.wires(m))
		}
	}
	if _, err := e.s.Stack(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Stack(c.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.RemoveNode(a.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.stack(b.ID); !slices.Equal(got, []string{"B", "C"}) || !slices.Equal(e.wires(b), []string{"IMP"}) {
		t.Fatalf("after removing the base: %v wires %v", got, e.wires(b))
	}
}

// A stack moves to another floor as one.
func TestStackFloors(t *testing.T) {
	e := newStackEnv(t)
	if _, err := e.s.AddFloor(Floor{ID: "fl", Name: "FL"}); err != nil {
		t.Fatal(err)
	}
	a, b := e.note("A", 0), e.note("B", 500)
	up, err := e.s.AddNode(Node{Kind: KindNote, Floor: "fl", W: 400, H: 350, Note: &NoteData{Title: "C", File: "c.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Stack(up.ID, a.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("across levels: %v", err)
	}
	if _, err := e.s.Stack(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.MoveNode(a.ID, "fl", 5, 6); err != nil {
		t.Fatal(err)
	}
	if nb, _ := e.s.Node(b.ID); nb.Floor != "fl" || nb.X != 5 || nb.Y != 6 {
		t.Fatalf("B stayed: %+v", nb)
	}
}

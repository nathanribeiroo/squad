package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "workspace.json"), Workspace{ID: "w", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func term(name string) Node {
	return Node{Kind: KindTerminal, W: 560, H: 440, Terminal: &TerminalData{Name: name, Agent: "shell"}}
}

func TestNodesEdgesAndPersistence(t *testing.T) {
	s := newStore(t)
	a, err := s.AddNode(term("Planejador"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.AddNode(term("Implementador"))
	note, err := s.AddNode(Node{Kind: KindNote, Note: &NoteData{Title: "Plano", File: "PLANO.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddNode(term("Planejador")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate name: %v", err)
	}
	e, err := s.AddEdge(a.ID, b.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEdge(b.ID, a.ID, nil); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("reverse duplicate edge: %v", err)
	}
	if _, err := s.AddEdge(a.ID, a.ID, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self edge: %v", err)
	}
	if _, err := s.AddEdge(a.ID, "nope", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing node: %v", err)
	}
	_, _ = s.AddEdge(a.ID, note.ID, nil)
	if !s.Connected(b.ID, a.ID) || s.Connected(b.ID, note.ID) {
		t.Fatal("Connected wrong")
	}
	if got := len(s.Neighbors(a.ID)); got != 2 {
		t.Fatalf("neighbors of a: %d", got)
	}
	if _, err := s.UpdateEdgeRope(e.ID, [][2]float64{{1, 2}}); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk.
	s2, err := Open(s.Path(), Workspace{})
	if err != nil {
		t.Fatal(err)
	}
	ws := s2.Snapshot()
	if len(ws.Nodes) != 3 || len(ws.Edges) != 2 || ws.Name != "test" {
		t.Fatalf("reloaded: %+v", ws)
	}
	raw, _ := os.ReadFile(s.Path())
	if !strings.Contains(string(raw), `"type": "workspace"`) || !strings.Contains(string(raw), `"schemaVersion": 1`) {
		t.Fatalf("envelope missing:\n%s", raw)
	}

	removed, err := s2.RemoveNode(a.ID)
	if err != nil || len(removed) != 2 {
		t.Fatalf("remove node: %v removed=%d", err, len(removed))
	}
	if len(s2.Snapshot().Edges) != 0 {
		t.Fatal("edges not removed with node")
	}
}

func TestUpdateNodeKeepsIdentity(t *testing.T) {
	s := newStore(t)
	a, _ := s.AddNode(term("A"))
	_, _ = s.AddNode(term("B"))
	n, err := s.UpdateNode(a.ID, func(n *Node) error {
		n.X, n.Y = 10, 20
		n.ID, n.Kind = "hijack", KindNote
		return nil
	})
	if err != nil || n.ID != a.ID || n.Kind != KindTerminal || n.X != 10 {
		t.Fatalf("update: %+v %v", n, err)
	}
	if _, err := s.UpdateNode(a.ID, func(n *Node) error { n.Terminal.Name = "B"; return nil }); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("rename to existing: %v", err)
	}
	if found, ok := s.TerminalByName("A"); !ok || found.ID != a.ID {
		t.Fatal("TerminalByName")
	}
}

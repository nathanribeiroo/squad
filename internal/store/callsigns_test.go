package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCallsigns(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.json"), Workspace{ID: "w", Name: "w"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(name, callsign string) Node {
		t.Helper()
		n, err := s.AddNode(Node{Kind: KindTerminal, Terminal: &TerminalData{Name: name, Agent: "claude", Callsign: callsign}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	// From the role's abbreviation (the server passes it), numbered when taken.
	a, b, c := add("Bia", "IMP"), add("Caio", "IMP"), add("Davi", "imp")
	if a.Terminal.Callsign != "IMP" || b.Terminal.Callsign != "IM2" || c.Terminal.Callsign != "IM3" {
		t.Fatalf("numbered: %s %s %s", a.Terminal.Callsign, b.Terminal.Callsign, c.Terminal.Callsign)
	}
	// Without one, from the name's consonants.
	if g := add("Gemini", ""); g.Terminal.Callsign != "GMN" {
		t.Fatalf("from the name: %s", g.Terminal.Callsign)
	}
	// Renamed by hand: cleaned, and refused when another flight has it.
	_, err = s.UpdateNode(b.ID, func(n *Node) error { n.Terminal.Callsign = "rv-1"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Node(b.ID); n.Terminal.Callsign != "RV1" {
		t.Fatalf("cleaned: %s", n.Terminal.Callsign)
	}
	if _, err := s.UpdateNode(c.ID, func(n *Node) error { n.Terminal.Callsign = "IMP"; return nil }); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("taken: %v", err)
	}
	// An update without one keeps it.
	if _, err := s.UpdateNode(c.ID, func(n *Node) error { n.Terminal.Callsign = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Node(c.ID); n.Terminal.Callsign != "IM3" {
		t.Fatalf("kept: %s", n.Terminal.Callsign)
	}
	if got, _ := s.AssignCallsign(c.ID, "REV"); got != "REV" {
		t.Fatalf("assign: %s", got)
	}
}

func TestFloorNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	s, err := Open(path, Workspace{ID: "w", Name: "w", Floors: []Floor{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}})
	if err != nil {
		t.Fatal(err)
	}
	// Opened again: the floors from before get numbers in order.
	if s, err = Open(path, Workspace{}); err != nil {
		t.Fatal(err)
	}
	if f := s.Floors(); f[0].Number != 1 || f[1].Number != 2 {
		t.Fatalf("migrated: %+v", f)
	}
	c, err := s.AddFloor(Floor{ID: "c", Name: "C"})
	if err != nil || c.Number != 3 {
		t.Fatalf("new: %+v %v", c, err)
	}
	// A discarded floor's number isn't given again.
	if err := s.RemoveFloor("c"); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.AddFloor(Floor{ID: "d", Name: "D"}); d.Number != 4 {
		t.Fatalf("after a discard: %d", d.Number)
	}
}

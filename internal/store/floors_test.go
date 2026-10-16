package store

import (
	"errors"
	"slices"
	"testing"
)

func TestFloors(t *testing.T) {
	s := newStore(t)
	if c := s.FloorConfig(); !slices.Equal(c.Copy, []string{".env*"}) || c.Setup != "" {
		t.Fatalf("default config: %+v", c)
	}
	n := term("Bia")
	n.Floor = "login"
	if _, err := s.AddNode(n); !errors.Is(err, ErrNotFound) {
		t.Fatalf("node on an unknown floor: %v", err)
	}
	if _, err := s.AddFloor(Floor{ID: "login", Name: "Login", Branch: "login", Status: FloorReady, Copied: []string{".env"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFloor(Floor{ID: "login-2", Name: "LOGIN"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate floor name: %v", err)
	}
	bia, err := s.AddNode(n)
	if err != nil || bia.Floor != "login" {
		t.Fatalf("node on a floor: %+v %v", bia, err)
	}
	// Editing a node keeps its floor.
	if u, err := s.UpdateNode(bia.ID, func(n *Node) error { n.Floor = ""; n.X = 10; return nil }); err != nil || u.Floor != "login" {
		t.Fatalf("update: %+v %v", u, err)
	}
	if err := s.RemoveFloor("login"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("remove a floor with nodes: %v", err)
	}
	if f, err := s.UpdateFloor("login", func(f *Floor) { f.Status = FloorFailed; f.ID = "x" }); err != nil || f.ID != "login" || f.Status != FloorFailed {
		t.Fatalf("update floor: %+v %v", f, err)
	}
	if _, err := s.SetFloorConfig(FloorConfig{Copy: []string{" .env ", "", ".env ", "config/*.json"}, Setup: " npm ci "}); err != nil {
		t.Fatal(err)
	}

	// Everything survives a reload.
	r, err := Open(s.Path(), Workspace{})
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := r.Floor("login"); !ok || f.Status != FloorFailed || !slices.Equal(f.Copied, []string{".env"}) {
		t.Fatalf("reloaded floor: %+v", f)
	}
	if c := r.FloorConfig(); !slices.Equal(c.Copy, []string{".env", "config/*.json"}) || c.Setup != "npm ci" {
		t.Fatalf("reloaded config: %+v", c)
	}
	if got := OnFloor(r.Snapshot().Nodes, "login"); len(got) != 1 || got[0].ID != bia.ID {
		t.Fatalf("on floor: %+v", got)
	}

	// Moving the node down frees the floor.
	if m, err := s.MoveNode(bia.ID, "", 1, 2); err != nil || m.Floor != "" || m.X != 1 {
		t.Fatalf("move: %+v %v", m, err)
	}
	if err := s.RemoveFloor("login"); err != nil {
		t.Fatal(err)
	}
	if len(s.Floors()) != 0 {
		t.Fatal("floor kept")
	}
}

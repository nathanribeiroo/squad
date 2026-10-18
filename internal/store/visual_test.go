package store

import (
	"errors"
	"slices"
	"testing"
)

func TestGroupsAndDrawings(t *testing.T) {
	e := newStackEnv(t)
	a := e.term("A")
	g, err := e.s.AddNode(Node{Kind: KindGroup, X: -10, Y: -10, W: 800, H: 600, Group: &GroupData{Name: "  ", Members: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if g.Group.Name != "Grupo" || g.Group.Members != nil || g.Label() != "Grupo" || !g.Visual() {
		t.Fatalf("group: %+v", g.Group)
	}
	// A group carries nothing of another kind.
	if g2, err := e.s.AddNode(Node{Kind: KindGroup, Terminal: &TerminalData{Name: "X"}}); err != nil || g2.Terminal != nil || g2.Group == nil {
		t.Fatalf("group with terminal data: %+v %v", g2, err)
	}

	for _, d := range []DrawingData{
		{Shape: "blob"},
		{Shape: "arrow", Points: [][2]float64{{0, 0}}},
		{Shape: "pen"},
		{Shape: "pen", Points: [][2]float64{{0, 0}, {2, 0}}},
		{Shape: "text", Text: string(make([]rune, MaxDrawingText+1))},
	} {
		if _, err := e.s.AddNode(Node{Kind: KindDrawing, W: 10, H: 10, Drawing: &d}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v: %v", d, err)
		}
	}
	if _, err := e.s.AddNode(Node{Kind: KindDrawing}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("drawing without data: %v", err)
	}
	d, err := e.s.AddNode(Node{Kind: KindDrawing, W: 100, H: 50, Drawing: &DrawingData{Shape: "rect", Points: [][2]float64{{0, 0}}, Text: "x", Color: "amber"}})
	if err != nil || d.Drawing.Points != nil || d.Drawing.Text != "" {
		t.Fatalf("rect: %+v %v", d.Drawing, err)
	}
	pen, err := e.s.AddNode(Node{Kind: KindDrawing, W: 100, H: 50, Drawing: &DrawingData{Shape: "pen", Points: [][2]float64{{0, 0}, {0.5, 1}, {1, 0.2}}}})
	if err != nil {
		t.Fatal(err)
	}

	// No wire reaches them.
	for _, v := range []Node{g, d} {
		if _, err := e.s.AddEdge(a.ID, v.ID, nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("wire to %s: %v", v.Kind, err)
		}
	}

	// Collapsing keeps who it hides; opening forgets them.
	c, err := e.s.UpdateNode(g.ID, func(n *Node) error {
		n.W, n.H = 280, 96
		n.Group.Collapsed, n.Group.OpenW, n.Group.OpenH = true, 800, 600
		n.Group.Members = []string{a.ID, a.ID, g.ID, pen.ID}
		return nil
	})
	if err != nil || !slices.Equal(c.Group.Members, sorted(a.ID, pen.ID)) {
		t.Fatalf("collapse: %+v %v", c.Group, err)
	}
	if x, y, w, h := c.Box(); x != -10 || y != -10 || w != 800 || h != 600 {
		t.Fatalf("open box: %v %v %v %v", x, y, w, h)
	}
	if got := nodeIDs(Inside(e.s.Snapshot().Nodes, c)); !slices.Equal(got, sorted(a.ID, pen.ID)) {
		t.Fatalf("inside a collapsed group: %v", got)
	}
	// The stored node is a copy.
	c.Group.Members[0] = "changed"
	if n, _ := e.s.Node(g.ID); slices.Contains(n.Group.Members, "changed") {
		t.Fatal("members shared with the caller")
	}
	o, err := e.s.UpdateNode(g.ID, func(n *Node) error { n.W, n.H, n.Group.Collapsed = 800, 600, false; return nil })
	if err != nil || o.Group.Members != nil || o.Group.OpenW != 0 {
		t.Fatalf("open: %+v %v", o.Group, err)
	}
	if _, err := e.s.UpdateNode(pen.ID, func(n *Node) error { n.Drawing.Shape = "arrow"; return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a stroke turned into an arrow of 3 points: %v", err)
	}

	// Inside an open group: what lies whole in its frame.
	nodes := []Node{
		{ID: "g", Kind: KindGroup, X: 0, Y: 0, W: 100, H: 100, Group: &GroupData{Name: "G"}},
		{ID: "in", X: 10, Y: 10, W: 50, H: 50},
		{ID: "edge", X: 60, Y: 60, W: 50, H: 50},
		{ID: "up", X: 10, Y: 10, W: 50, H: 50, Floor: "f1"},
	}
	if got := nodeIDs(Inside(nodes, nodes[0])); !slices.Equal(got, []string{"in"}) {
		t.Fatalf("inside: %v", got)
	}
}

func nodeIDs(nodes []Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	slices.Sort(out)
	return out
}

func sorted(s ...string) []string {
	slices.Sort(s)
	return s
}

// Moving several nodes saves them at once; a note of a stack takes it along.
func TestMoveNodes(t *testing.T) {
	e := newStackEnv(t)
	a, b, c := e.note("a", 0), e.note("b", 500), e.term("T")
	if _, err := e.s.Stack(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	moved, err := e.s.MoveNodes([]Move{{ID: a.ID, X: 1000, Y: 2000}, {ID: c.ID, X: 5, Y: 6}})
	if err != nil || len(moved) != 3 {
		t.Fatalf("moved: %+v %v", moved, err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if n, _ := e.s.Node(id); n.X != 1000 || n.Y != 2000 {
			t.Fatalf("%s at %v,%v", n.Label(), n.X, n.Y)
		}
	}
	if _, err := e.s.MoveNodes([]Move{{ID: c.ID, X: 1, Y: 1}, {ID: "nope"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown node: %v", err)
	}
	if n, _ := e.s.Node(c.ID); n.X != 5 {
		t.Fatal("a failed move moved something")
	}
}

// Open frames and drawings don't take room; a collapsed group's card does.
func TestFreeSpotIgnoresVisuals(t *testing.T) {
	near := Node{X: 0, Y: 0, W: 100, H: 100}
	nodes := []Node{
		near,
		{Kind: KindGroup, X: -500, Y: -500, W: 2000, H: 2000, Group: &GroupData{}},
		{Kind: KindDrawing, X: 140, Y: 0, W: 100, H: 100, Drawing: &DrawingData{Shape: "rect"}},
	}
	if x, y := FreeSpot(nodes, near, 100, 100); x != 140 || y != 0 {
		t.Fatalf("over the visuals: %v,%v", x, y)
	}
	nodes = append(nodes, Node{Kind: KindGroup, X: 140, Y: 0, W: 100, H: 60, Group: &GroupData{Collapsed: true}})
	if x, y := FreeSpot(nodes, near, 100, 100); x == 140 && y == 0 {
		t.Fatal("over a collapsed group's card")
	}
}

package store

import "testing"

func TestFreeSpot(t *testing.T) {
	me := Node{ID: "me", X: 100, Y: 100, W: 560, H: 440}
	overlaps := func(nodes []Node, x, y, w, h float64) bool {
		for _, n := range nodes {
			if x < n.X+n.W && n.X < x+w && y < n.Y+n.H && n.Y < y+h {
				return true
			}
		}
		return false
	}
	// Alone: right next to it, same top.
	if x, y := FreeSpot([]Node{me}, me, 560, 440); x != 100+560+placeGap || y != 100 {
		t.Fatalf("first spot %v,%v", x, y)
	}
	// Each new node takes another free spot near it.
	nodes := []Node{me}
	for i := 0; i < 12; i++ {
		x, y := FreeSpot(nodes, me, 560, 440)
		if overlaps(nodes, x, y, 560, 440) {
			t.Fatalf("node %d at %v,%v overlaps", i, x, y)
		}
		nodes = append(nodes, Node{X: x, Y: y, W: 560, H: 440})
	}
	// The second one goes below the first one on the right, not far away.
	if nodes[2].X != nodes[1].X || nodes[2].Y != 100+440+placeGap {
		t.Fatalf("second spot %v,%v", nodes[2].X, nodes[2].Y)
	}
}

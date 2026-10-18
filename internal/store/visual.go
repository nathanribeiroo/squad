package store

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// Groups and drawings are for the user's eyes only: a group is a named frame
// around nodes (whatever lies inside it; moving it moves them), a drawing a
// stroke, an arrow, a shape or a text on the canvas. Agents never see them
// (torre list, names, MCP) and no wire reaches them.

// GroupData configures a group node.
type GroupData struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"` // a color key of the UI: teal, blue…
	// Collapsed: the group shows as a card the node's size; Members are the
	// nodes it hides (those inside the frame when it collapsed) and OpenW,
	// OpenH the frame's size, given back when it opens.
	Collapsed bool     `json:"collapsed,omitempty"`
	Members   []string `json:"members,omitempty"`
	OpenW     float64  `json:"openW,omitempty"`
	OpenH     float64  `json:"openH,omitempty"`
}

// DrawingData configures a drawing node.
type DrawingData struct {
	Shape string `json:"shape"` // pen | arrow | rect | ellipse | text
	// Points of a pen stroke or an arrow (from, to), 0–1 across the node's
	// box: resizing the node scales the drawing.
	Points [][2]float64 `json:"points,omitempty"`
	Color  string       `json:"color,omitempty"`
	Text   string       `json:"text,omitempty"` // a text's words
}

// Drawing shapes.
var drawingShapes = []string{"pen", "arrow", "rect", "ellipse", "text"}

// Limits on groups and drawings.
const (
	MaxGroupName     = 60
	MaxGroupMembers  = 500
	MaxDrawingPoints = 2000
	MaxDrawingText   = 2000
	maxColorKey      = 24
)

// Visual reports whether a node is a group or a drawing.
func (n Node) Visual() bool { return n.Kind == KindGroup || n.Kind == KindDrawing }

// Box returns the node's frame: a collapsed group's open frame.
func (n Node) Box() (x, y, w, h float64) {
	if n.Group != nil && n.Group.Collapsed && n.Group.OpenW > 0 && n.Group.OpenH > 0 {
		return n.X, n.Y, n.Group.OpenW, n.Group.OpenH
	}
	return n.X, n.Y, n.W, n.H
}

// checkVisual validates and tidies a group's or a drawing's data, and drops
// data of another kind.
func checkVisual(n *Node) error {
	switch n.Kind {
	case KindGroup:
		n.Terminal, n.Note, n.Portal, n.Drawing = nil, nil, nil, nil
		if n.Group == nil {
			n.Group = &GroupData{}
		}
		g := n.Group
		g.Name = strings.TrimSpace(g.Name)
		if g.Name == "" {
			g.Name = "Grupo"
		}
		if utf8.RuneCountInString(g.Name) > MaxGroupName {
			return fmt.Errorf("%w: group names have at most %d characters", ErrInvalid, MaxGroupName)
		}
		if len(g.Color) > maxColorKey {
			return fmt.Errorf("%w: color %q", ErrInvalid, g.Color)
		}
		if !g.Collapsed {
			g.Members, g.OpenW, g.OpenH = nil, 0, 0
		}
		if len(g.Members) > MaxGroupMembers {
			return fmt.Errorf("%w: a group hides at most %d nodes", ErrInvalid, MaxGroupMembers)
		}
		g.Members = slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(g.Members))), func(id string) bool { return id == "" || id == n.ID })
		if g.OpenW < 0 || g.OpenH < 0 {
			return fmt.Errorf("%w: a group's size", ErrInvalid)
		}
	case KindDrawing:
		n.Terminal, n.Note, n.Portal, n.Group = nil, nil, nil, nil
		d := n.Drawing
		if d == nil || !slices.Contains(drawingShapes, d.Shape) {
			return fmt.Errorf("%w: a drawing is one of %s", ErrInvalid, strings.Join(drawingShapes, ", "))
		}
		if len(d.Color) > maxColorKey {
			return fmt.Errorf("%w: color %q", ErrInvalid, d.Color)
		}
		switch d.Shape {
		case "pen":
			if len(d.Points) == 0 || len(d.Points) > MaxDrawingPoints {
				return fmt.Errorf("%w: a stroke has 1 to %d points", ErrInvalid, MaxDrawingPoints)
			}
		case "arrow":
			if len(d.Points) != 2 {
				return fmt.Errorf("%w: an arrow has 2 points", ErrInvalid)
			}
		default:
			d.Points = nil
		}
		for _, p := range d.Points {
			if math.IsNaN(p[0]) || math.IsNaN(p[1]) || p[0] < -0.01 || p[0] > 1.01 || p[1] < -0.01 || p[1] > 1.01 {
				return fmt.Errorf("%w: a drawing's points go from 0 to 1", ErrInvalid)
			}
		}
		if d.Shape != "text" {
			d.Text = ""
		}
		if utf8.RuneCountInString(d.Text) > MaxDrawingText {
			return fmt.Errorf("%w: a text has at most %d characters", ErrInvalid, MaxDrawingText)
		}
	default:
		n.Group, n.Drawing = nil, nil
	}
	return nil
}

// Inside returns the nodes of nodes that lie whole inside g's frame, on its
// floor: a group's contents (a collapsed group's are its members).
func Inside(nodes []Node, g Node) []Node {
	var out []Node
	if g.Group != nil && g.Group.Collapsed {
		for _, n := range nodes {
			if slices.Contains(g.Group.Members, n.ID) && n.Floor == g.Floor {
				out = append(out, n)
			}
		}
		return out
	}
	gx, gy, gw, gh := g.Box()
	for _, n := range nodes {
		if n.ID == g.ID || n.Floor != g.Floor {
			continue
		}
		x, y, w, h := n.Box()
		if x >= gx && y >= gy && x+w <= gx+gw && y+h <= gy+gh {
			out = append(out, n)
		}
	}
	return out
}

// Move puts a node at X, Y (MoveNodes).
type Move struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

// MoveNodes moves several nodes at once (a group with what is inside it)
// and saves once; a note of a stack takes the stack along. It returns the
// nodes moved.
func (s *Store) MoveNodes(moves []Move) ([]Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range moves {
		if s.nodeIndexLocked(m.ID) < 0 {
			return nil, fmt.Errorf("node %q: %w", m.ID, ErrNotFound)
		}
		if math.IsNaN(m.X) || math.IsNaN(m.Y) || math.IsInf(m.X, 0) || math.IsInf(m.Y, 0) {
			return nil, fmt.Errorf("%w: a position", ErrInvalid)
		}
	}
	restore := s.snapshotLocked()
	var moved []int
	for _, m := range moves {
		for _, i := range s.stackLocked(s.baseLocked(m.ID)) {
			s.ws.Nodes[i].X, s.ws.Nodes[i].Y = m.X, m.Y
			if !slices.Contains(moved, i) {
				moved = append(moved, i)
			}
		}
	}
	if err := s.save(); err != nil {
		restore()
		return nil, err
	}
	out := make([]Node, 0, len(moved))
	for _, i := range moved {
		out = append(out, cloneNode(s.ws.Nodes[i]))
	}
	return out, nil
}

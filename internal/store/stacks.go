package store

import (
	"fmt"
	"slices"
	"time"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/ids"
)

// A stack of notes takes the place of one note on the canvas. The note at
// its base holds the stack's wires and its place: a wire to any note of a
// stack ends at the base, the other notes have no wires of their own, and
// they share the base's box. A wire to the stack reaches every note in it
// (notes.Service follows the stack like a chain of notes).

// StackChange is what stacking or unstacking changed, for the events: the
// notes updated, and the wires removed and added.
type StackChange struct {
	Nodes   []Node
	Added   []Edge
	Removed []Edge
}

func (s *Store) nodeIndexLocked(id string) int {
	return slices.IndexFunc(s.ws.Nodes, func(n Node) bool { return n.ID == id })
}

// baseLocked is the base of the stack id is in, or id when it isn't stacked.
func (s *Store) baseLocked(id string) string {
	if i := s.nodeIndexLocked(id); i >= 0 && s.ws.Nodes[i].Note != nil && s.ws.Nodes[i].Note.Stack != "" {
		return s.ws.Nodes[i].Note.Stack
	}
	return id
}

// stackLocked returns the indexes of the notes stacked on base, in order
// (just base's when it isn't a stack; nil when there is no such node).
func (s *Store) stackLocked(base string) []int {
	var out []int
	for i, n := range s.ws.Nodes {
		if n.Note != nil && n.Note.Stack == base {
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		if i := s.nodeIndexLocked(base); i >= 0 {
			return []int{i}
		}
		return nil
	}
	slices.SortStableFunc(out, func(a, b int) int { return s.ws.Nodes[a].Note.StackPos - s.ws.Nodes[b].Note.StackPos })
	return out
}

// StackMembers returns the notes of the stack id is in, base first (just
// id's node when it isn't stacked).
func (s *Store) StackMembers(id string) []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Node
	for _, i := range s.stackLocked(s.baseLocked(id)) {
		out = append(out, cloneNode(s.ws.Nodes[i]))
	}
	return out
}

func stackError(msg, code string) error {
	return apierr.With(fmt.Errorf("%w: %s", ErrInvalid, msg), code)
}

// snapshotLocked keeps nodes and edges to restore if saving fails.
func (s *Store) snapshotLocked() func() {
	nodes := make([]Node, len(s.ws.Nodes))
	for i, n := range s.ws.Nodes {
		nodes[i] = cloneNode(n)
	}
	edges := slices.Clone(s.ws.Edges)
	return func() { s.ws.Nodes, s.ws.Edges = nodes, edges }
}

// Stack puts the note id (with its stack, when it has one) on top of the
// stack of onto, or makes a stack of the two. Their wires move to the base
// (a wire between them goes), and they take the base's place.
func (s *Store) Stack(id, onto string) (StackChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, j := s.nodeIndexLocked(id), s.nodeIndexLocked(onto)
	if i < 0 || j < 0 {
		return StackChange{}, ErrNotFound
	}
	a, b := s.ws.Nodes[i], s.ws.Nodes[j]
	switch {
	case a.Note == nil || b.Note == nil:
		return StackChange{}, stackError("only notes stack", "notam.stack_kind")
	case a.Floor != b.Floor:
		return StackChange{}, stackError("notes on different levels don't stack", "notam.stack_level")
	case s.baseLocked(id) == s.baseLocked(onto):
		return StackChange{}, stackError("a note can't stack on its own stack", "notam.stack_self")
	}
	restore := s.snapshotLocked()
	members := append(s.stackLocked(s.baseLocked(onto)), s.stackLocked(s.baseLocked(id))...)
	base := s.ws.Nodes[members[0]]
	in := map[string]bool{}
	for k, ix := range members {
		n := &s.ws.Nodes[ix]
		n.Note.Stack, n.Note.StackPos = base.ID, k
		n.X, n.Y, n.W, n.H = base.X, base.Y, base.W, base.H
		in[n.ID] = true
	}
	var ch StackChange
	ch.Removed, ch.Added = s.rewireLocked(in, base.ID)
	for _, ix := range members {
		ch.Nodes = append(ch.Nodes, cloneNode(s.ws.Nodes[ix]))
	}
	if err := s.save(); err != nil {
		restore()
		return StackChange{}, err
	}
	return ch, nil
}

// rewireLocked moves the wires of a stack's notes to its base: a wire
// between two of them goes, and one the base already has isn't repeated.
func (s *Store) rewireLocked(in map[string]bool, base string) (removed, added []Edge) {
	has := func(a, b string, edges []Edge) bool {
		return slices.ContainsFunc(edges, func(e Edge) bool { return (e.A == a && e.B == b) || (e.A == b && e.B == a) })
	}
	var kept []Edge
	var moved []string // the other ends, in order
	for _, e := range s.ws.Edges {
		inA, inB := in[e.A], in[e.B]
		switch {
		case !inA && !inB, e.A == base && !inB, e.B == base && !inA:
			kept = append(kept, e)
		case inA && inB:
			removed = append(removed, e)
		case inA:
			removed = append(removed, e)
			moved = append(moved, e.B)
		default:
			removed = append(removed, e)
			moved = append(moved, e.A)
		}
	}
	for _, other := range moved {
		if has(base, other, kept) {
			continue
		}
		e := Edge{ID: ids.New(), A: base, B: other, CreatedAt: time.Now().UTC()}
		kept = append(kept, e)
		added = append(added, e)
	}
	s.ws.Edges = kept
	return removed, added
}

// Unstack takes the note id out of its stack, or every note with all. With
// keep, the notes that leave keep the stack's wires (copies); the stack
// keeps them too. When the base leaves, the next note becomes the base and
// gets its wires; a stack left with one note is no stack.
func (s *Store) Unstack(id string, all, keep bool) (StackChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	restore := s.snapshotLocked()
	ch, err := s.unstackLocked(id, all, keep)
	if err != nil {
		return StackChange{}, err
	}
	if err := s.save(); err != nil {
		restore()
		return StackChange{}, err
	}
	return ch, nil
}

func (s *Store) unstackLocked(id string, all, keep bool) (StackChange, error) {
	i := s.nodeIndexLocked(id)
	if i < 0 {
		return StackChange{}, ErrNotFound
	}
	if s.ws.Nodes[i].Note == nil || s.ws.Nodes[i].Note.Stack == "" {
		return StackChange{}, stackError("the note isn't in a stack", "notam.not_stacked")
	}
	base := s.ws.Nodes[i].Note.Stack
	members := s.stackLocked(base)
	var leaving, staying []int
	for _, ix := range members {
		if all || s.ws.Nodes[ix].ID == id {
			leaving = append(leaving, ix)
		} else {
			staying = append(staying, ix)
		}
	}
	box := s.ws.Nodes[members[0]]
	// The wires of the stack: the base's.
	var others []string
	for _, e := range s.ws.Edges {
		if e.A == base || e.B == base {
			others = append(others, e.Other(base))
		}
	}
	var ch StackChange
	connect := func(a string) {
		for _, o := range others {
			if !slices.ContainsFunc(s.ws.Edges, func(e Edge) bool { return (e.A == a && e.B == o) || (e.A == o && e.B == a) }) {
				e := Edge{ID: ids.New(), A: a, B: o, CreatedAt: time.Now().UTC()}
				s.ws.Edges = append(s.ws.Edges, e)
				ch.Added = append(ch.Added, e)
			}
		}
	}
	newBase := base
	if len(staying) > 0 && s.ws.Nodes[staying[0]].ID != base {
		newBase = s.ws.Nodes[staying[0]].ID // the base leaves: the next one holds the wires
		connect(newBase)
	}
	if keep {
		for _, ix := range leaving {
			if s.ws.Nodes[ix].ID != base {
				connect(s.ws.Nodes[ix].ID)
			}
		}
	}
	// Leaving notes go beside the stack, in a row; with all, the base stays.
	k := 0
	for _, ix := range leaving {
		n := &s.ws.Nodes[ix]
		n.Note.Stack, n.Note.StackPos = "", 0
		if all && n.ID == base {
			continue
		}
		k++
		n.X, n.Y = box.X+float64(k)*(box.W+40), box.Y
		ch.Nodes = append(ch.Nodes, cloneNode(*n))
	}
	if all {
		ch.Nodes = append(ch.Nodes, cloneNode(s.ws.Nodes[members[0]]))
	}
	for p, ix := range staying {
		n := &s.ws.Nodes[ix]
		if len(staying) == 1 {
			n.Note.Stack, n.Note.StackPos = "", 0
		} else {
			n.Note.Stack, n.Note.StackPos = newBase, p
		}
		ch.Nodes = append(ch.Nodes, cloneNode(*n))
	}
	return ch, nil
}

// syncStackLocked gives the notes of a stack the box of its base, after the
// base moved or was resized.
func (s *Store) syncStackLocked(base Node) {
	if base.Note == nil || base.Note.Stack != base.ID {
		return
	}
	for i := range s.ws.Nodes {
		if n := &s.ws.Nodes[i]; n.Note != nil && n.Note.Stack == base.ID && n.ID != base.ID {
			n.X, n.Y, n.W, n.H, n.Floor = base.X, base.Y, base.W, base.H, base.Floor
		}
	}
}

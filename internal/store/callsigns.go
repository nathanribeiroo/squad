package store

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nathanribeiroo/squad/internal/ids"
)

// freeCallsignLocked returns a callsign for terminal id that no other
// terminal of the canvas has: want (cleaned), else one made from the name,
// numbered when taken (IMP, IM2…IM9, I10…).
func (s *Store) freeCallsignLocked(want, name, id string) string {
	base := ids.CleanCallsign(want)
	if base == "" {
		base = ids.Callsign(name)
	}
	if base == "" {
		base = "VOO"
	}
	if !s.callsignTakenLocked(base, id) {
		return base
	}
	for i := 2; i < 1000; i++ {
		n := strconv.Itoa(i)
		keep := max(1, min(len(base), 3)-len(n))
		if c := base[:keep] + n; !s.callsignTakenLocked(c, id) {
			return c
		}
	}
	return base + ids.Short(2)
}

func (s *Store) callsignTakenLocked(c, except string) bool {
	for _, n := range s.ws.Nodes {
		if n.ID != except && n.Terminal != nil && strings.EqualFold(n.Terminal.Callsign, c) {
			return true
		}
	}
	return false
}

// checkCallsignLocked keeps an update's callsign valid: cleaned, the old one
// when it comes empty, and refused when another terminal has it.
func (s *Store) checkCallsignLocked(n *Node, old Node) error {
	c := ids.CleanCallsign(n.Terminal.Callsign)
	if c == "" && old.Terminal != nil {
		c = old.Terminal.Callsign
	}
	if c == "" {
		c = s.freeCallsignLocked("", n.Terminal.Name, n.ID)
	}
	if old.Terminal == nil || !strings.EqualFold(c, old.Terminal.Callsign) {
		if s.callsignTakenLocked(c, n.ID) {
			return fmt.Errorf("%w: another flight has the callsign %s", ErrDuplicate, c)
		}
	}
	n.Terminal.Callsign = c
	return nil
}

// AssignCallsign gives terminal id a callsign from want (its role's
// abbreviation, say), numbered if taken; it reports the one given.
func (s *Store) AssignCallsign(id, want string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.ws.Nodes {
		n := &s.ws.Nodes[i]
		if n.ID != id || n.Terminal == nil {
			continue
		}
		c := s.freeCallsignLocked(want, n.Terminal.Name, id)
		if c == n.Terminal.Callsign {
			return c, nil
		}
		old := n.Terminal.Callsign
		n.Terminal.Callsign = c
		if err := s.save(); err != nil {
			n.Terminal.Callsign = old
			return "", err
		}
		return c, nil
	}
	return "", ErrNotFound
}

// migrateLocked numbers the floors of a canvas saved before Torre (the
// app gives its terminals their callsigns, from their roles). It reports
// whether anything changed.
func (s *Store) migrateLocked() bool {
	changed := false
	for i := range s.ws.Floors {
		if s.ws.Floors[i].Number == 0 {
			s.ws.FloorSeq++
			s.ws.Floors[i].Number = s.ws.FloorSeq
			changed = true
		} else if s.ws.Floors[i].Number > s.ws.FloorSeq {
			s.ws.FloorSeq = s.ws.Floors[i].Number
			changed = true
		}
	}
	return changed
}

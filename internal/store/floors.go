package store

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/apierr"
)

// Floor statuses: its setup command is running, it is ready, or the setup
// failed (the floor still works; Error says why).
const (
	FloorPreparing = "preparing"
	FloorReady     = "ready"
	FloorFailed    = "failed"
)

// Floor is an isolated copy of the workspace's repository (a git worktree on
// its own branch) with its own canvas: the nodes whose Floor is its id.
// Wires may cross floors.
type Floor struct {
	ID   string `json:"id"`   // slug of the name: "login"
	Name string `json:"name"` // "Login"
	// Number: the flight level, FL1, FL2…, in the order floors were
	// created in this canvas (never given again).
	Number int    `json:"number,omitempty"`
	Branch string `json:"branch"` // the floor's branch
	// Base is the branch the floor started from, where landing merges by
	// default ("" when the ground floor was on a detached HEAD); BaseCommit
	// is the commit.
	Base       string `json:"base,omitempty"`
	BaseCommit string `json:"baseCommit"`
	Path       string `json:"path"` // the worktree folder
	// Cwd is the workspace folder inside the copy: terminals of the floor
	// start there, and their relative folders resolve from it.
	Cwd string `json:"cwd"`
	// NewBranch: the floor created its branch, so discarding it deletes it.
	NewBranch bool `json:"newBranch,omitempty"`
	// Copied are the untracked files copied from the ground floor (paths
	// relative to the worktree, with "/"): landing ignores them.
	Copied    []string  `json:"copied,omitempty"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedBy string    `json:"createdBy,omitempty"` // terminal whose agent created it
	CreatedAt time.Time `json:"createdAt"`
}

// FloorConfig is how new floors of a workspace are seeded, beyond the
// tracked files: untracked files to copy and a setup command.
type FloorConfig struct {
	// Copy are glob patterns, relative to the workspace folder, of files or
	// folders to copy from the ground floor when they aren't tracked
	// (.env, config/local.json…).
	Copy []string `json:"copy"`
	// Setup runs in the new floor's folder after the copy (npm install…).
	Setup string `json:"setup,omitempty"`
}

// DefaultFloorConfig copies the .env files.
func DefaultFloorConfig() FloorConfig { return FloorConfig{Copy: []string{".env*"}} }

func cloneFloor(f Floor) Floor {
	f.Copied = slices.Clone(f.Copied)
	return f
}

// Floors returns the floors in creation order.
func (s *Store) Floors() []Floor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Floor, 0, len(s.ws.Floors))
	for _, f := range s.ws.Floors {
		out = append(out, cloneFloor(f))
	}
	return out
}

// Floor returns a floor by id.
func (s *Store) Floor(id string) (Floor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i := s.floorIndexLocked(id); i >= 0 {
		return cloneFloor(s.ws.Floors[i]), true
	}
	return Floor{}, false
}

func (s *Store) floorIndexLocked(id string) int {
	return slices.IndexFunc(s.ws.Floors, func(f Floor) bool { return f.ID == id })
}

// AddFloor stores a new floor; its id and name must be free.
func (s *Store) AddFloor(f Floor) (Floor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.ID == "" || f.Name == "" {
		return Floor{}, fmt.Errorf("%w: a floor needs an id and a name", ErrInvalid)
	}
	for _, o := range s.ws.Floors {
		if o.ID == f.ID || strings.EqualFold(o.Name, f.Name) {
			return Floor{}, apierr.With(fmt.Errorf("%w: a floor named %q", ErrDuplicate, o.Name), "level.name_taken", "name", o.Name)
		}
	}
	seq := s.ws.FloorSeq
	s.ws.FloorSeq++
	f.Number = s.ws.FloorSeq
	s.ws.Floors = append(s.ws.Floors, cloneFloor(f))
	if err := s.save(); err != nil {
		s.ws.Floors = s.ws.Floors[:len(s.ws.Floors)-1]
		s.ws.FloorSeq = seq
		return Floor{}, err
	}
	return cloneFloor(f), nil
}

// UpdateFloor applies fn to a floor and saves (its id is kept).
func (s *Store) UpdateFloor(id string, fn func(*Floor)) (Floor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.floorIndexLocked(id)
	if i < 0 {
		return Floor{}, ErrNotFound
	}
	old := s.ws.Floors[i]
	f := cloneFloor(old)
	fn(&f)
	f.ID = id
	s.ws.Floors[i] = f
	if err := s.save(); err != nil {
		s.ws.Floors[i] = old
		return Floor{}, err
	}
	return cloneFloor(f), nil
}

// RemoveFloor forgets a floor. Its nodes must be gone (or moved) first.
func (s *Store) RemoveFloor(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.floorIndexLocked(id)
	if i < 0 {
		return ErrNotFound
	}
	for _, n := range s.ws.Nodes {
		if n.Floor == id {
			return fmt.Errorf("%w: the floor still has nodes", ErrInvalid)
		}
	}
	old := slices.Clone(s.ws.Floors)
	s.ws.Floors = slices.Delete(s.ws.Floors, i, i+1)
	if err := s.save(); err != nil {
		s.ws.Floors = old
		return err
	}
	return nil
}

// FloorConfig returns how new floors are seeded.
func (s *Store) FloorConfig() FloorConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ws.FloorConfig == nil {
		return DefaultFloorConfig()
	}
	c := *s.ws.FloorConfig
	c.Copy = slices.Clone(c.Copy)
	return c
}

// SetFloorConfig changes how new floors are seeded.
func (s *Store) SetFloorConfig(c FloorConfig) (FloorConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var clean []string
	for _, p := range c.Copy {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(clean, p) {
			clean = append(clean, p)
		}
	}
	c.Copy = clean
	if c.Copy == nil {
		c.Copy = []string{}
	}
	c.Setup = strings.TrimSpace(c.Setup)
	old := s.ws.FloorConfig
	s.ws.FloorConfig = &c
	if err := s.save(); err != nil {
		s.ws.FloorConfig = old
		return FloorConfig{}, err
	}
	return c, nil
}

// MoveNode puts a node on another floor ("" is the ground floor) at x, y.
func (s *Store) MoveNode(id, floor string, x, y float64) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if floor != "" && s.floorIndexLocked(floor) < 0 {
		return Node{}, fmt.Errorf("%w: floor %q", ErrNotFound, floor)
	}
	for i := range s.ws.Nodes {
		if s.ws.Nodes[i].ID == id {
			old := s.ws.Nodes[i]
			s.ws.Nodes[i].Floor, s.ws.Nodes[i].X, s.ws.Nodes[i].Y = floor, x, y
			if err := s.save(); err != nil {
				s.ws.Nodes[i] = old
				return Node{}, err
			}
			return cloneNode(s.ws.Nodes[i]), nil
		}
	}
	return Node{}, ErrNotFound
}

// IsGround reports whether a floor name means the ground floor.
func IsGround(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ground", "pista", "térreo", "terreo":
		return true
	}
	return false
}

// FindFloor returns a floor by id or name, in any case.
func (s *Store) FindFloor(q string) (Floor, bool) {
	q = strings.TrimSpace(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.ws.Floors {
		if f.ID == q || strings.EqualFold(f.Name, q) {
			return cloneFloor(f), true
		}
	}
	return Floor{}, false
}

// OnFloor returns the nodes of a floor ("" is the ground floor).
func OnFloor(nodes []Node, floor string) []Node {
	var out []Node
	for _, n := range nodes {
		if n.Floor == floor {
			out = append(out, n)
		}
	}
	return out
}

// Package store persists a workspace (canvas nodes and the connections
// between them) as JSON, written atomically after every change.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/ids"
)

// Node kinds.
const (
	KindTerminal = "terminal"
	KindNote     = "note"
	KindPortal   = "portal"
)

// SchemaVersion of workspace.json.
const SchemaVersion = 1

// Workspace is one canvas.
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Cwd       string    `json:"cwd"`
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	CreatedAt time.Time `json:"createdAt"`
}

// Node is an element on the canvas.
type Node struct {
	ID       string        `json:"id"`
	Kind     string        `json:"kind"`
	X        float64       `json:"x"`
	Y        float64       `json:"y"`
	W        float64       `json:"w"`
	H        float64       `json:"h"`
	Terminal *TerminalData `json:"terminal,omitempty"`
	Note     *NoteData     `json:"note,omitempty"`
	Portal   *PortalData   `json:"portal,omitempty"`
}

// Label returns the node's display name.
func (n Node) Label() string {
	switch {
	case n.Terminal != nil:
		return n.Terminal.Name
	case n.Note != nil:
		return n.Note.Title
	case n.Portal != nil:
		return "Portal"
	}
	return n.ID
}

// TerminalData configures a terminal node.
type TerminalData struct {
	Name    string `json:"name"`
	Agent   string `json:"agent"`             // claude | codex | gemini | shell | custom
	Command string `json:"command,omitempty"` // launch line; empty = plain shell
	Shell   string `json:"shell,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Role    string `json:"role,omitempty"` // plan | impl | rev | test | ""
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

// NoteData configures a note node; the content lives in a .md file.
type NoteData struct {
	Title  string `json:"title"`
	File   string `json:"file"`
	Locked bool   `json:"locked,omitempty"`
}

// PortalData configures a portal node.
type PortalData struct {
	URL string `json:"url"`
}

// Edge is an undirected connection between two nodes.
type Edge struct {
	ID         string       `json:"id"`
	A          string       `json:"a"`
	B          string       `json:"b"`
	RopePoints [][2]float64 `json:"ropePoints,omitempty"`
	CreatedAt  time.Time    `json:"createdAt"`
}

// Other returns the endpoint opposite to id.
func (e Edge) Other(id string) string {
	if e.A == id {
		return e.B
	}
	return e.A
}

type envelope struct {
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schemaVersion"`
	Payload       json.RawMessage `json:"payload"`
}

// Errors returned by Store methods.
var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("already exists")
	ErrInvalid   = errors.New("invalid")
)

// Store guards one workspace and its file.
type Store struct {
	mu   sync.RWMutex
	ws   Workspace
	path string
}

// Dir returns the directory holding a workspace's files.
func Dir(wsID string) (string, error) { return config.Dir("workspaces", wsID) }

// OpenDefault loads (or creates) the default workspace under ~/.squad.
func OpenDefault(cwd string) (*Store, error) {
	dir, err := Dir("default")
	if err != nil {
		return nil, err
	}
	return Open(filepath.Join(dir, "workspace.json"), Workspace{
		ID: "default", Name: filepath.Base(cwd), Cwd: cwd, CreatedAt: time.Now().UTC(),
	})
}

// Open loads path, or initializes it with def when it doesn't exist.
func Open(path string, def Workspace) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.ws = def
		if s.ws.Nodes == nil {
			s.ws.Nodes = []Node{}
		}
		if s.ws.Edges == nil {
			s.ws.Edges = []Edge{}
		}
		return s, s.save()
	case err != nil:
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if env.Type != "workspace" || env.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("%s: unsupported %s v%d", path, env.Type, env.SchemaVersion)
	}
	if err := json.Unmarshal(env.Payload, &s.ws); err != nil {
		return nil, fmt.Errorf("parse %s payload: %w", path, err)
	}
	return s, nil
}

// Path returns the workspace file path.
func (s *Store) Path() string { return s.path }

func (s *Store) save() error {
	payload, err := json.Marshal(s.ws)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(envelope{Type: "workspace", SchemaVersion: SchemaVersion, Payload: payload}, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(s.path, data, 0o600)
}

// Snapshot returns a deep copy of the workspace.
func (s *Store) Snapshot() Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out Workspace
	b, _ := json.Marshal(s.ws)
	_ = json.Unmarshal(b, &out)
	return out
}

// Node returns a node by id.
func (s *Store) Node(id string) (Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.ws.Nodes {
		if n.ID == id {
			return cloneNode(n), true
		}
	}
	return Node{}, false
}

// AddNode validates and stores a node, assigning an id if missing.
func (s *Store) AddNode(n Node) (Node, error) {
	if n.ID == "" {
		n.ID = ids.New()
	}
	switch n.Kind {
	case KindTerminal:
		if n.Terminal == nil || n.Terminal.Name == "" {
			return Node{}, fmt.Errorf("%w: terminal needs a name", ErrInvalid)
		}
	case KindNote:
		if n.Note == nil {
			return Node{}, fmt.Errorf("%w: note needs data", ErrInvalid)
		}
	case KindPortal:
		if n.Portal == nil {
			n.Portal = &PortalData{}
		}
	default:
		return Node{}, fmt.Errorf("%w: kind %q", ErrInvalid, n.Kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.ws.Nodes {
		if other.ID == n.ID {
			return Node{}, ErrDuplicate
		}
		if n.Terminal != nil && other.Terminal != nil && other.Terminal.Name == n.Terminal.Name {
			return Node{}, fmt.Errorf("%w: a terminal named %q", ErrDuplicate, n.Terminal.Name)
		}
	}
	s.ws.Nodes = append(s.ws.Nodes, cloneNode(n))
	return cloneNode(n), s.save()
}

// UpdateNode applies fn to the stored node and saves.
func (s *Store) UpdateNode(id string, fn func(*Node) error) (Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.ws.Nodes {
		if s.ws.Nodes[i].ID != id {
			continue
		}
		n := cloneNode(s.ws.Nodes[i]) // fn must not mutate the stored node on failure
		if err := fn(&n); err != nil {
			return Node{}, err
		}
		n.ID, n.Kind = id, s.ws.Nodes[i].Kind
		if n.Terminal != nil {
			for _, other := range s.ws.Nodes {
				if other.ID != id && other.Terminal != nil && other.Terminal.Name == n.Terminal.Name {
					return Node{}, fmt.Errorf("%w: a terminal named %q", ErrDuplicate, n.Terminal.Name)
				}
			}
		}
		s.ws.Nodes[i] = n
		return n, s.save()
	}
	return Node{}, ErrNotFound
}

func cloneNode(n Node) Node {
	if n.Terminal != nil {
		t := *n.Terminal
		n.Terminal = &t
	}
	if n.Note != nil {
		v := *n.Note
		n.Note = &v
	}
	if n.Portal != nil {
		v := *n.Portal
		n.Portal = &v
	}
	return n
}

// RemoveNode deletes a node and every edge touching it; it returns the
// removed edges.
func (s *Store) RemoveNode(id string) ([]Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := slices.IndexFunc(s.ws.Nodes, func(n Node) bool { return n.ID == id })
	if idx < 0 {
		return nil, ErrNotFound
	}
	s.ws.Nodes = slices.Delete(s.ws.Nodes, idx, idx+1)
	var removed []Edge
	kept := s.ws.Edges[:0]
	for _, e := range s.ws.Edges {
		if e.A == id || e.B == id {
			removed = append(removed, e)
		} else {
			kept = append(kept, e)
		}
	}
	s.ws.Edges = kept
	return removed, s.save()
}

// AddEdge connects two existing nodes.
func (s *Store) AddEdge(a, b string, rope [][2]float64) (Edge, error) {
	if a == b {
		return Edge{}, fmt.Errorf("%w: a node can't connect to itself", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	has := func(id string) bool {
		return slices.ContainsFunc(s.ws.Nodes, func(n Node) bool { return n.ID == id })
	}
	if !has(a) || !has(b) {
		return Edge{}, ErrNotFound
	}
	for _, e := range s.ws.Edges {
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			return Edge{}, ErrDuplicate
		}
	}
	e := Edge{ID: ids.New(), A: a, B: b, RopePoints: rope, CreatedAt: time.Now().UTC()}
	s.ws.Edges = append(s.ws.Edges, e)
	return e, s.save()
}

// UpdateEdgeRope replaces an edge's rope points.
func (s *Store) UpdateEdgeRope(id string, rope [][2]float64) (Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.ws.Edges {
		if s.ws.Edges[i].ID == id {
			s.ws.Edges[i].RopePoints = rope
			return s.ws.Edges[i], s.save()
		}
	}
	return Edge{}, ErrNotFound
}

// RemoveEdge deletes an edge.
func (s *Store) RemoveEdge(id string) (Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := slices.IndexFunc(s.ws.Edges, func(e Edge) bool { return e.ID == id })
	if idx < 0 {
		return Edge{}, ErrNotFound
	}
	e := s.ws.Edges[idx]
	s.ws.Edges = slices.Delete(s.ws.Edges, idx, idx+1)
	return e, s.save()
}

// Connected reports whether a and b share an edge.
func (s *Store) Connected(a, b string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.ws.Edges {
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			return true
		}
	}
	return false
}

// EdgeBetween returns the edge connecting a and b.
func (s *Store) EdgeBetween(a, b string) (Edge, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.ws.Edges {
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			return e, true
		}
	}
	return Edge{}, false
}

// Neighbors returns the nodes connected to id.
func (s *Store) Neighbors(id string) []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for _, e := range s.ws.Edges {
		if e.A == id || e.B == id {
			ids = append(ids, e.Other(id))
		}
	}
	var out []Node
	for _, n := range s.ws.Nodes {
		if slices.Contains(ids, n.ID) {
			out = append(out, cloneNode(n))
		}
	}
	return out
}

// TerminalByName finds a terminal node by its name.
func (s *Store) TerminalByName(name string) (Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.ws.Nodes {
		if n.Terminal != nil && n.Terminal.Name == name {
			return cloneNode(n), true
		}
	}
	return Node{}, false
}

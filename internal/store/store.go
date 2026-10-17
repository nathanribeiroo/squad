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
	Color     string    `json:"color,omitempty"` // UI color key: teal, blue, purple…
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	CreatedAt time.Time `json:"createdAt"`
	// Floors are isolated copies of the repository, each with its own
	// canvas; FloorConfig seeds new ones (nil: DefaultFloorConfig).
	Floors      []Floor      `json:"floors,omitempty"`
	FloorConfig *FloorConfig `json:"floorConfig,omitempty"`
	// SummaryConfig: turn cards and who writes summaries (nil: the default).
	SummaryConfig *SummaryConfig `json:"summaryConfig,omitempty"`
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
	// CreatedBy is the terminal whose agent created this node (squad new,
	// squad portal create); empty when the user did.
	CreatedBy string `json:"createdBy,omitempty"`
	// Floor is the floor the node is on (a Floor id); "" is the ground floor.
	Floor string `json:"floor,omitempty"`
}

// Label returns the node's display name.
func (n Node) Label() string {
	switch {
	case n.Terminal != nil:
		return n.Terminal.Name
	case n.Note != nil:
		return n.Note.Title
	case n.Portal != nil:
		if n.Portal.Name != "" {
			return n.Portal.Name
		}
		return "Portal"
	}
	return n.ID
}

// TerminalData configures a terminal node.
type TerminalData struct {
	Name    string `json:"name"`
	Agent   string `json:"agent"`             // claude | codex | devin | gemini | shell | custom
	Command string `json:"command,omitempty"` // launch line; empty = plain shell
	Shell   string `json:"shell,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Role    string `json:"role,omitempty"` // a role of the library: plan | impl | rev | test | …
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	// Manager lets the agent create and connect terminals (squad new); nil
	// follows its role.
	Manager *bool `json:"manager,omitempty"`
	// Conversation is the agent's own id for the conversation the terminal
	// continues when it starts again (claude/devin/gemini --resume, codex
	// resume); empty starts a new one. Squad keeps it (Server.SetConversation).
	Conversation string `json:"conversation,omitempty"`
	// Busy: the agent was in the middle of a turn. A terminal that stops
	// while busy (Squad closed or restarted) resumes as interrupted.
	Busy bool `json:"busy,omitempty"`
	// Empty: the conversation hasn't had a turn yet, so there is nothing to
	// continue (the agent wouldn't find it): the terminal starts a new one.
	Empty bool `json:"empty,omitempty"`
}

// NoteData configures a note node; the content lives in a .md file.
type NoteData struct {
	Title  string `json:"title"`
	File   string `json:"file"` // "" while Draft
	Locked bool   `json:"locked,omitempty"`
	// Draft: a note made in the UI whose name the user hasn't confirmed; its
	// file is created from the name it ends up with.
	Draft bool `json:"draft,omitempty"`
	// Aliases are the note's former titles: agents that knew one still find
	// the note.
	Aliases []string `json:"aliases,omitempty"`
}

// PortalData configures a portal node.
type PortalData struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"` // how agents refer to it: "Portal", "Docs"…
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
	if s.ws, err = parse(path, data); err != nil {
		return nil, err
	}
	return s, nil
}

// Read loads a workspace file without opening it for changes (a session
// that isn't open).
func Read(path string) (Workspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workspace{}, err
	}
	return parse(path, data)
}

func parse(path string, data []byte) (Workspace, error) {
	var env envelope
	var ws Workspace
	if err := json.Unmarshal(data, &env); err != nil {
		return ws, fmt.Errorf("parse %s: %w", path, err)
	}
	if env.Type != "workspace" || env.SchemaVersion > SchemaVersion {
		return ws, fmt.Errorf("%s: unsupported %s v%d", path, env.Type, env.SchemaVersion)
	}
	if err := json.Unmarshal(env.Payload, &ws); err != nil {
		return ws, fmt.Errorf("parse %s payload: %w", path, err)
	}
	return ws, nil
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

// Meta is a workspace without its canvas.
type Meta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Cwd       string    `json:"cwd"`
	Color     string    `json:"color,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) metaLocked() Meta {
	return Meta{ID: s.ws.ID, Name: s.ws.Name, Cwd: s.ws.Cwd, Color: s.ws.Color, CreatedAt: s.ws.CreatedAt}
}

// Meta returns the workspace's id, name, folder and color.
func (s *Store) Meta() Meta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metaLocked()
}

// UpdateMeta changes the workspace's name, folder or color (fn edits a copy;
// the id and creation time are kept) and saves.
func (s *Store) UpdateMeta(fn func(*Meta) error) (Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.metaLocked()
	if err := fn(&m); err != nil {
		return Meta{}, err
	}
	old := s.ws
	s.ws.Name, s.ws.Cwd, s.ws.Color = m.Name, m.Cwd, m.Color
	if err := s.save(); err != nil {
		s.ws = old
		return Meta{}, err
	}
	return s.metaLocked(), nil
}

// Shared is what every session of a workspace has in common (each session
// is a canvas of its own): the workspace's id, name, folder and color, and
// its settings for floors and turn summaries.
type Shared struct {
	Meta
	FloorConfig   *FloorConfig
	SummaryConfig *SummaryConfig
}

// Shared returns what the workspace's sessions have in common.
func (s *Store) Shared() Shared {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Shared{Meta: s.metaLocked(), FloorConfig: cloneFloorConfig(s.ws.FloorConfig), SummaryConfig: cloneSummaryConfig(s.ws.SummaryConfig)}
}

// SetShared makes this canvas take the workspace's shared part (a session
// being opened catches up with changes made while it was closed).
func (s *Store) SetShared(sh Shared) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.ws
	s.ws.ID, s.ws.Name, s.ws.Cwd, s.ws.Color, s.ws.CreatedAt = sh.ID, sh.Name, sh.Cwd, sh.Color, sh.CreatedAt
	s.ws.FloorConfig, s.ws.SummaryConfig = cloneFloorConfig(sh.FloorConfig), cloneSummaryConfig(sh.SummaryConfig)
	if err := s.save(); err != nil {
		s.ws = old
		return err
	}
	return nil
}

func cloneFloorConfig(c *FloorConfig) *FloorConfig {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Copy = append([]string(nil), c.Copy...)
	return &cp
}

func cloneSummaryConfig(c *SummaryConfig) *SummaryConfig {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

// TerminalIDs returns the ids of the terminal nodes.
func (s *Store) TerminalIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, n := range s.ws.Nodes {
		if n.Terminal != nil {
			out = append(out, n.ID)
		}
	}
	return out
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
	if n.Floor != "" && s.floorIndexLocked(n.Floor) < 0 {
		return Node{}, fmt.Errorf("%w: floor %q", ErrNotFound, n.Floor)
	}
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
		n.ID, n.Kind, n.Floor = id, s.ws.Nodes[i].Kind, s.ws.Nodes[i].Floor
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
		if t.Manager != nil {
			m := *t.Manager
			t.Manager = &m
		}
		n.Terminal = &t
	}
	if n.Note != nil {
		v := *n.Note
		v.Aliases = slices.Clone(v.Aliases)
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

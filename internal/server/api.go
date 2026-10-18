package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// TerminalRuntime is the live state of a terminal node, sent with the state
// and in terminal.state events. Activity is filled by the activity monitor.
type TerminalRuntime struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // starting | running | exited | stopped | failed
	ExitCode int    `json:"exitCode"`
	Title    string `json:"title,omitempty"`
	Error    string `json:"error,omitempty"`
	// Activity is the UI state: boot, idle, work, wait, done, err, off.
	Activity      string    `json:"activity"`
	ActivitySince time.Time `json:"activitySince"`
	// Interrupted: the agent continued a conversation it was in the middle
	// of when it stopped (Torre closed or restarted).
	Interrupted bool `json:"interrupted"`
	// Attention: what the flight asks the user (activity "ask").
	Attention *attention.Attention `json:"attention,omitempty"`
	// YourTurn: the flight's last turn was the user's and it ended; the
	// flight waits for them (shown while "done").
	YourTurn bool `json:"yourTurn,omitempty"`
}

// StateResponse is GET /api/state.
type StateResponse struct {
	Workspace store.Workspace            `json:"workspace"`
	Runtime   map[string]TerminalRuntime `json:"runtime"`
	Extra     map[string]any             `json:"extra,omitempty"`
}

func (s *Server) runtimeOf(id string) TerminalRuntime {
	rt := TerminalRuntime{ID: id, Status: "stopped"}
	if t := s.terms.Get(id); t != nil {
		st, code := t.State()
		rt.Status, rt.ExitCode = string(st), code
	}
	s.mu.Lock()
	rt.Title = s.titles[id]
	rt.Interrupted = s.interrupted[id]
	rt.YourTurn = s.yourTurn[id]
	s.mu.Unlock()
	if s.activityOf != nil {
		rt.Activity, rt.ActivitySince = s.activityOf(id)
	}
	if s.attentionOf != nil {
		rt.Attention = s.attentionOf(id)
	}
	return rt
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	ws := s.store.Snapshot()
	resp := StateResponse{Workspace: ws, Runtime: map[string]TerminalRuntime{}}
	for _, n := range ws.Nodes {
		if n.Kind == store.KindTerminal {
			resp.Runtime[n.ID] = s.runtimeOf(n.ID)
		}
	}
	if s.stateExtra != nil {
		resp.Extra = s.stateExtra()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAddNode(w http.ResponseWriter, r *http.Request) {
	var n store.Node
	if err := readJSON(r, &n); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	n.ID, n.CreatedBy = "", "" // assigned by the server
	n, err := s.AddNode(n)
	if err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

// AddNode adds a node as the canvas does: default size, the preparers
// (portal names, note files), then a terminal starts.
func (s *Server) AddNode(n store.Node) (store.Node, error) {
	if err := checkTerminalDir(n.Terminal, s.cwdResolver(n.Floor)); err != nil {
		return store.Node{}, err
	}
	if n.Terminal != nil { // a new terminal starts a new conversation
		t := *n.Terminal
		t.Conversation, t.Busy, t.Empty = "", false, false
		if t.Callsign == "" && s.roleAbbr != nil {
			t.Callsign = s.roleAbbr(t.Role) // numbered by the store when taken
		}
		n.Terminal = &t
	}
	if n.W == 0 && n.H == 0 {
		n.W, n.H = defaultSize(n.Kind)
	}
	if s.prepareNode != nil {
		if err := s.prepareNode(&n); err != nil {
			return store.Node{}, invalidError(err.Error())
		}
	}
	n, err := s.store.AddNode(n)
	if err != nil {
		return store.Node{}, err
	}
	s.bus.Publish(events.NodeAdded, n)
	if n.Kind == store.KindTerminal {
		_ = s.StartTerminal(n.ID) // failures are reported as terminal state
	}
	return n, nil
}

// DefaultSize is the size of a new node of a kind.
func DefaultSize(kind string) (w, h float64) { return defaultSize(kind) }

func defaultSize(kind string) (float64, float64) {
	switch kind {
	case store.KindNote:
		return 400, 350
	case store.KindPortal:
		return 560, 420
	case store.KindGroup:
		return 720, 480
	case store.KindDrawing:
		return 240, 160
	}
	return 560, 440
}

// nodePatch is the body of PATCH /api/nodes/{id}; absent fields are kept.
type nodePatch struct {
	X        *float64            `json:"x"`
	Y        *float64            `json:"y"`
	W        *float64            `json:"w"`
	H        *float64            `json:"h"`
	Terminal *store.TerminalData `json:"terminal"`
	Note     *store.NoteData     `json:"note"`
	Portal   *store.PortalData   `json:"portal"`
	Group    *store.GroupData    `json:"group"`
	Drawing  *store.DrawingData  `json:"drawing"`
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	var p nodePatch
	if err := readJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	id := r.PathValue("id")
	cur, _ := s.store.Node(id)
	resolve := s.cwdResolver(cur.Floor) // read before UpdateNode takes the store lock
	n, err := s.store.UpdateNode(id, func(n *store.Node) error {
		if p.X != nil {
			n.X = *p.X
		}
		if p.Y != nil {
			n.Y = *p.Y
		}
		if p.W != nil {
			n.W = *p.W
		}
		if p.H != nil {
			n.H = *p.H
		}
		if p.Terminal != nil && n.Terminal != nil {
			if p.Terminal.Name == "" {
				return apierr.With(errors.Join(store.ErrInvalid, errors.New("terminal needs a name")), "flight.name_required")
			}
			if p.Terminal.Cwd != n.Terminal.Cwd {
				if err := checkTerminalDir(p.Terminal, resolve); err != nil {
					return err
				}
			}
			old := *n.Terminal
			*n.Terminal = *p.Terminal
			// Torre keeps the conversation; another agent or folder can't
			// continue it (agents keep conversations per folder).
			n.Terminal.Conversation, n.Terminal.Busy, n.Terminal.Empty = old.Conversation, old.Busy, old.Empty
			if p.Terminal.Agent != old.Agent || p.Terminal.Cwd != old.Cwd {
				n.Terminal.Conversation, n.Terminal.Busy, n.Terminal.Empty = "", false, false
			}
		}
		if p.Note != nil && n.Note != nil {
			// Only the lock: the name changes by renaming (notes.Rename,
			// which keeps the old one for agents) and the file never does.
			n.Note.Locked = p.Note.Locked
		}
		if p.Portal != nil && n.Portal != nil {
			// Agents address a portal by its name: changing the address keeps it.
			name := strings.TrimSpace(p.Portal.Name)
			if name == "" {
				name = n.Portal.Name
			}
			*n.Portal = *p.Portal
			n.Portal.Name = name
		}
		if p.Group != nil && n.Group != nil {
			*n.Group = *p.Group
		}
		if p.Drawing != nil && n.Drawing != nil {
			*n.Drawing = *p.Drawing
		}
		return nil
	})
	if err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	// Another role brings its abbreviation, unless the callsign was chosen
	// by hand.
	if n.Terminal != nil && cur.Terminal != nil && s.roleAbbr != nil && n.Terminal.Role != cur.Terminal.Role &&
		n.Terminal.Callsign == cur.Terminal.Callsign && derivedCallsign(cur.Terminal.Callsign, s.roleAbbr(cur.Terminal.Role), cur.Terminal.Name) {
		if c, err := s.store.AssignCallsign(id, s.roleAbbr(n.Terminal.Role)); err == nil {
			n.Terminal.Callsign = c
		}
	}
	// The canvas resizes a terminal by patching its cols×rows (with the node
	// size); a running process follows at once.
	if n.Terminal != nil && p.Terminal != nil && n.Terminal.Cols > 0 && n.Terminal.Rows > 0 {
		if t := s.terms.Get(id); t != nil {
			if c, r := t.Screen().Size(); c != n.Terminal.Cols || r != n.Terminal.Rows {
				_ = t.Resize(n.Terminal.Cols, n.Terminal.Rows)
			}
		}
	}
	s.bus.Publish(events.NodeUpdated, n)
	writeJSON(w, http.StatusOK, n)
}

// maxMoves is how many nodes one POST /api/nodes/move may move.
const maxMoves = 1000

// handleMoveNodes moves several nodes at once (a group with what is inside
// it): {moves: [{id, x, y}]}.
func (s *Server) handleMoveNodes(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Moves []store.Move `json:"moves"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(b.Moves) == 0 || len(b.Moves) > maxMoves {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("move 1 to %d nodes", maxMoves))
		return
	}
	moved, err := s.store.MoveNodes(b.Moves)
	if err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	for _, n := range moved {
		s.bus.Publish(events.NodeUpdated, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": moved})
}

func (s *Server) handleRemoveNode(w http.ResponseWriter, r *http.Request) {
	if err := s.RemoveNode(r.PathValue("id")); err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveNode removes a node with its edges, stopping its terminal. A note
// of a stack leaves it first (the next base gets its wires).
func (s *Server) RemoveNode(id string) error {
	n, ok := s.store.Node(id)
	if !ok {
		return fmt.Errorf("node %w", store.ErrNotFound)
	}
	if n.Note != nil && n.Note.Stack != "" {
		ch, err := s.store.Unstack(id, false, false)
		if err != nil {
			return err
		}
		s.PublishStack(ch)
	}
	removed, err := s.store.RemoveNode(id)
	if err != nil {
		return err
	}
	if n.Kind == store.KindTerminal {
		s.terms.Remove(id)
		s.mu.Lock()
		delete(s.termTokens, id)
		delete(s.titles, id)
		s.mu.Unlock()
	}
	for _, e := range removed {
		if s.onEdgeGone != nil {
			s.onEdgeGone(e)
		}
		s.bus.Publish(events.EdgeRemoved, e)
	}
	if s.onNodeGone != nil {
		s.onNodeGone(n)
	}
	s.bus.Publish(events.NodeRemoved, map[string]string{"id": id})
	return nil
}

// PublishStack announces what stacking or unstacking notes changed.
func (s *Server) PublishStack(ch store.StackChange) {
	for _, e := range ch.Removed {
		if s.onEdgeGone != nil {
			s.onEdgeGone(e)
		}
		s.bus.Publish(events.EdgeRemoved, e)
	}
	for _, e := range ch.Added {
		s.bus.Publish(events.EdgeAdded, e)
	}
	for _, n := range ch.Nodes {
		s.bus.Publish(events.NodeUpdated, n)
	}
}

func (s *Server) handleNodeAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, ok := s.store.Node(id)
	if !ok || n.Kind != store.KindTerminal {
		writeError(w, http.StatusNotFound, "terminal not found")
		return
	}
	var err error
	switch r.PathValue("action") {
	// start and restart rebuild the terminal from the node, so edits to its
	// folder, shell or command take effect.
	case "start":
		if t := s.terms.Get(id); t != nil {
			if st, _ := t.State(); st == terminal.StatusRunning || st == terminal.StatusStarting {
				writeJSON(w, http.StatusOK, s.runtimeOf(id))
				return
			}
		}
		err = s.StartTerminal(id)
	case "stop":
		if t := s.terms.Get(id); t != nil {
			t.Stop()
		}
	case "restart":
		err = s.StartTerminal(id)
	case "new-conversation":
		err = s.StartTerminalFresh(id)
	default:
		writeError(w, http.StatusNotFound, "unknown action")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.runtimeOf(id))
}

type edgeBody struct {
	A          string       `json:"a"`
	B          string       `json:"b"`
	RopePoints [][2]float64 `json:"ropePoints"`
}

func (s *Server) handleAddEdge(w http.ResponseWriter, r *http.Request) {
	var b edgeBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	e, err := s.AddEdge(b.A, b.B, b.RopePoints)
	if err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// AddEdge connects two nodes, as dragging a wire on the canvas does.
func (s *Server) AddEdge(a, b string, rope [][2]float64) (store.Edge, error) {
	e, err := s.store.AddEdge(a, b, rope)
	if err != nil {
		return store.Edge{}, err
	}
	s.bus.Publish(events.EdgeAdded, e)
	return e, nil
}

func (s *Server) handleUpdateEdge(w http.ResponseWriter, r *http.Request) {
	var b edgeBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	e, err := s.store.UpdateEdgeRope(r.PathValue("id"), b.RopePoints)
	if err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	s.bus.Publish(events.EdgeUpdated, e)
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleRemoveEdge(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.RemoveEdge(r.PathValue("id"))
	if err != nil {
		writeErr(w, storeErrorCode(err), err)
		return
	}
	if s.onEdgeGone != nil {
		s.onEdgeGone(e)
	}
	s.bus.Publish(events.EdgeRemoved, e)
	w.WriteHeader(http.StatusNoContent)
}

// derivedCallsign reports whether callsign c came from abbr (a role's
// abbreviation; the name's letters without one), as given or numbered
// (IMP, IM2, I10).
func derivedCallsign(c, abbr, name string) bool {
	base := abbr
	if base == "" {
		base = ids.Callsign(name)
	}
	if c == base {
		return true
	}
	stem := strings.TrimRight(c, "0123456789")
	return stem != c && strings.HasPrefix(base, stem)
}

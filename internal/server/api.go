package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
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
	s.mu.Unlock()
	if s.activityOf != nil {
		rt.Activity, rt.ActivitySince = s.activityOf(id)
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n.ID = "" // ids are assigned by the server
	if n.W == 0 && n.H == 0 {
		n.W, n.H = defaultSize(n.Kind)
	}
	n, err := s.store.AddNode(n)
	if err != nil {
		writeError(w, storeErrorCode(err), err.Error())
		return
	}
	s.bus.Publish(events.NodeAdded, n)
	if n.Kind == store.KindTerminal {
		_ = s.StartTerminal(n.ID) // failures are reported as terminal state
	}
	writeJSON(w, http.StatusCreated, n)
}

func defaultSize(kind string) (float64, float64) {
	switch kind {
	case store.KindNote:
		return 400, 350
	case store.KindPortal:
		return 560, 420
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
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	var p nodePatch
	if err := readJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
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
				return errors.Join(store.ErrInvalid, errors.New("terminal needs a name"))
			}
			*n.Terminal = *p.Terminal
		}
		if p.Note != nil && n.Note != nil {
			*n.Note = *p.Note
		}
		if p.Portal != nil && n.Portal != nil {
			*n.Portal = *p.Portal
		}
		return nil
	})
	if err != nil {
		writeError(w, storeErrorCode(err), err.Error())
		return
	}
	s.bus.Publish(events.NodeUpdated, n)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleRemoveNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, ok := s.store.Node(id)
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	removed, err := s.store.RemoveNode(id)
	if err != nil {
		writeError(w, storeErrorCode(err), err.Error())
		return
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
	w.WriteHeader(http.StatusNoContent)
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
	case "start":
		if t := s.terms.Get(id); t != nil {
			if st, _ := t.State(); st == terminal.StatusRunning || st == terminal.StatusStarting {
				writeJSON(w, http.StatusOK, s.runtimeOf(id))
				return
			}
			err = s.terms.Restart(id)
		} else {
			err = s.StartTerminal(id)
		}
	case "stop":
		if t := s.terms.Get(id); t != nil {
			t.Stop()
		}
	case "restart":
		if s.terms.Get(id) != nil {
			err = s.terms.Restart(id)
		} else {
			err = s.StartTerminal(id)
		}
	default:
		writeError(w, http.StatusNotFound, "unknown action")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := s.store.AddEdge(b.A, b.B, b.RopePoints)
	if err != nil {
		writeError(w, storeErrorCode(err), err.Error())
		return
	}
	s.bus.Publish(events.EdgeAdded, e)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleUpdateEdge(w http.ResponseWriter, r *http.Request) {
	var b edgeBody
	if err := readJSON(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := s.store.UpdateEdgeRope(r.PathValue("id"), b.RopePoints)
	if err != nil {
		writeError(w, storeErrorCode(err), err.Error())
		return
	}
	s.bus.Publish(events.EdgeUpdated, e)
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleRemoveEdge(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.RemoveEdge(r.PathValue("id"))
	if err != nil {
		writeError(w, storeErrorCode(err), err.Error())
		return
	}
	if s.onEdgeGone != nil {
		s.onEdgeGone(e)
	}
	s.bus.Publish(events.EdgeRemoved, e)
	w.WriteHeader(http.StatusNoContent)
}

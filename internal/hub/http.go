package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/floors"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/webui"
)

// Handler returns the root HTTP handler:
//
//	/w/<id>/api/…, /w/<id>/ws/…   a workspace's API (see server.APIHandler)
//	/api/workspaces…, /api/dirs     the workspace list (and folder suggestions)
//	/api/workspaces/{id}/sessions…  a workspace's sessions: list, new (in a copy, optionally), open, rename, remove
//	/api/roles…                     the role library, shared by every workspace
//	/ws/workspaces                  the list and the roles, live
//	POST /cli, POST /hooks/{agent}  calls from terminals, routed by terminal id
//	/                               the web UI
func (h *Hub) Handler() http.Handler {
	sess := func(fn http.HandlerFunc) http.Handler { return server.Session(h.opts.Token, fn) }
	mux := http.NewServeMux()
	mux.Handle("GET /api/workspaces", sess(h.handleList))
	mux.Handle("POST /api/workspaces", sess(h.handleCreate))
	mux.Handle("POST /api/workspaces/open", sess(h.handleOpen))
	mux.Handle("PATCH /api/workspaces/{id}", sess(h.handleUpdate))
	mux.Handle("DELETE /api/workspaces/{id}", sess(h.handleRemove))
	mux.Handle("GET /api/workspaces/{id}/sessions", sess(h.handleSessions))
	mux.Handle("POST /api/workspaces/{id}/sessions", sess(h.handleNewSession))
	mux.Handle("POST /api/workspaces/{id}/sessions/{sid}/open", sess(h.handleOpenSession))
	mux.Handle("PATCH /api/workspaces/{id}/sessions/{sid}", sess(h.handleRenameSession))
	mux.Handle("DELETE /api/workspaces/{id}/sessions/{sid}", sess(h.handleRemoveSession))
	mux.Handle("GET /api/dirs", sess(h.handleDirs))
	mux.Handle("GET /api/roles", sess(h.handleRolesList))
	mux.Handle("POST /api/roles", sess(h.handleRoleCreate))
	mux.Handle("PUT /api/roles/{id}", sess(h.handleRoleUpdate))
	mux.Handle("POST /api/roles/{id}/reset", sess(h.handleRoleReset))
	mux.Handle("DELETE /api/roles/{id}", sess(h.handleRoleDelete))
	mux.Handle("GET /ws/workspaces", sess(h.handleWorkspacesWS))
	mux.Handle("/api/", sess(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found (a workspace's API is under /w/<id>/api/…)")
	}))
	mux.Handle("/w/", sess(h.handleWorkspace))
	mux.HandleFunc("POST /cli", h.handleTerminal)
	mux.HandleFunc("POST /hooks/{agent}", h.handleTerminal)
	mux.Handle("/", server.UI(h.opts.Token, webui.Handler()))
	return server.Guard(h.opts.Port, mux)
}

// handleWorkspace serves /w/<id>/… with that workspace's API.
func (h *Hub) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	id, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/w/"), "/")
	ws := h.get(id)
	if ws == nil {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path, r2.URL.RawPath = "/"+rest, ""
	ws.api.ServeHTTP(w, r2)
}

// handleTerminal sends a call from inside a terminal to its workspace, which
// checks the terminal's token.
func (h *Hub) handleTerminal(w http.ResponseWriter, r *http.Request) {
	ws := h.byTerminal(r.Header.Get("X-Terminal-ID"))
	if ws == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("torre: this command only works inside a Torre terminal (missing or invalid terminal credentials)\n"))
		return
	}
	ws.term.ServeHTTP(w, r)
}

func (h *Hub) handleList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.List())
}

type createBody struct {
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Color   string `json:"color"`
	Session string `json:"session"` // the first session's name
}

func (h *Hub) handleCreate(w http.ResponseWriter, r *http.Request) {
	var b createBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s, err := h.Create(b.Name, b.Cwd, b.Color, b.Session)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *Hub) handleOpen(w http.ResponseWriter, r *http.Request) {
	var b createBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s, err := h.Open(b.Cwd)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Hub) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var p Patch
	if err := readJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s, err := h.Update(r.PathValue("id"), p)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Hub) handleRemove(w http.ResponseWriter, r *http.Request) {
	if err := h.Remove(r.PathValue("id")); err != nil {
		writeHubError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Hub) handleSessions(w http.ResponseWriter, r *http.Request) {
	l, err := h.Sessions(r.PathValue("id"))
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Hub) handleNewSession(w http.ResponseWriter, r *http.Request) {
	var b NewSession
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	l, err := h.NewSession(r.PathValue("id"), b)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *Hub) handleOpenSession(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Confirm bool `json:"confirm"`
	}
	_ = readJSON(r, &b) // an empty body doesn't confirm
	l, err := h.OpenSession(r.PathValue("id"), r.PathValue("sid"), b.Confirm)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Hub) handleRenameSession(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	l, err := h.RenameSession(r.PathValue("id"), r.PathValue("sid"), b.Name)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Hub) handleRemoveSession(w http.ResponseWriter, r *http.Request) {
	confirm := r.URL.Query().Get("confirm") == "true" // uncommitted changes in its copy are lost
	l, err := h.RemoveSession(r.PathValue("id"), r.PathValue("sid"), confirm)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// handleDirs suggests folders for a new workspace (relative to the home
// folder; a workspace's own /api/dirs is relative to its folder).
func (h *Hub) handleDirs(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	writeJSON(w, http.StatusOK, map[string][]string{"suggestions": server.DirSuggestions(r.URL.Query().Get("path"), home)})
}

// handleWorkspacesWS streams the workspace list and the role library: first
// the current ones, then every change, as {"type":"workspaces","data":[…]}
// and {"type":"roles","data":[…]}.
func (h *Hub) handleWorkspacesWS(w http.ResponseWriter, r *http.Request) {
	port := strconv.Itoa(h.opts.Port)
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"127.0.0.1:" + port, "localhost:" + port}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	ch, cancel := h.events.Subscribe()
	defer cancel()
	ctx := c.CloseRead(r.Context())
	if err := writeWS(ctx, c, events.Event{Type: EventWorkspaces, Time: time.Now().UTC(), Data: h.List()}); err != nil {
		return
	}
	if err := writeWS(ctx, c, events.Event{Type: EventRoles, Time: time.Now().UTC(), Data: h.roles.List()}); err != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				c.Close(websocket.StatusTryAgainLater, "too slow")
				return
			}
			if err := writeWS(ctx, c, ev); err != nil {
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

func writeWS(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, b)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // messages like "/w/<id>/api/…" stay readable
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeErr answers err, with its code for the interface (apierr).
func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, apierr.Body(err))
}

func writeHubError(w http.ResponseWriter, err error) {
	var busy *BusyError
	var dirty *DirtyCopyError
	switch {
	case errors.As(err, &busy):
		body := apierr.Body(err)
		body["working"], body["pending"] = busy.Working, busy.Pending
		writeJSON(w, http.StatusConflict, body)
	case errors.As(err, &dirty):
		body := apierr.Body(err)
		body["files"] = dirty.Files
		writeJSON(w, http.StatusConflict, body)
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case errors.Is(err, ErrInvalid), errors.Is(err, floors.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err)
	default:
		writeErr(w, http.StatusInternalServerError, err)
	}
}

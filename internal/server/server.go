// Package server is Squad's local HTTP server: the REST API for the canvas,
// WebSockets for events and terminal I/O, the endpoint used by the `squad`
// CLI from inside terminals, and the embedded web UI. It only listens on
// loopback and checks Host/Origin to defend against DNS rebinding.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
	"github.com/nathanribeiroo/squad/internal/webui"
)

// SessionCookie holds the browser session token.
const SessionCookie = "squad_session"

// Options configure a Server.
type Options struct {
	Port    int          // loopback port the server listens on
	Token   string       // session token for the browser and admin clients
	Store   *store.Store // the workspace
	CLIPath string       // absolute path of the squad binary (put on terminals' PATH)
	Logger  *slog.Logger
}

// Server wires the store, terminals and event bus to HTTP.
type Server struct {
	opts  Options
	log   *slog.Logger
	store *store.Store
	bus   *events.Bus
	terms *terminal.Manager

	mu         sync.Mutex
	termTokens map[string]string // terminal id → token for CLI calls
	titles     map[string]string

	// cliHandler and hookHandler are installed by later stages (agent bus,
	// adapters); until then the endpoints answer 501.
	cliHandler   http.Handler
	hookHandler  http.Handler
	onEdgeGone   func(store.Edge)
	onNodeGone   func(store.Node)
	termListener func(terminal.Event)
	launchHook   LaunchHook
	stateExtra   func() map[string]any
	activityOf   func(id string) (string, time.Time)
	extraRoutes  []route
	prepareNode  func(*store.Node) error
}

// SetNodePreparer installs a function that completes new nodes before they
// are stored (e.g. the notes service assigns a file to a new note).
func (s *Server) SetNodePreparer(fn func(*store.Node) error) { s.prepareNode = fn }

type route struct {
	pattern string
	handler http.Handler
}

// HandleSession registers an extra route behind session auth; call before
// Handler.
func (s *Server) HandleSession(pattern string, h http.Handler) {
	s.extraRoutes = append(s.extraRoutes, route{pattern, h})
}

// SetActivity installs the function giving a terminal's UI state.
func (s *Server) SetActivity(fn func(id string) (string, time.Time)) { s.activityOf = fn }

// PublishTerminalState emits terminal.state for id with its current runtime.
func (s *Server) PublishTerminalState(id string) {
	s.bus.Publish(events.TerminalState, s.runtimeOf(id))
}

// New creates a server; call StartTerminals and serve Handler.
func New(opts Options) (*Server, error) {
	if opts.Store == nil || opts.Token == "" || opts.Port <= 0 {
		return nil, errors.New("server: store, token and port are required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{
		opts: opts, log: opts.Logger, store: opts.Store, bus: events.NewBus(),
		termTokens: map[string]string{}, titles: map[string]string{},
	}
	cliDir := ""
	if opts.CLIPath != "" {
		cliDir = dirOf(opts.CLIPath)
	}
	s.terms = terminal.NewManager(cliDir, s.onTerminalEvent)
	return s, nil
}

// Bus returns the event bus.
func (s *Server) Bus() *events.Bus { return s.bus }

// Terminals returns the terminal manager.
func (s *Server) Terminals() *terminal.Manager { return s.terms }

// Store returns the workspace store.
func (s *Server) Store() *store.Store { return s.store }

// SetCLIHandler installs the handler for POST /cli.
func (s *Server) SetCLIHandler(h http.Handler) { s.cliHandler = h }

// SetHookHandler installs the handler for POST /hooks/{agent}.
func (s *Server) SetHookHandler(h http.Handler) { s.hookHandler = h }

// OnEdgeRemoved registers a callback for removed edges (cancel requests).
func (s *Server) OnEdgeRemoved(fn func(store.Edge)) { s.onEdgeGone = fn }

// OnNodeRemoved registers a callback for removed nodes.
func (s *Server) OnNodeRemoved(fn func(store.Node)) { s.onNodeGone = fn }

// SetStateExtra lets later stages add fields to /api/state and the events
// hello message (activity, pending requests, limits).
func (s *Server) SetStateExtra(fn func() map[string]any) { s.stateExtra = fn }

// OnTerminalEvent registers an extra listener for terminal events.
func (s *Server) OnTerminalEvent(fn func(terminal.Event)) { s.termListener = fn }

// URL returns the base URL of the server.
func (s *Server) URL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.opts.Port) }

// Handler returns the root HTTP handler of a single-workspace server, with
// the security checks applied (the hub mounts APIHandler and TerminalHandler
// itself to serve several workspaces).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := Session(s.opts.Token, s.APIHandler())
	mux.Handle("/api/", api)
	mux.Handle("/ws/", api)
	term := s.TerminalHandler()
	mux.Handle("POST /cli", term)
	mux.Handle("POST /hooks/{agent}", term)
	mux.Handle("/", UI(s.opts.Token, webui.Handler()))
	return Guard(s.opts.Port, mux)
}

// APIHandler serves the workspace's browser API (/api/…, /ws/…). It has no
// session check of its own: wrap it with Session.
func (s *Server) APIHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/nodes", s.handleAddNode)
	mux.HandleFunc("PATCH /api/nodes/{id}", s.handleUpdateNode)
	mux.HandleFunc("DELETE /api/nodes/{id}", s.handleRemoveNode)
	mux.HandleFunc("POST /api/nodes/{id}/{action}", s.handleNodeAction)
	mux.HandleFunc("POST /api/edges", s.handleAddEdge)
	mux.HandleFunc("PATCH /api/edges/{id}", s.handleUpdateEdge)
	mux.HandleFunc("DELETE /api/edges/{id}", s.handleRemoveEdge)
	mux.HandleFunc("GET /api/dirs", s.handleDirs)
	mux.HandleFunc("GET /api/shells", s.handleShells)
	mux.HandleFunc("GET /ws/events", s.handleEventsWS)
	mux.HandleFunc("GET /ws/term/{id}", s.handleTermWS)
	for _, r := range s.extraRoutes {
		mux.Handle(r.pattern, r.handler)
	}
	return mux
}

// TerminalHandler serves the endpoints called from inside the workspace's
// terminals (POST /cli, POST /hooks/{agent}), authenticated per terminal.
func (s *Server) TerminalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /cli", s.terminalAuth(http.HandlerFunc(s.handleCLI)))
	mux.Handle("POST /hooks/{agent}", s.terminalAuth(http.HandlerFunc(s.handleHook)))
	return mux
}

// HasTerminal reports whether id is one of this workspace's terminals.
func (s *Server) HasTerminal(id string) bool {
	n, ok := s.store.Node(id)
	return ok && n.Terminal != nil
}

// Guard rejects requests whose Host isn't this loopback server on port and
// cross-origin state-changing requests.
func Guard(port int, next http.Handler) http.Handler {
	p := strconv.Itoa(port)
	hosts := map[string]bool{"127.0.0.1:" + p: true, "localhost:" + p: true, "[::1]:" + p: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !hosts[u.Host] || u.Scheme != "http" {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// ValidSession reports whether r carries the session token (cookie or
// Authorization: Bearer).
func ValidSession(r *http.Request, token string) bool {
	tok := ""
	if c, err := r.Cookie(SessionCookie); err == nil {
		tok = c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok = strings.TrimPrefix(h, "Bearer ")
	}
	return tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(token)) == 1
}

// Session answers 401 to requests without a valid session.
func Session(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ValidSession(r, token) {
			writeError(w, http.StatusUnauthorized, "missing or invalid session")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UI accepts ?token= once, sets the session cookie and redirects to the URL
// without it (other parameters, like ?ws=, are kept); without a session it
// shows a hint instead of the app.
func UI(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Registered as "/" (a "GET /" pattern would clash with "/api/").
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		if tok := q.Get("token"); tok != "" {
			if subtle.ConstantTimeCompare([]byte(tok), []byte(token)) != 1 {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: SessionCookie, Value: token, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			q.Del("token")
			dest := r.URL.Path
			if len(q) > 0 {
				dest += "?" + q.Encode()
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
			return
		}
		if !ValidSession(r, token) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>Squad</title>`+
				`<p style="font:14px system-ui;margin:20vh auto;max-width:520px">Abra o Squad pela URL com token que o comando <code>squad serve</code> mostrou no terminal.</p>`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ctxKey int

const callerKey ctxKey = 0

// terminalAuth authenticates calls from inside a terminal: X-Terminal-ID
// plus that terminal's X-Squad-Token.
func (s *Server) terminalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Terminal-ID")
		tok := r.Header.Get("X-Squad-Token")
		s.mu.Lock()
		want, ok := s.termTokens[id]
		s.mu.Unlock()
		if id == "" || !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
			writeText(w, http.StatusUnauthorized, "squad: this command only works inside a Squad terminal (missing or invalid terminal credentials)")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey, id)))
	})
}

// CallerID returns the authenticated terminal id of a /cli or /hooks request.
func CallerID(r *http.Request) string {
	id, _ := r.Context().Value(callerKey).(string)
	return id
}

func (s *Server) handleCLI(w http.ResponseWriter, r *http.Request) {
	if s.cliHandler == nil {
		writeText(w, http.StatusNotImplemented, "squad: CLI not available yet")
		return
	}
	s.cliHandler.ServeHTTP(w, r)
}

func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	if s.hookHandler == nil {
		writeText(w, http.StatusNotImplemented, "squad: hooks not available yet")
		return
	}
	s.hookHandler.ServeHTTP(w, r)
}

// Shutdown stops every terminal.
func (s *Server) Shutdown() { s.terms.Shutdown() }

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeText(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	if msg != "" && !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	_, _ = w.Write([]byte(msg))
}

// WriteText is exported for handlers installed from other packages.
func WriteText(w http.ResponseWriter, code int, msg string) { writeText(w, code, msg) }

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}

func storeErrorCode(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrDuplicate):
		return http.StatusConflict
	case errors.Is(err, store.ErrInvalid):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func dirOf(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[:i]
	}
	return ""
}

// ListenLoopback listens on 127.0.0.1:port (port 0 picks a free one).
func ListenLoopback(port int) (net.Listener, int, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, 0, err
	}
	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

// newToken is used for per-terminal credentials.
func newToken() string { return ids.Token() }

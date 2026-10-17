// Package server is Torre's local HTTP server: the REST API for the canvas,
// WebSockets for events and terminal I/O, the endpoint used by the `torre`
// CLI from inside terminals, and the embedded web UI. It only listens on
// loopback and checks Host/Origin to defend against DNS rebinding.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
	"github.com/nathanribeiroo/squad/internal/webui"
)

// SessionCookie holds the browser session token. LegacySessionCookie is
// its name before Squad became Torre: still accepted, so nobody has to log
// in again (the UI replaces it with the new one).
const (
	SessionCookie       = "torre_session"
	LegacySessionCookie = "squad_session"
)

// Options configure a Server.
type Options struct {
	Port    int          // loopback port the server listens on
	Token   string       // session token for the browser and admin clients
	Store   *store.Store // the workspace
	CLIPath string       // absolute path of the torre binary (put on terminals' PATH)
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
	// interrupted: terminals that continued a conversation the agent was in
	// the middle of (it stopped busy); resuming: the conversation each one
	// asked to continue, until its session start says whether it did.
	interrupted map[string]bool
	resuming    map[string]string

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
	attentionOf  func(id string) *attention.Attention
	extraRoutes  []route
	prepareNode  func(*store.Node) error
	roleAbbr     func(role string) string
}

// SetNodePreparer installs a function that completes new nodes before they
// are stored (e.g. the notes service assigns a file to a new note).
func (s *Server) SetNodePreparer(fn func(*store.Node) error) { s.prepareNode = fn }

// SetRoleAbbr tells the server each role's abbreviation, which gives the
// terminals with that role their callsigns. It also gives one to each
// terminal saved without it.
func (s *Server) SetRoleAbbr(fn func(role string) string) {
	s.roleAbbr = fn
	for _, n := range s.store.Snapshot().Nodes {
		if n.Terminal != nil && n.Terminal.Callsign == "" {
			if _, err := s.store.AssignCallsign(n.ID, fn(n.Terminal.Role)); err != nil {
				s.log.Warn("callsign", "terminal", n.Terminal.Name, "err", err)
			}
		}
	}
}

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

// SetAttention installs the function giving what a terminal asks the user.
func (s *Server) SetAttention(fn func(id string) *attention.Attention) { s.attentionOf = fn }

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
		interrupted: map[string]bool{}, resuming: map[string]string{},
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
	// Only the interface answers a prompt (never /cli nor MCP).
	mux.HandleFunc("POST /api/nodes/{id}/attention/respond", s.handleRespond)
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
	} else if c, err := r.Cookie(LegacySessionCookie); err == nil {
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

// SessionMaxAge keeps the session cookie for a year: the token survives
// restarts (config.SessionToken), so an app window stays logged in.
const SessionMaxAge = 365 * 24 * 60 * 60

// PublicFiles are served without a session: the icons and the web app
// manifest, which browsers fetch without cookies.
var PublicFiles = map[string]bool{
	"/favicon.svg": true, "/icon-192.png": true, "/icon-512.png": true,
	"/apple-touch-icon.png": true, "/manifest.webmanifest": true,
}

// UI accepts ?token= once, sets the session cookie and redirects to the URL
// without it (other parameters, like ?ws=, are kept); without a session it
// shows a page to paste the URL that `torre serve` printed.
func UI(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Registered as "/" (a "GET /" pattern would clash with "/api/").
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if PublicFiles[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		q := r.URL.Query()
		if tok := q.Get("token"); tok != "" {
			if subtle.ConstantTimeCompare([]byte(tok), []byte(token)) != 1 {
				loginPage(w, "Esse link é de uma sessão antiga ou está incompleto.")
				return
			}
			setSessionCookie(w, token)
			q.Del("token")
			dest := r.URL.Path
			if len(q) > 0 {
				dest += "?" + q.Encode()
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
			return
		}
		if !ValidSession(r, token) {
			loginPage(w, "")
			return
		}
		if _, err := r.Cookie(SessionCookie); err != nil {
			setSessionCookie(w, token) // logged in with the cookie from before Torre
			http.SetCookie(w, &http.Cookie{Name: LegacySessionCookie, Path: "/", MaxAge: -1})
		}
		next.ServeHTTP(w, r)
	})
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", MaxAge: SessionMaxAge,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

// loginPage asks for the URL with the token. An app window has no address
// bar, so the page takes it in a field instead.
func loginPage(w http.ResponseWriter, problem string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	if problem != "" {
		problem = `<p class="err">` + html.EscapeString(problem) + `</p>`
	}
	fmt.Fprint(w, strings.Replace(loginHTML, "{{problem}}", problem, 1))
}

const loginHTML = `<!doctype html><html lang="pt-BR" translate="no"><meta charset="utf-8"><title>Torre</title>
<meta name="viewport" content="width=device-width, initial-scale=1"><link rel="icon" href="/favicon.svg">
<style>
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#030705;color:#cfeedd;font:14px/1.5 system-ui,sans-serif;overflow:hidden}
.radar{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;pointer-events:none}
.radar svg{width:min(130vh,130vw);height:auto}
main{position:relative;width:min(520px,calc(100vw - 32px))}
.steps{display:flex;gap:18px;font:700 10.5px/1 ui-monospace,monospace;letter-spacing:.12em;color:#3f6b52;margin:0 0 14px}
.steps .on{color:#cfeedd}
.card{background:#07100b;border:1.5px solid #2c5a3d;padding:22px 24px 20px}
h1{display:flex;align-items:center;gap:10px;font:700 15px/1 system-ui,sans-serif;letter-spacing:.14em;margin:0 0 6px}
h2{font:700 12px/1 system-ui,sans-serif;letter-spacing:.12em;text-transform:uppercase;color:#9fc4ae;margin:18px 0 10px}
p{color:#9fc4ae;margin:0 0 14px}code{color:#cfeedd;font-family:ui-monospace,monospace}.err{color:#ffd27a;border-left:3px solid #e0a526;background:#1a1407;padding:8px 10px}
form{display:flex;gap:8px}
input{flex:1;min-width:0;height:36px;padding:0 10px;border-radius:0;border:1px solid #2c5a3d;background:#030705;color:inherit;font:13px ui-monospace,monospace}
input:focus{outline:none;border-color:#cfeedd}
button{height:36px;padding:0 18px;border-radius:0;border:0;background:#cfeedd;color:#030705;font:700 12px system-ui,sans-serif;letter-spacing:.1em;text-transform:uppercase;cursor:pointer}
.foot{margin:14px 0 0;font:11px ui-monospace,monospace;color:#3f6b52}
</style>
<div class="radar" aria-hidden="true"><svg viewBox="0 0 600 600"><g fill="none" stroke="#0e2015"><circle cx="300" cy="300" r="290"/><circle cx="300" cy="300" r="218"/><circle cx="300" cy="300" r="145"/><circle cx="300" cy="300" r="72"/><path d="M10 300H590M300 10V590"/></g></svg></div>
<main><div class="steps"><span class="on">1 ENLACE</span><span>2 PASTA</span><span>3 AGENTES</span></div>
<div class="card"><h1><svg width="16" height="20" viewBox="0 0 56 64" aria-hidden="true"><path d="M20 62 L24 26 H32 L36 62 Z" fill="#cfeedd"/><path d="M10 26 H46 L40 12 H16 Z" fill="#cfeedd"/><path d="M18 14 H38 L35 22 H21 Z" fill="#7fe0a0"/><circle cx="28" cy="4" r="3" fill="#e05a2b"/></svg>TORRE</h1>
<p>A torre de controle dos seus agentes de código.</p>{{problem}}
<h2>Enlace</h2>
<p>Rode <code>torre serve</code> no terminal: ele abre a Torre já com o enlace. Ou cole aqui a URL que ele mostrou.</p>
<form id="f"><input id="u" placeholder="http://127.0.0.1:…/?token=…" autofocus aria-label="URL do torre serve"><button>Entrar</button></form>
<p class="foot">só 127.0.0.1 · o token fica num cookie</p></div></main>
<script>
document.getElementById('f').onsubmit = function (e) {
  e.preventDefault();
  var v = document.getElementById('u').value.trim(), q = new URLSearchParams();
  try { var u = new URL(v); q.set('token', u.searchParams.get('token') || ''); if (u.searchParams.get('ws')) q.set('ws', u.searchParams.get('ws')); }
  catch (_) { q.set('token', v); }
  if (q.get('token')) location.href = '/?' + q.toString();
};
</script></html>`

type ctxKey int

const callerKey ctxKey = 0

// terminalAuth authenticates calls from inside a terminal: X-Terminal-ID
// plus that terminal's X-Torre-Token.
func (s *Server) terminalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Terminal-ID")
		tok := r.Header.Get("X-Torre-Token")
		if tok == "" {
			tok = r.Header.Get("X-Squad-Token") // a squad binary from before the rename
		}
		s.mu.Lock()
		want, ok := s.termTokens[id]
		s.mu.Unlock()
		if id == "" || !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
			writeText(w, http.StatusUnauthorized, "torre: this command only works inside a Torre terminal (missing or invalid terminal credentials)")
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
		writeText(w, http.StatusNotImplemented, "torre: CLI not available yet")
		return
	}
	s.cliHandler.ServeHTTP(w, r)
}

func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	if s.hookHandler == nil {
		writeText(w, http.StatusNotImplemented, "torre: hooks not available yet")
		return
	}
	s.hookHandler.ServeHTTP(w, r)
}

// Shutdown stops every terminal and ends the event stream.
func (s *Server) Shutdown() {
	s.terms.Shutdown()
	s.bus.Close() // its event sockets end: the UI reconnects to what replaced it
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeErr answers err, with its code for the interface (apierr).
func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, apierr.Body(err))
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

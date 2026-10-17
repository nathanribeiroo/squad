// Package hub runs several workspaces in one Torre server. Each workspace is
// a complete app (canvas, terminals, agent bus, notes, activity monitor),
// mounted under /w/<id>/; the hub routes the calls made from inside
// terminals (/cli, /hooks) to the workspace that owns the terminal, and keeps
// the workspace list with a live summary for the selector in the UI.
package hub

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/app"
	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/plans"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/summary"
)

// Colors are the workspace color keys the UI knows; a new workspace takes
// the first one not in use.
var Colors = []string{"teal", "blue", "purple", "pink", "red", "amber", "lime", "green"}

// EventWorkspaces is the hub event carrying the whole list ([]Summary).
const EventWorkspaces = "workspaces"

// EventRoles carries the whole role library ([]roles.Role) after a change.
const EventRoles = "roles"

// summaryDelay coalesces bursts of workspace events into one list update.
const summaryDelay = 300 * time.Millisecond

// Errors returned by Hub methods.
var (
	ErrNotFound = errors.New("workspace not found")
	ErrInvalid  = errors.New("invalid workspace") // matched by every validation error
)

// invalidError is a validation error shown as is (it matches ErrInvalid).
type invalidError string

func (e invalidError) Error() string        { return string(e) }
func (e invalidError) Is(target error) bool { return target == ErrInvalid }

func invalidf(format string, a ...any) error { return invalidError(fmt.Sprintf(format, a...)) }

// Options configure a Hub.
type Options struct {
	Port       int    // loopback port (terminals reach the server through it)
	Token      string // browser session token
	CLIPath    string // torre binary, put on terminals' PATH
	Logger     *slog.Logger
	Root       string // one subfolder per workspace (~/.torre/workspaces)
	Trash      string // removed workspaces are moved here (~/.torre/trash)
	Activity   activity.Options
	Limits     agentbus.Limits
	Extensions []agentbus.Extension
	Setup      func(*app.App) error // runs on every workspace's app before it starts
	Browser    *browser.Manager     // the portals' browser, shared by all workspaces (nil: no portals)
	Roles      *roles.Library       // nil: roles.json next to Root (~/.torre/roles.json)
	Plans      *plans.Library       // nil: plans/ next to Root (~/.torre/plans)
}

// Summary is a workspace as the selector shows it.
type Summary struct {
	store.Meta
	Terminals int     `json:"terminals"`
	Active    int     `json:"active"`    // agents working or waiting for a reply
	Pending   int     `json:"pending"`   // requests waiting for a reply
	Attention int     `json:"attention"` // flights asking the user (PRIORIDADE)
	Session   Session `json:"session"`   // the open session
	Sessions  int     `json:"sessions"`  // how many it has
}

// workspace is a workspace with its open session's app; opening another
// session replaces it (sessions.go).
type workspace struct {
	app   *app.App
	api   http.Handler // /api/…, /ws/… of this workspace
	term  http.Handler // /cli, /hooks/… of this workspace
	unsub func()
	s     *sessions
}

// Hub owns the workspaces.
type Hub struct {
	opts   Options
	log    *slog.Logger
	roles  *roles.Library
	events *events.Bus // EventWorkspaces, EventRoles
	dirty  chan struct{}
	quit   chan struct{}

	mu      sync.RWMutex
	ws      map[string]*workspace
	started bool
	closed  bool
	// firstRun: the hub started with no workspace saved (a new
	// installation): the interface shows its first steps.
	firstRun bool
}

// New loads every workspace under opts.Root (none is started yet).
func New(opts Options) (*Hub, error) {
	if opts.Root == "" || opts.Trash == "" || opts.Token == "" || opts.Port <= 0 {
		return nil, errors.New("hub: root, trash, token and port are required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if err := os.MkdirAll(opts.Root, 0o700); err != nil {
		return nil, err
	}
	if opts.Roles == nil {
		lib, err := roles.Open(filepath.Join(filepath.Dir(opts.Root), "roles.json"))
		if err != nil {
			return nil, err
		}
		opts.Roles = lib
	}
	if opts.Plans == nil {
		opts.Plans = plans.Open(filepath.Join(filepath.Dir(opts.Root), "plans"))
	}
	h := &Hub{
		opts: opts, log: opts.Logger, roles: opts.Roles, events: events.NewBus(),
		dirty: make(chan struct{}, 1), quit: make(chan struct{}),
		ws: map[string]*workspace{},
	}
	h.roles.Subscribe(func(list []roles.Role) { h.events.Publish(EventRoles, list) })
	entries, err := os.ReadDir(opts.Root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(opts.Root, e.Name())
		st, ss, err := openWorkspace(dir)
		if errors.Is(err, errNotWorkspace) {
			continue
		}
		if err != nil {
			h.log.Warn("skip workspace", "dir", dir, "err", err)
			continue
		}
		if err := h.add(st, ss); err != nil {
			h.Close()
			return nil, err
		}
	}
	h.firstRun = len(h.ws) == 0
	// Workspaces from before colors (the old "default") get one, oldest first.
	for _, s := range h.List() {
		if s.Color == "" {
			color := h.freeColor()
			_, _ = h.get(s.ID).app.Server.Store().UpdateMeta(func(m *store.Meta) error { m.Color = color; return nil })
		}
	}
	return h, nil
}

// add builds the app of a loaded workspace's open session and registers it.
func (h *Hub) add(st *store.Store, ss *sessions) error {
	w, err := h.build(st, ss)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.ws[st.Meta().ID] = w
	h.mu.Unlock()
	return nil
}

// build makes the app of a session's canvas.
func (h *Hub) build(st *store.Store, ss *sessions) (*workspace, error) {
	a, err := app.New(app.Options{
		Server:     server.Options{Port: h.opts.Port, Token: h.opts.Token, Store: st, CLIPath: h.opts.CLIPath, Logger: h.log},
		Activity:   h.opts.Activity,
		Limits:     h.opts.Limits,
		Extensions: h.opts.Extensions,
		Browser:    h.opts.Browser,
		Roles:      h.roles,
		Plans:      h.opts.Plans,
	})
	if err != nil {
		return nil, err
	}
	if h.opts.Setup != nil {
		if err := h.opts.Setup(a); err != nil {
			a.Close()
			return nil, err
		}
	}
	// Any event of the workspace may change its summary; the first turn of
	// a session nobody named names it.
	id := st.Meta().ID
	ch, unsub := a.Server.Bus().Subscribe()
	go func() {
		for ev := range ch {
			h.markDirty()
			if c, ok := ev.Data.(summary.Card); ok && ev.Type == events.TurnCard {
				h.nameSession(id, ss, a, c)
			}
		}
	}()
	return &workspace{app: a, api: a.Server.APIHandler(), term: a.Server.TerminalHandler(), unsub: unsub, s: ss}, nil
}

// Start starts every workspace (activity, notes watcher, terminals) and the
// summary updates.
func (h *Hub) Start() {
	h.mu.Lock()
	h.started = true
	all := h.allLocked()
	h.mu.Unlock()
	for _, w := range all {
		w.app.Start()
	}
	go h.summaryLoop()
}

// Close stops every workspace.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	all := h.allLocked()
	h.mu.Unlock()
	close(h.quit)
	for _, w := range all {
		w.unsub()
		w.app.Close()
	}
}

func (h *Hub) allLocked() []*workspace {
	out := make([]*workspace, 0, len(h.ws))
	for _, w := range h.ws {
		out = append(out, w)
	}
	return out
}

func (h *Hub) get(id string) *workspace {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ws[id]
}

// App returns a workspace's app (nil if unknown).
func (h *Hub) App(id string) *app.App {
	if w := h.get(id); w != nil {
		return w.app
	}
	return nil
}

// byTerminal finds the workspace a terminal belongs to.
func (h *Hub) byTerminal(id string) *workspace {
	if id == "" {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, w := range h.ws {
		if w.app.Server.HasTerminal(id) {
			return w
		}
	}
	return nil
}

// List returns the workspaces in creation order.
func (h *Hub) List() []Summary {
	h.mu.RLock()
	all := h.allLocked()
	h.mu.RUnlock()
	out := make([]Summary, 0, len(all))
	for _, w := range all {
		out = append(out, summarize(w))
	}
	slices.SortFunc(out, func(a, b Summary) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func summarize(w *workspace) Summary {
	a := w.app
	st := a.Server.Store()
	s := Summary{Meta: st.Meta(), Pending: len(a.Bus.Pending()), Session: w.s.current()}
	w.s.mu.Lock()
	s.Sessions = len(w.s.index.Sessions)
	w.s.mu.Unlock()
	for _, id := range st.TerminalIDs() {
		s.Terminals++
		switch state, _ := a.Monitor.State(id); state {
		case activity.Work, activity.Wait:
			s.Active++
		case activity.Ask:
			s.Attention++
		}
	}
	return s
}

// Get returns one workspace's summary.
func (h *Hub) Get(id string) (Summary, error) {
	w := h.get(id)
	if w == nil {
		return Summary{}, ErrNotFound
	}
	return summarize(w), nil
}

// Create adds a workspace for folder dir ("~/…" allowed). An empty name
// becomes the folder's name, an empty color the first unused one; session
// names its first session ("": a provisional name, which an agent replaces).
func (h *Hub) Create(name, dir, color, session string) (Summary, error) {
	cwd, err := checkFolder(dir)
	if err != nil {
		return Summary{}, err
	}
	if color == "" {
		color = h.freeColor()
	} else if !slices.Contains(Colors, color) {
		return Summary{}, invalidf("unknown color %q", color)
	}
	if name = strings.TrimSpace(name); name == "" {
		name = filepath.Base(cwd)
	}
	if strings.TrimSpace(session) != "" {
		if session, err = checkSessionName(session); err != nil {
			return Summary{}, err
		}
	}
	id := ids.Short(8)
	ss := &sessions{dir: filepath.Join(h.opts.Root, id)}
	sid, now := ids.Short(8), time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(ss.path(sid)), 0o700); err != nil {
		return Summary{}, err
	}
	st, err := store.Open(ss.path(sid), store.Workspace{ID: id, Name: name, Cwd: cwd, Color: color, CreatedAt: now})
	if err != nil {
		return Summary{}, err
	}
	first := Session{ID: sid, Name: session, Named: session != "", CreatedAt: now, OpenedAt: now}
	if !first.Named {
		first.Name = provisionalName(now)
	}
	ss.index = sessionIndex{Current: sid, Sessions: []Session{first}}
	if err := ss.saveLocked(); err != nil {
		return Summary{}, err
	}
	if err := h.add(st, ss); err != nil {
		return Summary{}, err
	}
	h.mu.RLock()
	started := h.started
	h.mu.RUnlock()
	if started {
		h.get(id).app.Start()
	}
	h.markDirty()
	return h.Get(id)
}

// Open returns the workspace whose folder is dir, creating it if needed
// (used by `torre serve -dir`).
func (h *Hub) Open(dir string) (Summary, error) {
	cwd, err := checkFolder(dir)
	if err != nil {
		return Summary{}, err
	}
	for _, s := range h.List() {
		if samePath(s.Cwd, cwd) {
			return s, nil
		}
	}
	return h.Create("", cwd, "", "")
}

// Patch is a change to a workspace; nil fields are kept.
type Patch struct {
	Name  *string `json:"name"`
	Cwd   *string `json:"cwd"`
	Color *string `json:"color"`
}

// Update renames a workspace, moves its folder or recolors it. Running
// terminals keep their folder until they restart.
func (h *Hub) Update(id string, p Patch) (Summary, error) {
	w := h.get(id)
	if w == nil {
		return Summary{}, ErrNotFound
	}
	hasCopies := hasRepoCopies(w)
	m, err := w.app.Server.Store().UpdateMeta(func(m *store.Meta) error {
		if p.Name != nil {
			if strings.TrimSpace(*p.Name) == "" {
				return apierr.With(invalidf("the name can't be empty"), "workspace.name_required")
			}
			m.Name = strings.TrimSpace(*p.Name)
		}
		if p.Cwd != nil {
			cwd, err := checkFolder(*p.Cwd)
			if err != nil {
				return err
			}
			if hasCopies && !samePath(cwd, m.Cwd) {
				return apierr.With(invalidf("the workspace has copies of its repository (floors, or sessions working in a copy): land or discard the floors and remove those sessions before changing its folder"), "workspace.has_copies")
			}
			m.Cwd = cwd
		}
		if p.Color != nil {
			if !slices.Contains(Colors, *p.Color) {
				return invalidf("unknown color %q", *p.Color)
			}
			m.Color = *p.Color
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	w.app.Server.Bus().Publish(events.WorkspaceUpdated, m)
	h.markDirty()
	return summarize(w), nil
}

// Remove stops a workspace's terminals and moves its folder (every session:
// canvas, notes, attachments) to the trash folder. The last workspace can't
// be removed.
func (h *Hub) Remove(id string) error {
	h.mu.Lock()
	w := h.ws[id]
	switch {
	case w == nil:
		h.mu.Unlock()
		return ErrNotFound
	case len(h.ws) == 1:
		h.mu.Unlock()
		return apierr.With(invalidf("the last workspace can't be removed"), "workspace.last")
	case hasRepoCopies(w):
		h.mu.Unlock()
		return apierr.With(invalidf("the workspace has copies of its repository (floors, or sessions working in a copy): land or discard the floors and remove those sessions first"), "workspace.has_copies")
	}
	delete(h.ws, id)
	h.mu.Unlock()
	w.unsub()
	w.app.Close()
	h.markDirty()

	folder := w.s.dir
	if err := os.MkdirAll(h.opts.Trash, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(h.opts.Trash, id+"-"+time.Now().Format("20060102-150405"))
	// On Windows the notes watcher may still hold the folder for a moment.
	var err error
	for range 20 {
		if err = os.Rename(folder, dest); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func (h *Hub) freeColor() string {
	used := map[string]int{}
	for _, s := range h.List() {
		used[s.Color]++
	}
	// The least used color, in palette order.
	best := Colors[0]
	for _, c := range Colors {
		if used[c] < used[best] {
			best = c
		}
	}
	return best
}

func (h *Hub) markDirty() {
	select {
	case h.dirty <- struct{}{}:
	default:
	}
}

// summaryLoop publishes the list after workspace events, at most once per
// summaryDelay.
func (h *Hub) summaryLoop() {
	for {
		select {
		case <-h.quit:
			return
		case <-h.dirty:
		}
		select {
		case <-h.quit:
			return
		case <-time.After(summaryDelay):
		}
		select {
		case <-h.dirty:
		default:
		}
		h.events.Publish(EventWorkspaces, h.List())
	}
}

// checkFolder resolves dir ("~/…" or absolute) and checks it is a folder.
func checkFolder(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", apierr.With(invalidf("a folder is required"), "dir.required")
	}
	home, _ := os.UserHomeDir()
	cwd := server.ResolveCwd(dir, home)
	if !filepath.IsAbs(cwd) {
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return "", err
		}
		cwd = abs
	}
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return "", apierr.With(invalidf("folder not found: %s", cwd), "dir.not_found", "folder", cwd)
	}
	return cwd, nil
}

// samePath compares folders after cleaning and resolving symlinks,
// ignoring case on Windows.
func samePath(a, b string) bool {
	norm := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return p
	}
	a, b = norm(a), norm(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

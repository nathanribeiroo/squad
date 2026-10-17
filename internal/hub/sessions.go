package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/app"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/floors"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/summary"
)

// Sessions: a workspace keeps several sessions, each a whole canvas of its
// own (terminals, notes, portals, connections, floors, turn cards) in a
// folder of its own. One is open at a time, and only its processes run:
// opening another stops them (each agent keeps its conversation) and starts
// the other's, each terminal continuing its conversation.
//
//	<root>/<ws>/sessions.json                   the sessions and the open one
//	<root>/<ws>/sessions/<id>/workspace.json     a session's canvas (a store)
//	<root>/<ws>/sessions/<id>/notes/, attachments/, cards.json
//
// What the sessions share (the workspace's name, folder, color and settings,
// store.Shared) lives in the open session's canvas, and a session takes it
// when it opens.
//
// A session may work in a copy of the repository instead of the workspace
// folder (floors.NewCopy): a git worktree on a new branch, made with the
// session, kept when its branch is merged, removed with the session (the
// branch stays).

const sessionsFile = "sessions.json"

// MaxSessionName is the longest session name, in characters.
const MaxSessionName = 60

// EventSessions carries a workspace's sessions after a change
// ({"workspace": id, "current": id, "sessions": [SessionSummary]}).
const EventSessions = "sessions"

// Session is one session of a workspace.
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Named     bool      `json:"named,omitempty"` // by the user or an agent; else a provisional name
	CreatedAt time.Time `json:"createdAt"`
	OpenedAt  time.Time `json:"openedAt"` // when it was last opened
}

// SessionSummary is a session as the sessions panel shows it.
type SessionSummary struct {
	Session
	Current   bool     `json:"current"`
	Terminals int      `json:"terminals"`
	Notes     int      `json:"notes"`
	Portals   int      `json:"portals"`
	Floors    int      `json:"floors"`
	Agents    []string `json:"agents"`           // the agent types of its terminals, each once
	Branch    string   `json:"branch,omitempty"` // a session working in a copy: its branch
	Copy      string   `json:"copy,omitempty"`   // and the copy's folder
}

// SessionList is a workspace's sessions, newest first.
type SessionList struct {
	Workspace string           `json:"workspace"`
	Current   string           `json:"current"`
	Sessions  []SessionSummary `json:"sessions"`
}

// NewSession is how a new session starts.
type NewSession struct {
	Name string `json:"name"` // "" for a provisional one, which an agent replaces
	// KeepTerminals starts it with the open session's terminals (on the
	// ground floor), their connections and roles, in new conversations.
	KeepTerminals bool `json:"keepTerminals"`
	// Confirm leaves the open session even with agents working.
	Confirm bool `json:"confirm"`
	// Copy makes the session work in a copy of the repository on a new
	// branch (nil: in the workspace folder).
	Copy *NewCopy `json:"copy,omitempty"`
}

// NewCopy is where a session's copy starts.
type NewCopy struct {
	Branch string `json:"branch"` // the new branch ("": from the session's name)
	From   string `json:"from"`   // the branch it starts from ("": the workspace folder's)
}

// DirtyCopyError refuses to remove a session whose copy has uncommitted
// changes, unless confirmed: they are lost (the committed work stays on the
// session's branch).
type DirtyCopyError struct {
	Files []string `json:"files"`
}

func (e *DirtyCopyError) Error() string {
	list := e.Files
	more := ""
	if len(list) > 10 {
		list, more = list[:10], fmt.Sprintf(" and %d more", len(e.Files)-10)
	}
	return fmt.Sprintf("the session's copy has uncommitted changes (%s%s): confirm to remove it anyway, or open the session and commit them", strings.Join(list, ", "), more)
}

// BusyError refuses to leave a session whose agents are working, unless
// confirmed: they stop mid-turn (their conversations remain).
type BusyError struct {
	Working []string `json:"working"` // agents in the middle of a turn
	Pending int      `json:"pending"` // requests waiting for a reply
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("agents are working in the open session (%s; %d pending requests): confirm to stop them", strings.Join(e.Working, ", "), e.Pending)
}

type sessionIndex struct {
	SchemaVersion int       `json:"schemaVersion"`
	Current       string    `json:"current"`
	Sessions      []Session `json:"sessions"`
}

// sessions is a workspace's folder and sessions; it outlives the apps of the
// sessions it opens.
type sessions struct {
	dir string     // the workspace's folder
	op  sync.Mutex // one change of session at a time

	mu     sync.Mutex
	index  sessionIndex
	naming bool // an agent is naming the open session
}

// path is a session's canvas file.
func (s *sessions) path(id string) string {
	return filepath.Join(s.dir, "sessions", id, "workspace.json")
}

func (s *sessions) saveLocked() error {
	s.index.SchemaVersion = 1
	data, err := json.MarshalIndent(s.index, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(filepath.Join(s.dir, sessionsFile), data, 0o600)
}

func (s *sessions) findLocked(id string) *Session {
	for i := range s.index.Sessions {
		if s.index.Sessions[i].ID == id {
			return &s.index.Sessions[i]
		}
	}
	return nil
}

// current returns the open session.
func (s *sessions) current() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.findLocked(s.index.Current); c != nil {
		return *c
	}
	return Session{ID: s.index.Current}
}

var errNotWorkspace = errors.New("not a workspace folder")

// openWorkspace loads a workspace folder and opens its current session's
// canvas. A workspace from before sessions becomes one with a single
// session (migrate).
func openWorkspace(dir string) (*store.Store, *sessions, error) {
	s := &sessions{dir: dir}
	legacy := filepath.Join(dir, "workspace.json")
	data, err := os.ReadFile(filepath.Join(dir, sessionsFile))
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.index); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", filepath.Join(dir, sessionsFile), err)
		}
	case errors.Is(err, os.ErrNotExist):
		if _, err := os.Stat(legacy); err != nil {
			return nil, nil, errNotWorkspace
		}
		if err := s.migrate(); err != nil {
			return nil, nil, fmt.Errorf("move %s into its first session: %w", dir, err)
		}
	default:
		return nil, nil, err
	}
	path := s.path(s.index.Current)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// A migration stopped before its last step.
		if _, lerr := os.Stat(legacy); lerr == nil {
			if err := os.Rename(legacy, path); err != nil {
				return nil, nil, err
			}
		} else {
			return nil, nil, fmt.Errorf("%s: the open session %q has no canvas", dir, s.index.Current)
		}
	}
	st, err := store.Open(path, store.Workspace{})
	if err != nil {
		return nil, nil, err
	}
	return st, s, nil
}

// migrate moves a workspace from before sessions (workspace.json, notes/,
// attachments/ in its folder) into its first session. sessions.json is
// written before the canvas moves, so an interrupted migration finishes on
// the next start (openWorkspace).
func (s *sessions) migrate() error {
	id := ids.Short(8)
	if entries, err := os.ReadDir(filepath.Join(s.dir, "sessions")); err == nil && len(entries) == 1 && entries[0].IsDir() {
		id = entries[0].Name() // an earlier attempt's
	}
	sdir := filepath.Join(s.dir, "sessions", id)
	if err := os.MkdirAll(sdir, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"notes", "attachments", "cards.json"} {
		src := filepath.Join(s.dir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, filepath.Join(sdir, name)); err != nil {
			return err
		}
	}
	created := time.Now().UTC()
	if ws, err := store.Read(filepath.Join(s.dir, "workspace.json")); err == nil && !ws.CreatedAt.IsZero() {
		created = ws.CreatedAt
	}
	s.index = sessionIndex{Current: id, Sessions: []Session{{ID: id, Name: "Sessão 1", CreatedAt: created, OpenedAt: time.Now().UTC()}}}
	if err := s.saveLocked(); err != nil {
		return err
	}
	return os.Rename(filepath.Join(s.dir, "workspace.json"), s.path(id))
}

// provisionalName names a session nobody named yet.
func provisionalName(t time.Time) string {
	return "Nova sessão · " + t.Local().Format("02/01 15:04")
}

// checkSessionName validates a session name and returns it trimmed.
func checkSessionName(name string) (string, error) {
	n := strings.Join(strings.Fields(name), " ")
	switch {
	case n == "":
		return "", invalidf("a session needs a name")
	case utf8.RuneCountInString(n) > MaxSessionName:
		return "", invalidf("a session name has at most %d characters", MaxSessionName)
	case strings.IndexFunc(n, unicode.IsControl) >= 0:
		return "", invalidf("a session name is one line of text")
	}
	return n, nil
}

// Sessions lists a workspace's sessions.
func (h *Hub) Sessions(id string) (SessionList, error) {
	w := h.get(id)
	if w == nil {
		return SessionList{}, ErrNotFound
	}
	return h.sessionList(id, w), nil
}

func (h *Hub) sessionList(id string, w *workspace) SessionList {
	w.s.mu.Lock()
	index := w.s.index
	list := append([]Session(nil), index.Sessions...)
	w.s.mu.Unlock()
	out := SessionList{Workspace: id, Current: index.Current, Sessions: make([]SessionSummary, 0, len(list))}
	for _, s := range list {
		sum := SessionSummary{Session: s, Current: s.ID == index.Current, Agents: []string{}}
		var ws store.Workspace
		if sum.Current {
			ws = w.app.Server.Store().Snapshot()
		} else if read, err := store.Read(w.s.path(s.ID)); err == nil {
			ws = read
		}
		for _, n := range ws.Nodes {
			switch {
			case n.Terminal != nil:
				sum.Terminals++
				if agentbus.IsAgent(n.Terminal.Agent) && !slices.Contains(sum.Agents, n.Terminal.Agent) {
					sum.Agents = append(sum.Agents, n.Terminal.Agent)
				}
			case n.Note != nil:
				sum.Notes++
			case n.Portal != nil:
				sum.Portals++
			}
		}
		sum.Floors = len(ws.Floors)
		if ws.Copy != nil {
			sum.Branch, sum.Copy = ws.Copy.Branch, ws.Copy.Path
		}
		out.Sessions = append(out.Sessions, sum)
	}
	slices.SortFunc(out.Sessions, func(a, b SessionSummary) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out
}

func (h *Hub) publishSessions(id string) {
	if w := h.get(id); w != nil {
		h.events.Publish(EventSessions, h.sessionList(id, w))
	}
	h.markDirty()
}

// working reports the agents of an app that are in the middle of a turn and
// its pending requests.
func working(a *app.App) error {
	var names []string
	for _, n := range a.Server.Store().Snapshot().Nodes {
		if n.Terminal != nil && agentbus.IsAgent(n.Terminal.Agent) && a.Monitor.InTurn(n.ID) {
			names = append(names, n.Terminal.Name)
		}
	}
	pending := len(a.Bus.Pending())
	if len(names) == 0 && pending == 0 {
		return nil
	}
	return &BusyError{Working: names, Pending: pending}
}

// NewSession leaves the open session and opens a new one.
func (h *Hub) NewSession(id string, o NewSession) (SessionList, error) {
	name := ""
	if strings.TrimSpace(o.Name) != "" {
		n, err := checkSessionName(o.Name)
		if err != nil {
			return SessionList{}, err
		}
		name = n
	}
	return h.switchSession(id, o.Confirm, func(w *workspace) (string, *store.Store, error) {
		cur := w.app.Server.Store()
		sid := ids.Short(8)
		ws := store.Workspace{Nodes: []store.Node{}, Edges: []store.Edge{}}
		if o.Copy != nil {
			branch := strings.TrimSpace(o.Copy.Branch)
			if branch == "" {
				branch = copyBranch(name, sid)
			}
			f, err := floors.NewCopy(context.Background(), floors.CopyRequest{
				Cwd: cur.Meta().Cwd, Branch: branch, From: o.Copy.From, Config: cur.FloorConfig(),
			})
			if err != nil {
				return "", nil, err
			}
			ws.Copy = &f
		}
		dropCopy := func() {
			if ws.Copy != nil {
				if err := floors.RemoveCopy(context.Background(), cur.Meta().Cwd, *ws.Copy, true); err != nil {
					h.log.Warn("new session: remove its copy", "workspace", id, "err", err)
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(w.s.path(sid)), 0o700); err != nil {
			dropCopy()
			return "", nil, err
		}
		if o.KeepTerminals {
			ws.Nodes, ws.Edges = sameTerminals(cur.Snapshot())
		}
		st, err := store.Open(w.s.path(sid), ws)
		if err == nil {
			err = st.SetShared(cur.Shared())
		}
		if err != nil {
			dropCopy()
			return "", nil, err
		}
		now := time.Now().UTC()
		s := Session{ID: sid, Name: name, Named: name != "", CreatedAt: now, OpenedAt: now}
		if !s.Named {
			s.Name = provisionalName(now)
		}
		w.s.mu.Lock()
		w.s.index.Sessions = append(w.s.index.Sessions, s)
		err = w.s.saveLocked()
		w.s.mu.Unlock()
		return sid, st, err
	})
}

// copyBranch names the branch of a session's copy: after the session's name,
// or its id when it has none yet.
func copyBranch(name, sid string) string {
	if slug := ids.Slug(name); slug != "" {
		return slug
	}
	return "sessao-" + sid
}

// sameTerminals copies a canvas's ground-floor terminals, with their
// connections, roles and positions, for a new session: new ids, no
// conversations.
func sameTerminals(ws store.Workspace) ([]store.Node, []store.Edge) {
	newID := map[string]string{}
	var nodes []store.Node
	for _, n := range ws.Nodes {
		if n.Terminal == nil || n.Floor != "" {
			continue
		}
		t := *n.Terminal
		t.Conversation, t.Busy, t.Empty = "", false, false
		cp := store.Node{ID: ids.New(), Kind: n.Kind, X: n.X, Y: n.Y, W: n.W, H: n.H, Terminal: &t, CreatedBy: n.CreatedBy}
		newID[n.ID] = cp.ID
		nodes = append(nodes, cp)
	}
	for i := range nodes {
		nodes[i].CreatedBy = newID[nodes[i].CreatedBy] // "" when its creator isn't copied
	}
	edges := []store.Edge{}
	for _, e := range ws.Edges {
		a, b := newID[e.A], newID[e.B]
		if a != "" && b != "" {
			edges = append(edges, store.Edge{ID: ids.New(), A: a, B: b, RopePoints: e.RopePoints, CreatedAt: e.CreatedAt})
		}
	}
	if nodes == nil {
		nodes = []store.Node{}
	}
	return nodes, edges
}

// OpenSession leaves the open session and opens session sid as it was.
func (h *Hub) OpenSession(id, sid string, confirm bool) (SessionList, error) {
	return h.switchSession(id, confirm, func(w *workspace) (string, *store.Store, error) {
		w.s.mu.Lock()
		s, cur := w.s.findLocked(sid), w.s.index.Current
		w.s.mu.Unlock()
		switch {
		case s == nil:
			return "", nil, fmt.Errorf("%w: no session %q", ErrNotFound, sid)
		case sid == cur:
			return "", nil, invalidf("that session is already open")
		}
		if _, err := os.Stat(w.s.path(sid)); err != nil {
			return "", nil, fmt.Errorf("session %q has no canvas: %w", sid, err)
		}
		st, err := store.Open(w.s.path(sid), store.Workspace{})
		if err == nil {
			err = st.SetShared(w.app.Server.Store().Shared())
		}
		return sid, st, err
	})
}

// switchSession replaces the open session with the one prepare makes ready:
// unless confirmed, not while agents work; then the open session's
// processes stop, and the other's app starts on its canvas.
func (h *Hub) switchSession(id string, confirm bool, prepare func(*workspace) (string, *store.Store, error)) (SessionList, error) {
	w := h.get(id)
	if w == nil {
		return SessionList{}, ErrNotFound
	}
	w.s.op.Lock()
	defer w.s.op.Unlock()
	if w = h.get(id); w == nil {
		return SessionList{}, ErrNotFound
	}
	if !confirm {
		if err := working(w.app); err != nil {
			return SessionList{}, err
		}
	}
	sid, st, err := prepare(w)
	if err != nil {
		return SessionList{}, err
	}
	prev := w.s.current().ID
	w.unsub()
	w.app.Close()
	nw, err := h.build(st, w.s)
	if err != nil {
		// Back to the session that was open.
		h.log.Error("open session", "workspace", id, "session", sid, "err", err)
		if old, oerr := store.Open(w.s.path(prev), store.Workspace{}); oerr == nil {
			if back, berr := h.build(old, w.s); berr == nil {
				nw, sid = back, prev
			}
		}
		if nw == nil {
			return SessionList{}, err
		}
	}
	h.mu.Lock()
	h.ws[id] = nw
	started := h.started
	h.mu.Unlock()
	w.s.mu.Lock()
	w.s.index.Current = sid
	if s := w.s.findLocked(sid); s != nil {
		s.OpenedAt = time.Now().UTC()
	}
	serr := w.s.saveLocked()
	w.s.mu.Unlock()
	if started {
		nw.app.Start()
	}
	h.publishSessions(id)
	if err == nil {
		err = serr
	}
	return h.sessionList(id, nw), err
}

// RenameSession names a session.
func (h *Hub) RenameSession(id, sid, name string) (SessionList, error) {
	w := h.get(id)
	if w == nil {
		return SessionList{}, ErrNotFound
	}
	n, err := checkSessionName(name)
	if err != nil {
		return SessionList{}, err
	}
	w.s.mu.Lock()
	s := w.s.findLocked(sid)
	if s == nil {
		w.s.mu.Unlock()
		return SessionList{}, fmt.Errorf("%w: no session %q", ErrNotFound, sid)
	}
	s.Name, s.Named = n, true
	err = w.s.saveLocked()
	w.s.mu.Unlock()
	h.publishSessions(id)
	return h.sessionList(id, w), err
}

// RemoveSession moves a session's folder (canvas, notes, attachments) to
// the trash. The agents keep its conversations. The open session and one
// with floors (copies of the repository) can't be removed. A session working
// in a copy loses it (the branch stays); one with uncommitted changes there
// needs confirm.
func (h *Hub) RemoveSession(id, sid string, confirm bool) (SessionList, error) {
	w := h.get(id)
	if w == nil {
		return SessionList{}, ErrNotFound
	}
	w.s.op.Lock()
	defer w.s.op.Unlock()
	w.s.mu.Lock()
	s, cur := w.s.findLocked(sid), w.s.index.Current
	w.s.mu.Unlock()
	switch {
	case s == nil:
		return SessionList{}, fmt.Errorf("%w: no session %q", ErrNotFound, sid)
	case sid == cur:
		return SessionList{}, invalidf("the open session can't be removed: open another one first")
	}
	ws, rerr := store.Read(w.s.path(sid))
	if rerr == nil && len(ws.Floors) > 0 {
		return SessionList{}, invalidf("the session has floors (copies of its repository): open it and land or discard them first")
	}
	if rerr == nil && ws.Copy != nil {
		files, err := floors.Uncommitted(context.Background(), *ws.Copy)
		if err != nil {
			return SessionList{}, err
		}
		if len(files) > 0 && !confirm {
			return SessionList{}, &DirtyCopyError{Files: files}
		}
		if err := floors.RemoveCopy(context.Background(), w.app.Server.Store().Meta().Cwd, *ws.Copy, false); err != nil {
			return SessionList{}, err
		}
	}
	if err := os.MkdirAll(h.opts.Trash, 0o700); err != nil {
		return SessionList{}, err
	}
	dest := filepath.Join(h.opts.Trash, id+"-"+sid+"-"+time.Now().Format("20060102-150405"))
	if err := os.Rename(filepath.Dir(w.s.path(sid)), dest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return SessionList{}, err
	}
	w.s.mu.Lock()
	w.s.index.Sessions = slices.DeleteFunc(w.s.index.Sessions, func(x Session) bool { return x.ID == sid })
	err := w.s.saveLocked()
	w.s.mu.Unlock()
	h.publishSessions(id)
	return h.sessionList(id, h.get(id)), err
}

// hasRepoCopies reports whether any session of a workspace has copies of
// its repository: floors, or a copy it works in.
func hasRepoCopies(w *workspace) bool {
	open := w.app.Server.Store()
	if _, ok := open.Copy(); ok || len(open.Floors()) > 0 {
		return true
	}
	w.s.mu.Lock()
	list := append([]Session(nil), w.s.index.Sessions...)
	cur := w.s.index.Current
	w.s.mu.Unlock()
	for _, s := range list {
		if s.ID == cur {
			continue
		}
		if ws, err := store.Read(w.s.path(s.ID)); err == nil && (len(ws.Floors) > 0 || ws.Copy != nil) {
			return true
		}
	}
	return false
}

// nameSession gives the open session a title when nobody named it: at the
// end of the first turn of its orchestrator (a manager terminal, or any agent
// when there is none), the same agent CLI writes one (claude -p for a
// Claude terminal…). Without one, the turn's prompt names it.
func (h *Hub) nameSession(id string, ss *sessions, a *app.App, c summary.Card) {
	if c.Summarizing || c.Summary != nil || c.Error != "" { // a summary of a turn already seen
		return
	}
	n, ok := a.Server.Store().Node(c.Terminal)
	if !ok || n.Terminal == nil {
		return
	}
	if !a.Team.IsManager(n) {
		for _, o := range a.Server.Store().Snapshot().Nodes {
			if o.Terminal != nil && a.Team.IsManager(o) {
				return // the orchestrator names it
			}
		}
	}
	ss.mu.Lock()
	cur := ss.findLocked(ss.index.Current)
	if cur == nil || cur.Named || ss.naming {
		ss.mu.Unlock()
		return
	}
	sid := cur.ID
	ss.naming = true
	ss.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		title, err := a.Summary.Title(ctx, n.Terminal.Agent, c)
		if err != nil {
			h.log.Info("session title", "workspace", id, "err", err)
			title = titleFromPrompt(c.Prompt)
		}
		ss.mu.Lock()
		ss.naming = false
		s := ss.findLocked(sid)
		changed := s != nil && !s.Named && title != ""
		if changed {
			s.Name, s.Named = title, true
			_ = ss.saveLocked()
		}
		ss.mu.Unlock()
		if changed {
			h.publishSessions(id)
		}
	}()
}

var envelopeRe = regexp.MustCompile(`^\[de: [^\]]*\]\s*`)

// titleFromPrompt makes a title from what started a turn: its first words.
func titleFromPrompt(prompt string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(envelopeRe.ReplaceAllString(strings.TrimSpace(prompt), "")), "\n")
	words := strings.Fields(line)
	if len(words) > 7 {
		words = words[:7]
	}
	t := strings.TrimRight(strings.Join(words, " "), ".,;:!?")
	if r := []rune(t); len(r) > MaxSessionName {
		t = strings.TrimSpace(string(r[:MaxSessionName-1])) + "…"
	}
	return t
}

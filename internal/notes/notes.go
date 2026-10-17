// Package notes stores canvas notes as Markdown files (the file is the
// source of truth; external edits are picked up by a watcher) and serves the
// `torre note …` commands, limited to notes connected to the caller.
//
// A note's name is its title. Names are unique in the workspace, ignoring
// case, and agents find a note by its title, a former title (Aliases) or its
// file. A note made in the UI starts as a draft without a file: the name the
// user confirms names the file, which never changes after that. The user may
// rename the notes they made (the old name stays an alias, so an agent that
// knew it doesn't lose the note); a note an agent made keeps its name.
package notes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Service manages note files.
type Service struct {
	store  *store.Store
	events *events.Bus
	log    *slog.Logger
	dir    string

	mu        sync.Mutex
	selfWrite map[string]time.Time // file → time of our own last write
	watcher   *fsnotify.Watcher

	nameMu sync.Mutex // naming: unique names, drafts getting their file
}

// New creates the notes directory next to the workspace file.
func New(st *store.Store, ev *events.Bus, log *slog.Logger) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}
	dir := filepath.Join(filepath.Dir(st.Path()), "notes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Service{store: st, events: ev, log: log, dir: dir, selfWrite: map[string]time.Time{}}, nil
}

// Dir returns the notes directory.
func (s *Service) Dir() string { return s.dir }

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns a title into a file stem.
func Slug(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	t = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i",
		"ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c").Replace(t)
	t = strings.Trim(slugRe.ReplaceAllString(t, "-"), "-")
	if t == "" {
		t = "nota"
	}
	if len(t) > 60 {
		t = strings.TrimRight(t[:60], "-")
	}
	return t
}

func (s *Service) path(file string) (string, error) {
	if file == "" || strings.ContainsAny(file, `/\`) || strings.HasPrefix(file, ".") {
		return "", fmt.Errorf("invalid note file %q", file)
	}
	return filepath.Join(s.dir, file), nil
}

// uniqueFile returns "<slug>.md", adding -2, -3… when taken.
func (s *Service) uniqueFile(title string) string {
	base := Slug(title)
	used := map[string]bool{}
	for _, n := range s.store.Snapshot().Nodes {
		if n.Note != nil {
			used[strings.ToLower(n.Note.File)] = true
		}
	}
	for i := 1; ; i++ {
		name := base + ".md"
		if i > 1 {
			name = base + "-" + strconv.Itoa(i) + ".md"
		}
		if _, err := os.Stat(filepath.Join(s.dir, name)); used[name] || err == nil {
			continue
		}
		return name
	}
}

// DefaultTitle names a note nobody named: "Nota", then "Nota 2"…
const DefaultTitle = "Nota"

// MaxTitle is the longest note name, in characters.
const MaxTitle = 60

// ErrAgentNote refuses renaming a note an agent made.
var ErrAgentNote = errors.New("an agent made this note: it keeps the name the agent gave it")

// match is how a note answers to a name.
type match int

const (
	noMatch match = iota
	byTitle
	byAlias
	byFile
)

// answers tells whether (and how) note n answers to name, ignoring case.
func answers(n store.Node, name string) match {
	name = strings.TrimSpace(name)
	switch {
	case n.Note == nil || name == "":
		return noMatch
	case strings.EqualFold(n.Note.Title, name):
		return byTitle
	case slices.ContainsFunc(n.Note.Aliases, func(a string) bool { return strings.EqualFold(a, name) }):
		return byAlias
	case n.Note.File != "" && (strings.EqualFold(n.Note.File, name) || strings.EqualFold(strings.TrimSuffix(n.Note.File, ".md"), name)):
		return byFile
	}
	return noMatch
}

// taken returns the note, other than except, that already answers to name.
func (s *Service) taken(name, except string) (store.Node, bool) {
	for _, n := range s.store.Snapshot().Nodes {
		if n.ID != except && answers(n, name) != noMatch {
			return n, true
		}
	}
	return store.Node{}, false
}

// uniqueTitle returns base, or "base 2", "base 3"… when taken.
func (s *Service) uniqueTitle(base string) string {
	for i := 1; ; i++ {
		t := base
		if i > 1 {
			t = base + " " + strconv.Itoa(i)
		}
		if _, ok := s.taken(t, ""); !ok {
			return t
		}
	}
}

// checkTitle validates a note name and returns it trimmed.
func checkTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	switch {
	case t == "":
		return "", fmt.Errorf("%w: a note needs a name", store.ErrInvalid)
	case utf8.RuneCountInString(t) > MaxTitle:
		return "", fmt.Errorf("%w: a note name has at most %d characters", store.ErrInvalid, MaxTitle)
	case strings.HasPrefix(t, "-"):
		return "", fmt.Errorf("%w: a note name can't start with -", store.ErrInvalid)
	case strings.IndexFunc(t, unicode.IsControl) >= 0:
		return "", fmt.Errorf("%w: a note name is one line of text", store.ErrInvalid)
	}
	return t, nil
}

// duplicate is the error for a name another note answers to.
func duplicate(name string, other store.Node) error {
	if strings.EqualFold(other.Note.Title, name) {
		return fmt.Errorf("%w: there is already a note named %q", store.ErrDuplicate, other.Note.Title)
	}
	return fmt.Errorf("%w: %q is taken by the note %q", store.ErrDuplicate, name, other.Note.Title)
}

// PrepareNode fills a new note node before it's stored (installed as the
// server's node preparer). A draft gets a free default name and no file yet;
// any other note must have a free name (a default one when blank) and gets
// a new file named after it.
func (s *Service) PrepareNode(n *store.Node) error {
	if n.Kind != store.KindNote {
		return nil
	}
	s.nameMu.Lock()
	defer s.nameMu.Unlock()
	return s.prepare(n)
}

func (s *Service) prepare(n *store.Node) error {
	if n.Note == nil {
		n.Note = &store.NoteData{}
	}
	n.Note.Aliases = nil
	if strings.TrimSpace(n.Note.Title) == "" || n.Note.Draft {
		n.Note.Title = s.uniqueTitle(DefaultTitle)
	} else {
		t, err := checkTitle(n.Note.Title)
		if err != nil {
			return err
		}
		if other, ok := s.taken(t, ""); ok {
			return duplicate(t, other)
		}
		n.Note.Title = t
	}
	if n.Note.Draft {
		n.Note.File = ""
		return nil
	}
	n.Note.File = s.uniqueFile(n.Note.Title)
	return s.writeFile(n.Note.File, "")
}

// Finalize ends a note's draft: it gets its file, named after its current
// name, and the name is no longer provisional. It happens when the user
// confirms the name, writes content, connects the note, or Torre restarts.
func (s *Service) Finalize(id string) (store.Node, error) {
	s.nameMu.Lock()
	defer s.nameMu.Unlock()
	return s.finalize(id)
}

func (s *Service) finalize(id string) (store.Node, error) {
	n, ok := s.store.Node(id)
	if !ok || n.Note == nil || !n.Note.Draft {
		return n, nil
	}
	file := s.uniqueFile(n.Note.Title)
	if err := s.writeFile(file, ""); err != nil {
		return n, err
	}
	n, err := s.store.UpdateNode(id, func(m *store.Node) error {
		m.Note.Draft, m.Note.File = false, file
		return nil
	})
	if err != nil {
		return n, err
	}
	s.events.Publish(events.NodeUpdated, n)
	return n, nil
}

// Rename gives a note the user made a new name. A draft gets its file from
// it; a note with a file keeps the file, and its old name becomes an alias,
// so an agent that knew it still finds the note. Notes made by agents keep
// their names (ErrAgentNote).
func (s *Service) Rename(id, title string) (store.Node, error) {
	s.nameMu.Lock()
	defer s.nameMu.Unlock()
	n, ok := s.store.Node(id)
	if !ok || n.Note == nil {
		return store.Node{}, store.ErrNotFound
	}
	if n.CreatedBy != "" {
		return n, ErrAgentNote
	}
	t, err := checkTitle(title)
	if err != nil {
		return n, err
	}
	if other, ok := s.taken(t, id); ok {
		return n, duplicate(t, other)
	}
	if t != n.Note.Title {
		old := n.Note.Title
		n, err = s.store.UpdateNode(id, func(m *store.Node) error {
			m.Note.Title = t
			m.Note.Aliases = slices.DeleteFunc(m.Note.Aliases, func(a string) bool { return strings.EqualFold(a, t) })
			// A draft's provisional name was never seen by an agent.
			if !m.Note.Draft && !strings.EqualFold(old, t) {
				m.Note.Aliases = append(m.Note.Aliases, old)
			}
			return nil
		})
		if err != nil {
			return n, err
		}
		s.events.Publish(events.NodeUpdated, n)
	}
	return s.finalize(id)
}

// finalizeAll ends every draft (Torre restarted: the UI that was naming
// them is gone).
func (s *Service) finalizeAll() {
	for _, n := range s.store.Snapshot().Nodes {
		if n.Note != nil && n.Note.Draft {
			if _, err := s.Finalize(n.ID); err != nil {
				s.log.Warn("note draft", "id", n.ID, "err", err)
			}
		}
	}
}

func (s *Service) writeFile(file, content string) error {
	p, err := s.path(file)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.selfWrite[file] = time.Now()
	s.mu.Unlock()
	return config.WriteFileAtomic(p, []byte(content), 0o600)
}

// Read returns a note's content ("" for a draft, which has no file yet).
func (s *Service) Read(file string) (string, error) {
	if file == "" {
		return "", nil
	}
	p, err := s.path(file)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

// Write replaces a note's content and publishes note.edited with the author.
func (s *Service) Write(n store.Node, content, author string) error {
	if n.Note == nil {
		return errors.New("not a note")
	}
	if n.Note.Locked {
		return ErrLocked
	}
	if n.Note.Draft { // content first: the name it has now names the file
		var err error
		if n, err = s.Finalize(n.ID); err != nil {
			return err
		}
	}
	if err := s.writeFile(n.Note.File, content); err != nil {
		return err
	}
	s.events.Publish(events.NoteEdited, map[string]any{"id": n.ID, "file": n.Note.File, "author": author})
	return nil
}

// ErrLocked is returned when writing a locked note.
var ErrLocked = errors.New("the note is locked; the user must unlock it in Torre")

// Watch publishes note.edited for changes made outside Torre (editors,
// agents writing the file directly) and ends the draft of a note when it is
// connected to anything. It stops when ctx ends.
func (s *Service) Watch(ctx context.Context) error {
	s.finalizeAll()
	go s.finalizeConnected(ctx)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(s.dir); err != nil {
		w.Close()
		return err
	}
	s.watcher = w
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if !ev.Has(fsnotify.Write) && !ev.Has(fsnotify.Create) && !ev.Has(fsnotify.Rename) {
					continue
				}
				file := filepath.Base(ev.Name)
				if !strings.HasSuffix(file, ".md") {
					continue
				}
				s.mu.Lock()
				recent := time.Since(s.selfWrite[file]) < time.Second
				s.mu.Unlock()
				if recent {
					continue
				}
				for _, n := range s.store.Snapshot().Nodes {
					if n.Note != nil && n.Note.File == file {
						s.events.Publish(events.NoteEdited, map[string]any{"id": n.ID, "file": file, "author": "external"})
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				s.log.Warn("notes watcher", "err", err)
			}
		}
	}()
	return nil
}

// finalizeConnected ends the draft of a note when a wire reaches it. The
// bus drops a subscriber that falls behind: it subscribes again, after
// ending the drafts whose wires it may have missed.
func (s *Service) finalizeConnected(ctx context.Context) {
	for ctx.Err() == nil {
		sub, unsubscribe := s.events.Subscribe()
		for open := true; open; {
			select {
			case <-ctx.Done():
				unsubscribe()
				return
			case ev, ok := <-sub:
				if open = ok; !ok {
					break
				}
				if e, isEdge := ev.Data.(store.Edge); isEdge && ev.Type == events.EdgeAdded {
					for _, id := range []string{e.A, e.B} {
						if _, err := s.Finalize(id); err != nil {
							s.log.Warn("note draft", "id", id, "err", err)
						}
					}
				}
			}
		}
		unsubscribe()
		for _, n := range s.store.Snapshot().Nodes {
			if n.Note != nil && n.Note.Draft && len(s.store.Neighbors(n.ID)) > 0 {
				_, _ = s.Finalize(n.ID)
			}
		}
	}
}

// accessible returns the notes reachable from a terminal: connected notes
// and notes chained to them (note ↔ note), not through other terminals.
func (s *Service) accessible(caller string) []store.Node {
	seen := map[string]bool{caller: true}
	var out []store.Node
	queue := []string{caller}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range s.store.Neighbors(cur) {
			if n.Kind != store.KindNote || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			if n.Note.Draft { // connected before the watcher saw it
				if f, err := s.Finalize(n.ID); err == nil {
					n = f
				}
			}
			out = append(out, n)
			queue = append(queue, n.ID)
		}
	}
	return out
}

// find returns the connected note that answers to name: by title first,
// then by a former title, then by file. renamed is set when it was found by
// a former title, so the reply can give the current one.
func (s *Service) find(caller, name string) (n store.Node, renamed bool, r agentbus.Result, ok bool) {
	notes := s.accessible(caller)
	for _, want := range []match{byTitle, byAlias, byFile} {
		for _, n := range notes {
			if answers(n, name) == want {
				return n, want == byAlias, agentbus.Result{}, true
			}
		}
	}
	return store.Node{}, false, agentbus.Result{Code: 404, Body: fmt.Sprintf("torre: no connected note named %q (run `torre note list`)", name)}, false
}

// renamedNote is the line that tells an agent a note's current name.
func renamedNote(n store.Node) string {
	return fmt.Sprintf("(this note is now named %q; use that name from now on)\n", n.Note.Title)
}

const noteUsage = `Usage:
  torre note list                         notes connected to you (chained notes included)
  torre note read "Name" [from] [count]   read with line numbers
  torre note write "Name" "content"       replace the content (use - to read stdin)
  torre note edit "Name" "old" "new"      replace text that appears exactly once
  torre note create "Name" ["content"]    create a note connected to you

A note's name is unique in the workspace and permanent: choose a clear one
when you create it. Names ignore case, and a note the user renamed still
answers to its former names.`

// Extension handles `torre note …` for the agent bus.
func (s *Service) Extension(_ context.Context, caller string, args []string) (agentbus.Result, bool) {
	if len(args) == 0 || args[0] != "note" {
		return agentbus.Result{}, false
	}
	if len(args) < 2 || args[1] == "help" {
		return agentbus.Result{Code: 200, Body: noteUsage}, true
	}
	me, _ := s.store.Node(caller)
	author := "agent"
	if me.Terminal != nil {
		author = me.Terminal.Name
	}
	switch args[1] {
	case "list":
		var sb strings.Builder
		notes := s.accessible(caller)
		if len(notes) == 0 {
			return agentbus.Result{Code: 200, Body: "No notes are connected to you."}, true
		}
		for _, n := range notes {
			fmt.Fprintf(&sb, "- name: %q, file: %s", n.Note.Title, n.Note.File)
			if len(n.Note.Aliases) > 0 {
				fmt.Fprintf(&sb, ", formerly: %s", quoteAll(n.Note.Aliases))
			}
			if n.Note.Locked {
				sb.WriteString(", locked: true")
			}
			sb.WriteString("\n")
		}
		return agentbus.Result{Code: 200, Body: sb.String()}, true
	case "read":
		if len(args) < 3 || len(args) > 5 {
			return agentbus.Result{Code: 400, Body: `Usage: torre note read "Name" [from] [count]`}, true
		}
		n, renamed, r, ok := s.find(caller, args[2])
		if !ok {
			return r, true
		}
		content, err := s.Read(n.Note.File)
		if err != nil {
			return agentbus.Result{Code: 500, Body: "torre: " + err.Error()}, true
		}
		from, count := 1, 0
		if len(args) >= 4 {
			from, _ = strconv.Atoi(args[3])
		}
		if len(args) == 5 {
			count, _ = strconv.Atoi(args[4])
		}
		body := numbered(content, from, count)
		if renamed {
			body = renamedNote(n) + body
		}
		return agentbus.Result{Code: 200, Body: body}, true
	case "write":
		if len(args) != 4 {
			return agentbus.Result{Code: 400, Body: `Usage: torre note write "Name" "content"`}, true
		}
		n, renamed, r, ok := s.find(caller, args[2])
		if !ok {
			return r, true
		}
		if err := s.Write(n, args[3], author); err != nil {
			return writeErr(err), true
		}
		return agentbus.Result{Code: 200, Body: okLine("wrote", n, renamed)}, true
	case "edit":
		if len(args) != 5 {
			return agentbus.Result{Code: 400, Body: `Usage: torre note edit "Name" "old" "new"`}, true
		}
		n, renamed, r, ok := s.find(caller, args[2])
		if !ok {
			return r, true
		}
		content, err := s.Read(n.Note.File)
		if err != nil {
			return agentbus.Result{Code: 500, Body: "torre: " + err.Error()}, true
		}
		switch c := strings.Count(content, args[3]); {
		case args[3] == "" || c == 0:
			return agentbus.Result{Code: 409, Body: "torre: the old text was not found in the note (read it again)"}, true
		case c > 1:
			return agentbus.Result{Code: 409, Body: fmt.Sprintf("torre: the old text appears %d times; include more context so it is unique", c)}, true
		}
		if err := s.Write(n, strings.Replace(content, args[3], args[4], 1), author); err != nil {
			return writeErr(err), true
		}
		return agentbus.Result{Code: 200, Body: okLine("edited", n, renamed)}, true
	case "create":
		if len(args) < 3 || len(args) > 4 {
			return agentbus.Result{Code: 400, Body: `Usage: torre note create "Name" ["content"]`}, true
		}
		content := ""
		if len(args) == 4 {
			content = args[3]
		}
		n, err := s.Create(caller, args[2], content, author)
		if err != nil {
			code := 500
			switch {
			case errors.Is(err, store.ErrDuplicate):
				code = 409
			case errors.Is(err, store.ErrInvalid):
				code = 400
			}
			return agentbus.Result{Code: code, Body: "torre: " + err.Error()}, true
		}
		return agentbus.Result{Code: 200, Body: fmt.Sprintf("Created note %q (file %s), connected to you.", n.Note.Title, n.Note.File)}, true
	}
	return agentbus.Result{Code: 400, Body: noteUsage}, true
}

// okLine confirms a write, with the note's current name when it was reached
// by a former one.
func okLine(verb string, n store.Node, renamed bool) string {
	line := fmt.Sprintf("OK: %s %q", verb, n.Note.Title)
	if renamed {
		line += fmt.Sprintf(" (the note is now named %q; use that name from now on)", n.Note.Title)
	}
	return line
}

func quoteAll(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = strconv.Quote(n)
	}
	return strings.Join(q, ", ")
}

func writeErr(err error) agentbus.Result {
	if errors.Is(err, ErrLocked) {
		return agentbus.Result{Code: 423, Body: "torre: " + err.Error()}
	}
	return agentbus.Result{Code: 500, Body: "torre: " + err.Error()}
}

// Create makes a note connected to caller, placed left of it on the canvas.
// Its name is the one the agent gives it, unique and permanent.
func (s *Service) Create(caller, title, content, author string) (store.Node, error) {
	t, err := checkTitle(title)
	if err != nil {
		return store.Node{}, err
	}
	x, y, floor := 0.0, 0.0, ""
	if me, ok := s.store.Node(caller); ok { // a free spot near the agent, like its portals
		floor = me.Floor
		x, y = store.FreeSpot(store.OnFloor(s.store.Snapshot().Nodes, floor), me, 400, 350)
	}
	n := store.Node{Kind: store.KindNote, X: x, Y: y, W: 400, H: 350, Floor: floor, CreatedBy: caller, Note: &store.NoteData{Title: t}}
	s.nameMu.Lock() // the name stays free until the note is stored
	err = s.prepare(&n)
	if err == nil {
		n, err = s.store.AddNode(n)
	}
	s.nameMu.Unlock()
	if err != nil {
		return store.Node{}, err
	}
	s.events.Publish(events.NodeAdded, n)
	if e, err := s.store.AddEdge(caller, n.ID, nil); err == nil {
		s.events.Publish(events.EdgeAdded, e)
	}
	if content != "" {
		if err := s.Write(n, content, author); err != nil {
			return n, err
		}
	}
	return n, nil
}

// numbered formats content with line numbers, from (1-based) and count lines.
func numbered(content string, from, count int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if content == "" {
		lines = nil
	}
	if from < 1 {
		from = 1
	}
	end := len(lines)
	if count > 0 && from-1+count < end {
		end = from - 1 + count
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[%d lines total]\n", len(lines))
	for i := from - 1; i < end; i++ {
		fmt.Fprintf(&sb, "%d\t%s\n", i+1, lines[i])
	}
	return sb.String()
}

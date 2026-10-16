// Package notes stores canvas notes as Markdown files (the file is the
// source of truth; external edits are picked up by a watcher) and serves the
// `squad note …` commands, limited to notes connected to the caller.
package notes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

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

// PrepareNode fills a new note node (file name, empty file) before it's
// stored; installed as the server's node preparer.
func (s *Service) PrepareNode(n *store.Node) error {
	if n.Kind != store.KindNote {
		return nil
	}
	if n.Note == nil {
		n.Note = &store.NoteData{}
	}
	if strings.TrimSpace(n.Note.Title) == "" {
		n.Note.Title = "Nota"
	}
	if n.Note.File == "" {
		n.Note.File = s.uniqueFile(n.Note.Title)
	}
	p, err := s.path(n.Note.File)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return s.writeFile(n.Note.File, "")
	}
	return nil
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

// Read returns a note's content.
func (s *Service) Read(file string) (string, error) {
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
	if err := s.writeFile(n.Note.File, content); err != nil {
		return err
	}
	s.events.Publish(events.NoteEdited, map[string]any{"id": n.ID, "file": n.Note.File, "author": author})
	return nil
}

// ErrLocked is returned when writing a locked note.
var ErrLocked = errors.New("the note is locked; the user must unlock it in Squad")

// Watch publishes note.edited for changes made outside Squad (editors,
// agents writing the file directly). It stops when ctx ends.
func (s *Service) Watch(ctx context.Context) error {
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
			out = append(out, n)
			queue = append(queue, n.ID)
		}
	}
	return out
}

func (s *Service) find(caller, name string) (store.Node, agentbus.Result, bool) {
	for _, n := range s.accessible(caller) {
		if n.Note.Title == name || n.Note.File == name || strings.TrimSuffix(n.Note.File, ".md") == name {
			return n, agentbus.Result{}, true
		}
	}
	return store.Node{}, agentbus.Result{Code: 404, Body: fmt.Sprintf("squad: no connected note named %q (run `squad note list`)", name)}, false
}

const noteUsage = `Usage:
  squad note list                         notes connected to you (chained notes included)
  squad note read "Name" [from] [count]   read with line numbers
  squad note write "Name" "content"       replace the content (use - to read stdin)
  squad note edit "Name" "old" "new"      replace text that appears exactly once
  squad note create "Name" ["content"]    create a note connected to you`

// Extension handles `squad note …` for the agent bus.
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
			if n.Note.Locked {
				sb.WriteString(", locked: true")
			}
			sb.WriteString("\n")
		}
		return agentbus.Result{Code: 200, Body: sb.String()}, true
	case "read":
		if len(args) < 3 || len(args) > 5 {
			return agentbus.Result{Code: 400, Body: `Usage: squad note read "Name" [from] [count]`}, true
		}
		n, r, ok := s.find(caller, args[2])
		if !ok {
			return r, true
		}
		content, err := s.Read(n.Note.File)
		if err != nil {
			return agentbus.Result{Code: 500, Body: "squad: " + err.Error()}, true
		}
		from, count := 1, 0
		if len(args) >= 4 {
			from, _ = strconv.Atoi(args[3])
		}
		if len(args) == 5 {
			count, _ = strconv.Atoi(args[4])
		}
		return agentbus.Result{Code: 200, Body: numbered(content, from, count)}, true
	case "write":
		if len(args) != 4 {
			return agentbus.Result{Code: 400, Body: `Usage: squad note write "Name" "content"`}, true
		}
		n, r, ok := s.find(caller, args[2])
		if !ok {
			return r, true
		}
		if err := s.Write(n, args[3], author); err != nil {
			return writeErr(err), true
		}
		return agentbus.Result{Code: 200, Body: fmt.Sprintf("OK: wrote %q", n.Note.Title)}, true
	case "edit":
		if len(args) != 5 {
			return agentbus.Result{Code: 400, Body: `Usage: squad note edit "Name" "old" "new"`}, true
		}
		n, r, ok := s.find(caller, args[2])
		if !ok {
			return r, true
		}
		content, err := s.Read(n.Note.File)
		if err != nil {
			return agentbus.Result{Code: 500, Body: "squad: " + err.Error()}, true
		}
		switch c := strings.Count(content, args[3]); {
		case args[3] == "" || c == 0:
			return agentbus.Result{Code: 409, Body: "squad: the old text was not found in the note (read it again)"}, true
		case c > 1:
			return agentbus.Result{Code: 409, Body: fmt.Sprintf("squad: the old text appears %d times; include more context so it is unique", c)}, true
		}
		if err := s.Write(n, strings.Replace(content, args[3], args[4], 1), author); err != nil {
			return writeErr(err), true
		}
		return agentbus.Result{Code: 200, Body: fmt.Sprintf("OK: edited %q", n.Note.Title)}, true
	case "create":
		if len(args) < 3 || len(args) > 4 {
			return agentbus.Result{Code: 400, Body: `Usage: squad note create "Name" ["content"]`}, true
		}
		content := ""
		if len(args) == 4 {
			content = args[3]
		}
		n, err := s.Create(caller, args[2], content, author)
		if err != nil {
			code := 500
			if errors.Is(err, store.ErrDuplicate) {
				code = 409
			}
			return agentbus.Result{Code: code, Body: "squad: " + err.Error()}, true
		}
		return agentbus.Result{Code: 200, Body: fmt.Sprintf("Created note %q (file %s), connected to you.", n.Note.Title, n.Note.File)}, true
	}
	return agentbus.Result{Code: 400, Body: noteUsage}, true
}

func writeErr(err error) agentbus.Result {
	if errors.Is(err, ErrLocked) {
		return agentbus.Result{Code: 423, Body: "squad: " + err.Error()}
	}
	return agentbus.Result{Code: 500, Body: "squad: " + err.Error()}
}

// Create makes a note connected to caller, placed left of it on the canvas.
func (s *Service) Create(caller, title, content, author string) (store.Node, error) {
	for _, n := range s.accessible(caller) {
		if n.Note.Title == title {
			return store.Node{}, fmt.Errorf("%w: a connected note named %q", store.ErrDuplicate, title)
		}
	}
	x, y, floor := 0.0, 0.0, ""
	if me, ok := s.store.Node(caller); ok {
		x, y, floor = me.X-440, me.Y, me.Floor
	}
	n := store.Node{Kind: store.KindNote, X: x, Y: y, W: 400, H: 350, Floor: floor, Note: &store.NoteData{Title: title}}
	if err := s.PrepareNode(&n); err != nil {
		return store.Node{}, err
	}
	n, err := s.store.AddNode(n)
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

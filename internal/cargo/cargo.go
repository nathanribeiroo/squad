// Package cargo is the canvas's CARGA: a file explorer of a folder of the
// computer. The user browses it and drags its files and folders into a
// flight (their paths are pasted into the terminal) or into Transmitir.
// Nothing here reaches the agents: they get the paths as text. The routes
// (http.go) are the canvas's only, and a watcher tells the canvas when a
// folder it shows changes.
package cargo

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Canvas is what the service needs from the workspace's server.
type Canvas interface {
	// ResolvePath resolves a path as terminal folders resolve on a floor.
	ResolvePath(floor, path string) string
}

// MaxWatched is how many folders one cargo may have watched (the ones its
// tree shows open).
const MaxWatched = 64

// Service manages the cargo of a workspace.
type Service struct {
	store  *store.Store
	bus    *events.Bus
	canvas Canvas
	log    *slog.Logger

	mu      sync.Mutex
	watcher *fsnotify.Watcher
	watches map[string][]string    // cargo id → the folders its tree shows
	refs    map[string]int         // folder → how many cargo watch it
	pending map[string]*time.Timer // "id\x00folder" → its change, being debounced
	stop    func()
}

// New returns the workspace's cargo service; Start begins watching.
func New(st *store.Store, bus *events.Bus, canvas Canvas, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, bus: bus, canvas: canvas, log: log,
		watches: map[string][]string{}, refs: map[string]int{}, pending: map[string]*time.Timer{}}
}

// Resolve is a cargo's folder, absolute, on its floor.
func (s *Service) Resolve(n store.Node) string {
	if n.Cargo == nil {
		return ""
	}
	return s.canvas.ResolvePath(n.Floor, n.Cargo.Path)
}

// portable is how a cargo's folder is kept: relative to the folder its
// floor (or the session) works in, or to the workspace folder, when inside
// one, so it follows the floor and travels in flight plans; else absolute.
func (s *Service) portable(floor, abs string) string {
	for _, base := range []string{s.canvas.ResolvePath(floor, ""), s.store.Meta().Cwd} {
		if rel, ok := inside(base, abs); ok {
			return rel
		}
	}
	return abs
}

// folder resolves a path on a floor ("": the folder the floor works in) and
// checks that it is a folder.
func (s *Service) folder(floor, path string) (string, error) {
	abs := s.canvas.ResolvePath(floor, strings.TrimSpace(path))
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		return "", apierr.With(fmt.Errorf("%w: folder not found: %s", store.ErrInvalid, abs), "dir.not_found", "folder", abs)
	}
	return abs, nil
}

// PrepareNode readies a new cargo node (the server calls it for every new
// node): its folder (the workspace's when none) must exist and is kept
// portable; without a name it takes its folder's.
func (s *Service) PrepareNode(n *store.Node) error {
	if n.Kind != store.KindCargo {
		return nil
	}
	if n.Cargo == nil {
		n.Cargo = &store.CargoData{}
	}
	abs, err := s.folder(n.Floor, n.Cargo.Path)
	if err != nil {
		return err
	}
	n.Cargo.Path = s.portable(n.Floor, abs)
	if strings.TrimSpace(n.Cargo.Name) == "" {
		n.Cargo.Name = baseName(abs)
	}
	return nil
}

// baseName names a cargo after its folder (the path itself for a root).
func baseName(abs string) string {
	b := filepath.Base(abs)
	if b == "." || b == string(filepath.Separator) || b == "" || strings.HasSuffix(b, ":") || strings.HasSuffix(b, `:\`) {
		b = abs
	}
	if r := []rune(b); len(r) > store.MaxCargoName {
		b = string(r[:store.MaxCargoName])
	}
	return b
}

// Update renames a cargo or opens it on another folder (empty: kept).
func (s *Service) Update(id, name, path string) (store.Node, error) {
	cur, ok := s.store.Node(id)
	if !ok || cur.Cargo == nil {
		return store.Node{}, store.ErrNotFound
	}
	rel := ""
	if strings.TrimSpace(path) != "" {
		abs, err := s.folder(cur.Floor, path)
		if err != nil {
			return store.Node{}, err
		}
		rel = s.portable(cur.Floor, abs)
	}
	n, err := s.store.UpdateNode(id, func(n *store.Node) error {
		if n.Cargo == nil {
			return store.ErrNotFound
		}
		if strings.TrimSpace(name) != "" {
			n.Cargo.Name = strings.TrimSpace(name)
		}
		if rel != "" {
			n.Cargo.Path = rel
		}
		return nil
	})
	if err != nil {
		return store.Node{}, err
	}
	s.bus.Publish(events.NodeUpdated, n)
	return n, nil
}

// Start watches the folders the cargo's trees show: the canvas hears when
// one changes.
func (s *Service) Start() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.log.Warn("cargo: no watcher", "err", err)
		return
	}
	ch, unsub := s.bus.Subscribe()
	done := make(chan struct{})
	s.mu.Lock()
	s.watcher = w
	s.stop = func() { unsub(); close(done) }
	s.mu.Unlock()
	go func() {
		for ev := range ch {
			if ev.Type != events.NodeRemoved {
				continue
			}
			if m, ok := ev.Data.(map[string]string); ok && m["id"] != "" {
				s.Watch(m["id"], nil) // a no-op for other nodes
			}
		}
	}()
	go func() {
		for {
			select {
			case <-done:
				return
			case e, ok := <-w.Events:
				if !ok {
					return
				}
				s.changed(filepath.Dir(e.Name))
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// Close stops watching.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	if s.watcher != nil {
		_ = s.watcher.Close()
		s.watcher = nil
	}
	for k, t := range s.pending {
		t.Stop()
		delete(s.pending, k)
	}
	s.watches, s.refs = map[string][]string{}, map[string]int{}
}

// Watch sets the folders a cargo's tree shows (absolute paths; nil: none),
// at most MaxWatched.
func (s *Service) Watch(id string, dirs []string) {
	var keep []string
	for _, d := range dirs {
		d = filepath.Clean(d)
		if !filepath.IsAbs(d) || len(keep) >= MaxWatched {
			continue
		}
		if fi, err := os.Stat(d); err == nil && fi.IsDir() && !contains(keep, d) {
			keep = append(keep, d)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.watches[id] {
		if s.refs[d]--; s.refs[d] <= 0 {
			delete(s.refs, d)
			if s.watcher != nil {
				_ = s.watcher.Remove(d)
			}
		}
	}
	if len(keep) == 0 {
		delete(s.watches, id)
		return
	}
	s.watches[id] = keep
	for _, d := range keep {
		if s.refs[d] == 0 && s.watcher != nil {
			if err := s.watcher.Add(d); err != nil {
				continue
			}
		}
		s.refs[d]++
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// changed tells the canvas which cargo show a folder that changed, once
// the burst of events settles (a program writing a file sends several).
func (s *Service) changed(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, dirs := range s.watches {
		if !contains(dirs, dir) {
			continue
		}
		key := id + "\x00" + dir
		if t, ok := s.pending[key]; ok {
			t.Reset(250 * time.Millisecond)
			continue
		}
		id, d := id, dir
		s.pending[key] = time.AfterFunc(250*time.Millisecond, func() {
			s.mu.Lock()
			delete(s.pending, key)
			s.mu.Unlock()
			s.bus.Publish(events.CargoChanged, map[string]string{"id": id, "dir": d})
		})
	}
}

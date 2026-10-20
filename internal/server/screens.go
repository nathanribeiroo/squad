package server

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Stopped terminals keep showing their last screen: a workspace off screen
// stops its agents (hub), and Torre itself may close. The screen is saved
// next to the session's canvas (screens/<id>.ans, a Render), so a terminal
// that hasn't run since Torre started shows it too.

// screenPath is where a terminal's last screen is kept ("" for a canvas
// kept in memory).
func (s *Server) screenPath(id string) string {
	p := s.store.Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "screens", id+".ans")
}

// saveScreen keeps a running terminal's screen for when it stops.
func (s *Server) saveScreen(t *terminal.Terminal) {
	path := s.screenPath(t.ID())
	if path == "" {
		return
	}
	if st, _ := t.State(); st != terminal.StatusRunning {
		return // a stopped one already shows what was saved
	}
	scr := t.Screen()
	if scr == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		s.log.Warn("save screen", "terminal", t.Name(), "err", err)
		return
	}
	if err := config.WriteFileAtomic(path, []byte(scr.Render()), 0o600); err != nil {
		s.log.Warn("save screen", "terminal", t.Name(), "err", err)
	}
}

// SaveScreens keeps the screen of every running terminal.
func (s *Server) SaveScreens() {
	for _, t := range s.terms.List() {
		s.saveScreen(t)
	}
}

// StopTerminal stops a terminal's process; the terminal stays, showing its
// last screen, until it starts again.
func (s *Server) StopTerminal(id string) {
	if t := s.terms.Get(id); t != nil {
		s.saveScreen(t)
		t.Stop()
	}
}

// Running reports whether a terminal's process runs or is starting.
func (s *Server) Running(id string) bool {
	t := s.terms.Get(id)
	if t == nil {
		return false
	}
	st, _ := t.State()
	return st == terminal.StatusRunning || st == terminal.StatusStarting
}

// RestoreTerminals shows the canvas's terminals that don't run yet as
// stopped, with their last screen.
func (s *Server) RestoreTerminals() {
	for _, n := range s.store.Snapshot().Nodes {
		if n.Kind != store.KindTerminal || n.Terminal == nil || s.terms.Get(n.ID) != nil {
			continue
		}
		saved := ""
		if path := s.screenPath(n.ID); path != "" {
			if b, err := os.ReadFile(path); err == nil {
				saved = string(b)
			} else if !errors.Is(err, os.ErrNotExist) {
				s.log.Warn("read screen", "terminal", n.Terminal.Name, "err", err)
			}
		}
		spec := terminal.Spec{ID: n.ID, Name: n.Terminal.Name, Agent: n.Terminal.Agent, Cols: n.Terminal.Cols, Rows: n.Terminal.Rows}
		if _, err := s.terms.Register(spec, saved); err != nil {
			s.log.Warn("show terminal", "name", n.Terminal.Name, "err", err)
		}
	}
}

// forgetScreen drops a removed terminal's saved screen.
func (s *Server) forgetScreen(id string) {
	if path := s.screenPath(id); path != "" {
		_ = os.Remove(path)
	}
}

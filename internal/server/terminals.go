package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// LaunchHook lets the agent adapters (B4) rewrite a terminal's launch line
// and add environment variables before it starts.
type LaunchHook func(node store.Node, spec *terminal.Spec) error

// SetLaunchHook installs the adapter hook.
func (s *Server) SetLaunchHook(h LaunchHook) { s.launchHook = h }

// TerminalToken returns the CLI token of a terminal.
func (s *Server) TerminalToken(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.termTokens[id]
}

// StartTerminal starts the process for a terminal node.
func (s *Server) StartTerminal(id string) error {
	n, ok := s.store.Node(id)
	if !ok || n.Terminal == nil {
		return errors.New("terminal node not found")
	}
	cwd := ResolveCwd(n.Terminal.Cwd, s.store.Snapshot().Cwd)
	tok := newToken()
	s.mu.Lock()
	s.termTokens[id] = tok
	s.mu.Unlock()
	spec := terminal.Spec{
		ID: id, Name: n.Terminal.Name, Agent: n.Terminal.Agent,
		Command: n.Terminal.Command, Shell: n.Terminal.Shell, Cwd: cwd,
		Cols: n.Terminal.Cols, Rows: n.Terminal.Rows,
		Env: map[string]string{
			"SQUAD_URL":           s.URL(),
			"SQUAD_TERMINAL_ID":   id,
			"SQUAD_TERMINAL_NAME": n.Terminal.Name,
			"SQUAD_TOKEN":         tok,
			"SQUAD_CLI":           s.opts.CLIPath,
		},
	}
	if s.launchHook != nil {
		if err := s.launchHook(n, &spec); err != nil {
			return err
		}
	}
	if old := s.terms.Get(id); old != nil {
		s.terms.Remove(id)
	}
	_, err := s.terms.Create(spec)
	return err
}

// ResolveCwd returns a terminal's working directory: its own folder (absolute,
// "~/…", or relative to the workspace folder, so it follows the workspace if
// that moves) or, when empty, the workspace folder.
func ResolveCwd(cwd, wsCwd string) string {
	switch {
	case cwd == "":
		return wsCwd
	case cwd == "~" || strings.HasPrefix(cwd, "~/") || strings.HasPrefix(cwd, `~\`):
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, cwd[1:])
		}
	case filepath.IsAbs(cwd):
		return filepath.Clean(cwd)
	}
	return filepath.Join(wsCwd, cwd)
}

// checkTerminalDir reports a terminal folder that doesn't exist, so the
// mistake shows in the form instead of as a terminal that fails to start.
func checkTerminalDir(t *store.TerminalData, wsCwd string) error {
	if t == nil || t.Cwd == "" {
		return nil
	}
	dir := ResolveCwd(t.Cwd, wsCwd)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.Join(store.ErrInvalid, fmt.Errorf("folder not found: %s", dir))
	}
	return nil
}

// StartTerminals starts every terminal node of the workspace.
func (s *Server) StartTerminals() {
	for _, n := range s.store.Snapshot().Nodes {
		if n.Kind == store.KindTerminal {
			if err := s.StartTerminal(n.ID); err != nil {
				s.log.Warn("start terminal", "name", n.Terminal.Name, "err", err)
			}
		}
	}
}

func (s *Server) onTerminalEvent(ev terminal.Event) {
	switch ev.Kind {
	case terminal.EventTitle:
		s.mu.Lock()
		s.titles[ev.ID] = ev.Title
		s.mu.Unlock()
		s.bus.Publish(events.TerminalTitle, map[string]string{"id": ev.ID, "title": ev.Title})
	case terminal.EventStatus:
		rt := s.runtimeOf(ev.ID)
		rt.Status, rt.ExitCode, rt.Error = string(ev.Status), ev.ExitCode, ev.Err
		s.bus.Publish(events.TerminalState, rt)
	}
	if s.termListener != nil {
		s.termListener(ev)
	}
}

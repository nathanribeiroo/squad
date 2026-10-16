package server

import (
	"errors"
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
	cwd := s.cwdResolver(n.Floor)(n.Terminal.Cwd)
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

// cwdResolver returns how terminal folders resolve on a floor ("" is the
// ground floor): from the workspace folder, or from the floor's copy of it,
// where a folder of the ground floor maps to its copy. It reads the store
// now, so the function can run under the store's lock.
func (s *Server) cwdResolver(floor string) func(cwd string) string {
	ws := s.store.Meta().Cwd
	f, ok := s.store.Floor(floor)
	if floor == "" || !ok {
		return func(cwd string) string { return ResolveCwd(cwd, ws) }
	}
	return func(cwd string) string {
		if filepath.IsAbs(cwd) {
			if rel, err := filepath.Rel(ws, cwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				cwd = rel
			}
		}
		return ResolveCwd(cwd, f.Cwd)
	}
}

// checkTerminalDir reports a terminal folder that doesn't exist, so the
// mistake shows in the form instead of as a terminal that fails to start.
func checkTerminalDir(t *store.TerminalData, resolve func(string) string) error {
	if t == nil || t.Cwd == "" {
		return nil
	}
	dir := resolve(t.Cwd)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return invalidError("folder not found: " + dir)
	}
	return nil
}

// invalidError is a 400 (it matches store.ErrInvalid) whose message is shown
// as is, without the "invalid" prefix errors.Join would add.
type invalidError string

func (e invalidError) Error() string        { return string(e) }
func (e invalidError) Is(target error) bool { return target == store.ErrInvalid }

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

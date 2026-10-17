package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// LaunchHook lets the agent adapters (B4) rewrite a terminal's launch line
// and add environment variables before it starts. It sets the node's
// conversation to the one the terminal is in, and reports whether the line
// continues it.
type LaunchHook func(node *store.Node, spec *terminal.Spec) (resumed bool, err error)

// SetLaunchHook installs the adapter hook.
func (s *Server) SetLaunchHook(h LaunchHook) { s.launchHook = h }

// TerminalToken returns the CLI token of a terminal.
func (s *Server) TerminalToken(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.termTokens[id]
}

// StartTerminal starts the process for a terminal node; its agent continues
// the conversation it was in.
func (s *Server) StartTerminal(id string) error { return s.startTerminal(id, false) }

// StartTerminalFresh starts it with a new conversation.
func (s *Server) StartTerminalFresh(id string) error { return s.startTerminal(id, true) }

func (s *Server) startTerminal(id string, fresh bool) error {
	n, ok := s.store.Node(id)
	if !ok || n.Terminal == nil {
		return errors.New("terminal node not found")
	}
	before := *n.Terminal
	if fresh {
		n.Terminal.Conversation, n.Terminal.Busy, n.Terminal.Empty = "", false, false
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
		Env: config.Env(map[string]string{
			"URL":           s.URL(),
			"TERMINAL_ID":   id,
			"TERMINAL_NAME": n.Terminal.Name,
			"TOKEN":         tok,
			"CLI":           s.opts.CLIPath,
		}),
	}
	resumed := false
	if s.launchHook != nil {
		r, err := s.launchHook(&n, &spec)
		if err != nil {
			return err
		}
		resumed = r
	}
	s.startedConversation(n, before, resumed)
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
// ground floor): from the folder the session works in (the workspace folder,
// or the session's copy of it), or from the floor's copy, where a folder of
// the project (or of the session's copy) maps to its copy. It reads the store
// now, so the function can run under the store's lock.
func (s *Server) cwdResolver(floor string) func(cwd string) string {
	ws, work := s.store.Meta().Cwd, s.store.Workdir()
	base := work
	if f, ok := s.store.Floor(floor); floor != "" && ok {
		base = f.Cwd
	}
	if base == ws {
		return func(cwd string) string { return ResolveCwd(cwd, ws) }
	}
	return func(cwd string) string {
		if filepath.IsAbs(cwd) {
			for _, from := range []string{work, ws} {
				if rel, err := filepath.Rel(from, cwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					cwd = rel
					break
				}
			}
		}
		return ResolveCwd(cwd, base)
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
		return apierr.With(invalidError("folder not found: "+dir), "dir.not_found", "folder", dir)
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

package app

import (
	"cmp"
	"slices"
	"sync"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/store"
)

// A workspace nobody looks at and with nothing to do stops its agents (the
// hub decides when): each keeps its conversation and its last screen, and
// starts again, continuing it, when the workspace is back on screen. Shells
// and own commands keep running: nothing would bring back what they had.

// Work is what keeps a workspace's agents running off screen.
type Work struct {
	Agents    []string // in the middle of a turn or asking the user
	Pending   int      // requests waiting for a reply, queued ones too
	Preparing int      // setup commands of floors (or of the session's copy) running
}

// None reports that nothing is going on.
func (w Work) None() bool { return len(w.Agents) == 0 && w.Pending == 0 && w.Preparing == 0 }

// Work returns what is going on in the workspace.
func (a *App) Work() Work {
	w := Work{Pending: len(a.Bus.Pending()), Preparing: a.Floors.Preparing()}
	for _, n := range a.Server.Store().Snapshot().Nodes {
		if !isAgent(n) {
			continue
		}
		st, _ := a.Monitor.State(n.ID)
		if a.Monitor.InTurn(n.ID) || st == activity.Ask || st == activity.Wait {
			w.Agents = append(w.Agents, n.Terminal.Name)
		}
	}
	return w
}

// StoppedAgents returns the agents that don't run, top to bottom and left to
// right on the canvas.
func (a *App) StoppedAgents() []string {
	var nodes []store.Node
	for _, n := range a.Server.Store().Snapshot().Nodes {
		if isAgent(n) && !a.Server.Running(n.ID) {
			nodes = append(nodes, n)
		}
	}
	slices.SortStableFunc(nodes, func(x, y store.Node) int {
		return cmp.Or(cmp.Compare(x.Y, y.Y), cmp.Compare(x.X, y.X))
	})
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

// AgentsRunning reports whether any agent runs.
func (a *App) AgentsRunning() bool {
	for _, n := range a.Server.Store().Snapshot().Nodes {
		if isAgent(n) && a.Server.Running(n.ID) {
			return true
		}
	}
	return false
}

// StartShells starts the shells and own commands that don't run.
func (a *App) StartShells() {
	for _, n := range a.Server.Store().Snapshot().Nodes {
		if n.Terminal != nil && !isAgent(n) && !a.Server.Running(n.ID) {
			_ = a.Server.StartTerminal(n.ID) // failures are reported as terminal state
		}
	}
}

// Sleep stops the agents, each keeping its conversation and last screen,
// and closes the portals' tabs. Requests still waiting for a reply are
// cancelled: they only remain when the user stops a busy workspace.
func (a *App) Sleep() {
	for _, p := range a.Bus.Pending() {
		a.Bus.Cancel(p.ID)
	}
	var wg sync.WaitGroup // one that ignores the polite signals takes a while
	for _, n := range a.Server.Store().Snapshot().Nodes {
		if isAgent(n) && a.Server.Running(n.ID) {
			wg.Go(func() {
				a.Server.StopTerminal(n.ID)
				a.Monitor.Reset(n.ID)
			})
		}
	}
	wg.Wait()
	a.Portals.ClosePages()
}

func isAgent(n store.Node) bool { return n.Terminal != nil && agentbus.IsAgent(n.Terminal.Agent) }

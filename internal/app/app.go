// Package app assembles the Squad server: HTTP server, terminals, activity
// monitor and the agent bus. Used by `squad serve` and integration tests.
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/agents"
	"github.com/nathanribeiroo/squad/internal/server"
)

// Options configure the application.
type Options struct {
	Server     server.Options
	Activity   activity.Options
	Limits     agentbus.Limits
	Extensions []agentbus.Extension // extra CLI commands (notes, …)
}

// App is a wired Squad instance.
type App struct {
	Server  *server.Server
	Monitor *activity.Monitor
	Bus     *agentbus.Bus
}

// New builds the app; call Start to begin activity sampling.
func New(o Options) (*App, error) {
	srv, err := server.New(o.Server)
	if err != nil {
		return nil, err
	}
	bus := agentbus.New(srv.Store(), srv.Terminals(), srv.Bus(), o.Server.Logger, o.Limits)
	mon := activity.New(srv.Terminals(), o.Activity, bus.IsWaiting, func(id string, _ activity.State, _ time.Time) {
		srv.PublishTerminalState(id)
	})
	bus.SetMonitor(mon)
	srv.SetActivity(func(id string) (string, time.Time) {
		st, since := mon.State(id)
		return string(st), since
	})
	srv.SetStateExtra(bus.StateExtra)
	srv.OnEdgeRemoved(bus.EdgeRemoved)
	srv.OnNodeRemoved(bus.NodeRemoved)
	srv.SetCLIHandler(bus.Handler(server.CallerID, o.Extensions...))

	// Agent adapters: launch flags that install hooks, and the hook receiver.
	if o.Server.CLIPath != "" {
		launcher, err := agents.NewLauncher(o.Server.CLIPath)
		if err != nil {
			return nil, err
		}
		srv.SetLaunchHook(launcher.Prepare)
	}
	srv.SetHookHandler(agents.HookHandler(mon, server.CallerID))
	bus.SetHookedAgents(agents.HasHooks)

	srv.HandleSession("POST /api/conversations/pause", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bus.SetPaused(true, "user")
		writeJSON(w, bus.StateExtra())
	}))
	srv.HandleSession("POST /api/conversations/resume", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bus.SetPaused(false, "user")
		writeJSON(w, bus.StateExtra())
	}))
	return &App{Server: srv, Monitor: mon, Bus: bus}, nil
}

// Start begins activity sampling and starts the workspace's terminals.
func (a *App) Start() {
	a.Monitor.Start()
	a.Server.StartTerminals()
}

// Close stops sampling and every terminal.
func (a *App) Close() {
	a.Monitor.Stop()
	a.Server.Shutdown()
}

// Ask is a convenience for tests: run a CLI command as a terminal.
func (a *App) Run(ctx context.Context, caller string, args ...string) agentbus.Result {
	return a.Bus.Dispatch(ctx, caller, args)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

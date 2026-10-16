// Package app assembles the Squad server: HTTP server, terminals, activity
// monitor and the agent bus. Used by `squad serve` and integration tests.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/agents"
	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/floors"
	"github.com/nathanribeiroo/squad/internal/notes"
	"github.com/nathanribeiroo/squad/internal/portal"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/summary"
	"github.com/nathanribeiroo/squad/internal/team"
)

// Options configure the application.
type Options struct {
	Server     server.Options
	Activity   activity.Options
	Limits     agentbus.Limits
	Extensions []agentbus.Extension // extra CLI commands (notes, …)
	Browser    *browser.Manager     // shared by the portals of every workspace; nil disables them
	Roles      *roles.Library       // shared by every workspace; nil: roles are only labels
}

// App is a wired Squad instance.
type App struct {
	Server  *server.Server
	Monitor *activity.Monitor
	Bus     *agentbus.Bus
	Notes   *notes.Service
	Portals *portal.Service
	Floors  *floors.Service
	Summary *summary.Service

	exts   []agentbus.Extension
	cancel context.CancelFunc
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
	var launcher *agents.Launcher
	var summaries *summary.Service
	srv.OnNodeRemoved(func(n store.Node) {
		bus.NodeRemoved(n)
		if launcher != nil {
			launcher.Forget(n.ID)
		}
		if summaries != nil {
			summaries.Forget(n.ID)
		}
	})
	// roleOf is the instructions a terminal's agent receives for its role.
	roleOf := func(n store.Node) (string, bool) {
		if o.Roles == nil || n.Terminal == nil || n.Terminal.Role == "" {
			return "", false
		}
		r, ok := o.Roles.Get(n.Terminal.Role)
		if !ok || r.Prompt == "" {
			return "", false
		}
		return roles.Instructions(r, n.Terminal.Name), true
	}
	roleName := func(id string) string {
		if o.Roles != nil {
			if r, ok := o.Roles.Get(id); ok {
				return r.Name
			}
		}
		return ""
	}
	bus.SetRoleNames(roleName)
	// Turn cards and summaries (R6).
	summaries = summary.New(summary.Options{
		Canvas: srv, Logger: o.Server.Logger, IsAgent: agentbus.IsAgent, HasHooks: agents.HasHooks, RoleName: roleName,
		State: func(id string) activity.State { st, _ := mon.State(id); return st },
		Pending: func() []summary.PendingRequest {
			var out []summary.PendingRequest
			for _, p := range bus.Pending() {
				out = append(out, summary.PendingRequest{From: p.FromName, To: p.ToName, Preview: p.Preview})
			}
			return out
		},
	})
	summaries.Attach(mon)
	summaries.Register(srv.HandleSession)
	notesSvc, err := notes.New(srv.Store(), srv.Bus(), o.Server.Logger)
	if err != nil {
		return nil, err
	}
	portals := portal.New(srv.Store(), srv.Bus(), o.Browser, o.Server.Port, o.Server.Logger)
	srv.SetNodePreparer(func(n *store.Node) error {
		if err := notesSvc.PrepareNode(n); err != nil {
			return err
		}
		return portals.PrepareNode(n)
	})
	srv.HandleSession("GET /ws/portal/{id}", http.HandlerFunc(portals.HandleWS))
	portals.SetCanvas(srv)
	teamSvc := team.New(team.Options{Canvas: srv, Monitor: mon, Roles: o.Roles, MaxTeam: o.Limits.MaxTeam})
	bus.SetSelfNote(teamSvc.Note)
	floorsSvc := floors.New(floors.Options{Canvas: srv, Logger: o.Server.Logger, IsManager: teamSvc.IsManager})
	floorsSvc.Register(srv.HandleSession)
	exts := append([]agentbus.Extension{notesSvc.Extension, portals.Extension, teamSvc.Extension, floorsSvc.Extension}, o.Extensions...)
	srv.SetCLIHandler(bus.Handler(server.CallerID, exts...))
	srv.HandleSession("GET /api/notes/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, ok := srv.Store().Node(r.PathValue("id"))
		if !ok || n.Note == nil {
			http.Error(w, `{"error":"note not found"}`, http.StatusNotFound)
			return
		}
		content, err := notesSvc.Read(n.Note.File)
		if err != nil {
			http.Error(w, `{"error":"read failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": n.ID, "file": n.Note.File, "title": n.Note.Title, "locked": n.Note.Locked, "content": content})
	}))
	srv.HandleSession("PUT /api/notes/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, ok := srv.Store().Node(r.PathValue("id"))
		if !ok || n.Note == nil {
			http.Error(w, `{"error":"note not found"}`, http.StatusNotFound)
			return
		}
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}
		if err := notesSvc.Write(n, body.Content, "user"); err != nil {
			code := http.StatusInternalServerError
			if errors.Is(err, notes.ErrLocked) {
				code = http.StatusLocked
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, code)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))

	// Agent adapters: launch flags that install hooks, and the hook receiver.
	if o.Server.CLIPath != "" {
		if launcher, err = agents.NewLauncher(o.Server.CLIPath); err != nil {
			return nil, err
		}
		launcher.Role = roleOf
		srv.SetLaunchHook(launcher.Prepare)
	}
	srv.SetHookHandler(agents.HookHandler(mon, server.CallerID, func(id string) (string, bool) {
		n, ok := srv.Store().Node(id)
		if !ok || n.Terminal == nil || !agents.Lookup(n.Terminal.Agent).Roles {
			return "", false
		}
		return roleOf(n)
	}))
	bus.SetHookedAgents(agents.HasHooks)

	var app *App
	srv.HandleSession("POST /api/nodes/{id}/send", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.handleSend(w, r) }))

	srv.HandleSession("POST /api/conversations/pause", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bus.SetPaused(true, "user")
		writeJSON(w, bus.StateExtra())
	}))
	srv.HandleSession("POST /api/conversations/resume", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bus.SetPaused(false, "user")
		writeJSON(w, bus.StateExtra())
	}))
	app = &App{Server: srv, Monitor: mon, Bus: bus, Notes: notesSvc, Portals: portals, Floors: floorsSvc, Summary: summaries, exts: exts}
	return app, nil
}

// Start begins activity sampling, watches notes and starts the workspace's
// terminals.
func (a *App) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.Monitor.Start()
	_ = a.Notes.Watch(ctx)
	a.Summary.Watch(ctx)
	a.Portals.Start()
	a.Floors.Start()
	a.Server.StartTerminals()
}

// Close stops sampling, the notes watcher and every terminal.
func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.Monitor.Stop()
	a.Portals.Close()
	a.Floors.Close()
	a.Server.Shutdown()
}

// Run executes a CLI command as a terminal (used by tests).
func (a *App) Run(ctx context.Context, caller string, args ...string) agentbus.Result {
	return a.Bus.Dispatch(ctx, caller, args, a.exts...)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

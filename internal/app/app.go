// Package app assembles the Torre server: HTTP server, terminals, activity
// monitor and the agent bus. Used by `torre serve` and integration tests.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/agents"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/floors"
	"github.com/nathanribeiroo/squad/internal/notes"
	"github.com/nathanribeiroo/squad/internal/plans"
	"github.com/nathanribeiroo/squad/internal/portal"
	"github.com/nathanribeiroo/squad/internal/radio"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/summary"
	"github.com/nathanribeiroo/squad/internal/team"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Options configure the application.
type Options struct {
	Server     server.Options
	Activity   activity.Options
	Limits     agentbus.Limits
	Extensions []agentbus.Extension // extra CLI commands (notes, …)
	Browser    *browser.Manager     // shared by the portals of every workspace; nil disables them
	Roles      *roles.Library       // shared by every workspace; nil: roles are only labels
	Plans      *plans.Library       // flight plans, shared by every workspace; nil: none
}

// App is a wired Torre instance.
type App struct {
	Server  *server.Server
	Monitor *activity.Monitor
	Bus     *agentbus.Bus
	Notes   *notes.Service
	Portals *portal.Service
	Floors  *floors.Service
	Summary *summary.Service
	Team    *team.Service
	Radio   *radio.Service

	exts     []agentbus.Extension
	cancel   context.CancelFunc
	plans    *plans.Library
	roles    *roles.Library
	defaults store.Limits // the server's limits, before the workspace's
}

// New builds the app; call Start to begin activity sampling.
func New(o Options) (*App, error) {
	srv, err := server.New(o.Server)
	if err != nil {
		return nil, err
	}
	bus := agentbus.New(srv.Store(), srv.Terminals(), srv.Bus(), o.Server.Logger, o.Limits)
	// The Rádio, kept with the canvas (each session has its own).
	radioFile := ""
	if p := srv.Store().Path(); p != "" {
		radioFile = filepath.Join(filepath.Dir(p), "radio.jsonl")
	}
	radioSvc := radio.New(radio.Options{File: radioFile, Events: srv.Bus(), Logger: o.Server.Logger,
		Name: func(id string) string {
			if n, ok := srv.Store().Node(id); ok {
				return n.Label()
			}
			return ""
		},
		Texts: func(id string) (string, string) {
			req, _ := bus.RequestText(id)
			reply, _ := bus.ReplyText(id)
			return req, reply
		}})
	asks := newAskLog(radioSvc)
	var mon *activity.Monitor
	mon = activity.New(srv.Terminals(), o.Activity, bus.IsWaiting, func(id string, _ activity.State, _ time.Time) {
		srv.PublishTerminalState(id)
		asks.seen(id, mon.Attention(id))
	})
	bus.SetMonitor(mon)
	srv.SetActivity(func(id string) (string, time.Time) {
		st, since := mon.State(id)
		return string(st), since
	})
	// Only agents' screens are read for prompts (PRIORIDADE).
	mon.SetAgentOf(func(id string) (string, bool) {
		n, ok := srv.Store().Node(id)
		if !ok || n.Terminal == nil || !agentbus.IsAgent(n.Terminal.Agent) {
			return "", false
		}
		return n.Terminal.Agent, true
	})
	srv.SetAttention(mon.Attention)
	srv.SetOnAnswer(func(id string, c attention.Choice) {
		asks.answered(id, c)
		if c == attention.Deny {
			mon.Denied(id)
		}
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
	// Callsigns come from the role's abbreviation (PLN, IMP…).
	srv.SetRoleAbbr(func(id string) string {
		if o.Roles != nil && id != "" {
			if r, ok := o.Roles.Get(id); ok {
				return r.Abbr
			}
		}
		return ""
	})
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
	registerStacks(srv, notesSvc)
	// Naming a note the user made: its first name names the file; renaming
	// later keeps the file and the old name (agents still find the note).
	srv.HandleSession("PATCH /api/notes/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, "invalid body")
			return
		}
		n, err := notesSvc.Rename(r.PathValue("id"), body.Title)
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeErrorJSON(w, http.StatusNotFound, "note not found")
		case errors.Is(err, store.ErrInvalid):
			writeErrorJSON(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, store.ErrDuplicate), errors.Is(err, notes.ErrAgentNote):
			writeErrorJSON(w, http.StatusConflict, err.Error())
		case err != nil:
			writeErrorJSON(w, http.StatusInternalServerError, err.Error())
		default:
			writeJSON(w, n)
		}
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
		launcher.InCopy = func() bool { _, ok := srv.Store().Copy(); return ok }
		srv.SetLaunchHook(func(n *store.Node, spec *terminal.Spec) (bool, error) {
			// What the user allowed "always" on the ground floor reaches the
			// floor (or the session's copy) the flight works in.
			if n.Terminal != nil && (n.Terminal.Agent == "claude" || n.Terminal.Agent == "devin") {
				floorsSvc.SyncRules(n.Floor)
			}
			return launcher.Prepare(n, spec)
		})
	}
	srv.SetHookHandler(agents.HookHandler(agents.HookOptions{
		Monitor:  mon,
		CallerOf: server.CallerID,
		Role: func(id string) (string, bool) {
			n, ok := srv.Store().Node(id)
			if !ok || n.Terminal == nil || !agents.Lookup(n.Terminal.Agent).Roles {
				return "", false
			}
			return roleOf(n)
		},
		Conversation: func(id, conv, source string) {
			if agents.ValidConversation(conv) {
				srv.SetConversation(id, conv, source)
			}
		},
		Busy: srv.SetBusy,
	}))
	bus.SetHookedAgents(agents.HasHooks)

	var app *App
	srv.HandleSession("POST /api/nodes/{id}/send", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.handleSend(w, r) }))

	// Agents interrupted mid-turn (Torre closed or restarted): continue them,
	// or forget it.
	srv.HandleSession("POST /api/interrupted/{action}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		if action != "continue" && action != "dismiss" {
			writeErrorJSON(w, http.StatusNotFound, "unknown action")
			return
		}
		var body struct {
			IDs []string `json:"ids"` // empty: all of them
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)
		done := []string{}
		for _, id := range srv.Interrupted() {
			if len(body.IDs) > 0 && !slices.Contains(body.IDs, id) {
				continue
			}
			if action == "continue" {
				if err := app.Send(id, ComposeRequest{Text: continueText}); err != nil {
					continue
				}
			}
			srv.DismissInterrupted(id)
			done = append(done, id)
		}
		writeJSON(w, map[string]any{"ids": done})
	}))

	srv.HandleSession("POST /api/conversations/pause", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bus.SetPaused(true, "user")
		writeJSON(w, bus.StateExtra())
	}))
	srv.HandleSession("POST /api/conversations/resume", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bus.SetPaused(false, "user")
		writeJSON(w, bus.StateExtra())
	}))
	// Requests in the Rádio: the full message, and cancelling one.
	registerRadio(srv, radioSvc, bus)
	srv.HandleSession("POST /api/requests/{id}/cancel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !bus.Cancel(r.PathValue("id")) {
			writeErrorCode(w, http.StatusNotFound, "request.gone", "the request already ended")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// Flight plans: save this canvas (or part of it), and put one in the air.
	if o.Plans != nil {
		srv.HandleSession("POST /api/plans", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.handlePlanSave(w, r) }))
		srv.HandleSession("POST /api/plans/{id}/apply", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.handlePlanApply(w, r) }))
	}
	// Limits (Configurações → Limites): the workspace's, else the server's.
	srv.HandleSession("GET /api/limits", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.handleLimits(w, r) }))
	srv.HandleSession("PUT /api/limits", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { app.handleLimits(w, r) }))
	bl := bus.Limits()
	app = &App{Server: srv, Monitor: mon, Bus: bus, Notes: notesSvc, Portals: portals, Floors: floorsSvc, Summary: summaries, Team: teamSvc, Radio: radioSvc, exts: exts, plans: o.Plans, roles: o.Roles,
		defaults: store.Limits{MaxHops: bl.MaxHops, PerMinute: bl.PerMinute, MaxTeam: teamSvc.MaxTeam(), PortalsPerAgent: portal.MaxCreated}}
	app.applyLimits()
	return app, nil
}

// continueText is what an agent interrupted mid-turn gets when the user
// continues it.
const continueText = "O seu trabalho foi interrompido (o terminal reiniciou). Continue de onde parou."

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
	a.Radio.Close()
	a.Portals.Close()
	a.Floors.Close()
	a.Server.Shutdown()
}

// Run executes a CLI command as a terminal (used by tests).
func (a *App) Run(ctx context.Context, caller string, args ...string) agentbus.Result {
	return a.Bus.Dispatch(ctx, caller, args, a.exts...)
}

func writeErrorJSON(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeErrorCode writes an error with a code the web translates.
func writeErrorCode(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/nathanribeiroo/squad/internal/apierr"
)

// Workspaces off screen: a workspace runs its agents while a window shows
// it, and stops them (app.Sleep) once nobody looks at it and nothing has
// gone on in it for the time set in Configurações (settings.go): no agent in
// a turn or asking the user, no request waiting, no floor being prepared.
// Shells and own commands keep running. When a window shows it again, its
// agents start through the queue (launch.go), each continuing its
// conversation. Torre opens every workspace with its agents stopped, except
// those kept active (one by one, or all with the setting).
//
// A window shows a workspace while it holds that workspace's event socket
// (/w/<id>/ws/events), so a closed window leaves its workspace too.

// SleepOptions tune when workspaces off screen stop their agents; zero
// values take the defaults.
type SleepOptions struct {
	Check time.Duration // how often the rule runs (5 s)
	Idle  time.Duration // overrides the setting's minutes (tests)
	Slots int           // agents starting at once (defaultSlots)
	Quiet time.Duration // a screen still this long after the agent drew it: up (0.5 s)
	Ready time.Duration // the longest wait for an agent to come up (15 s)
}

func (o *SleepOptions) defaults() {
	if o.Check == 0 {
		o.Check = 5 * time.Second
	}
	if o.Slots <= 0 {
		o.Slots = defaultSlots()
	}
	if o.Quiet == 0 {
		o.Quiet = 500 * time.Millisecond
	}
	if o.Ready == 0 {
		o.Ready = 15 * time.Second
	}
}

// presence is where a workspace stands: how many windows show it, whether
// its agents run, and since when it has been off screen with nothing going
// on.
type presence struct {
	viewers int
	awake   bool
	idle    time.Time
	dropped []string // agents left waiting when it went off screen
}

// The states of a workspace in its summary.
const (
	StateActive   = "active"   // its agents run
	StateLoading  = "loading"  // its agents are starting
	StateInactive = "inactive" // its agents are stopped
)

// presenceOfLocked returns a workspace's presence (h.pmu held).
func (h *Hub) presenceOfLocked(id string) *presence {
	p := h.pres[id]
	if p == nil {
		p = &presence{idle: time.Now()}
		h.pres[id] = p
	}
	return p
}

// kept reports whether a workspace never stops its agents.
func (h *Hub) kept(w *workspace) bool {
	return h.WorkspaceSettings().KeepAll || w.app.Server.Store().Meta().KeepActive
}

// view counts a window showing a workspace until the returned function is
// called. The first one wakes it up.
func (h *Hub) view(id string) (leave func()) {
	h.pmu.Lock()
	p := h.presenceOfLocked(id)
	p.viewers++
	first := p.viewers == 1
	h.pmu.Unlock()
	h.mu.RLock()
	started := h.started // else Start wakes it
	h.mu.RUnlock()
	if first && started {
		h.wake(id)
	}
	return func() {
		h.pmu.Lock()
		p := h.presenceOfLocked(id)
		p.viewers--
		last := p.viewers == 0
		if last {
			p.idle = time.Now()
		}
		h.pmu.Unlock()
		if last {
			// Agents still waiting to start wait for its return.
			dropped := h.launches.drop(id)
			h.pmu.Lock()
			p.dropped = append(p.dropped, dropped...)
			h.pmu.Unlock()
		}
	}
}

// wake starts a workspace's agents: all of them if they were stopped, else
// those left waiting when it went off screen.
func (h *Hub) wake(id string) {
	w := h.get(id)
	if w == nil {
		return
	}
	w.s.op.Lock()
	defer w.s.op.Unlock()
	h.wakeLocked(id)
}

// wakeLocked is wake with the workspace's op held (a session change).
func (h *Hub) wakeLocked(id string) {
	w := h.get(id)
	if w == nil {
		return
	}
	h.pmu.Lock()
	p := h.presenceOfLocked(id)
	was := p.awake
	p.awake, p.idle = true, time.Now()
	dropped := p.dropped
	p.dropped = nil
	h.pmu.Unlock()
	if !was {
		// Shells never stop with the agents: these are the ones that
		// haven't run since Torre (or the session) opened.
		w.app.StartShells()
		h.launches.add(id, w.app, w.app.StoppedAgents())
	} else if len(dropped) > 0 {
		var still []string
		for _, tid := range dropped {
			if !w.app.Server.Running(tid) && w.app.Server.HasTerminal(tid) {
				still = append(still, tid)
			}
		}
		h.launches.add(id, w.app, still)
	}
	h.markDirty()
}

// sleep stops a workspace's agents, unless a window shows it by now; by
// the rule (auto), also unless something started going on or it became kept
// active meanwhile.
func (h *Hub) sleep(id string, auto bool) {
	w := h.get(id)
	if w == nil {
		return
	}
	w.s.op.Lock()
	defer w.s.op.Unlock()
	if w = h.get(id); w == nil {
		return
	}
	h.pmu.Lock()
	shown := h.presenceOfLocked(id).viewers > 0
	h.pmu.Unlock()
	if shown || (auto && (h.kept(w) || !w.app.Work().None())) {
		return
	}
	h.launches.drop(id)
	w.app.Sleep()
	// Only now: a check seeing agents run in a sleeping workspace takes it
	// for awake.
	h.pmu.Lock()
	p := h.presenceOfLocked(id)
	p.awake, p.dropped = false, nil
	h.pmu.Unlock()
	h.log.Info("workspace inactive", "workspace", w.app.Server.Store().Meta().Name)
	h.markDirty()
}

// idleFor is how long a workspace off screen with nothing going on keeps
// its agents.
func (h *Hub) idleFor() time.Duration {
	if h.opts.Sleep.Idle > 0 {
		return h.opts.Sleep.Idle
	}
	return time.Duration(h.WorkspaceSettings().IdleMinutes) * time.Minute
}

// sleepLoop applies the rule every Check.
func (h *Hub) sleepLoop() {
	t := time.NewTicker(h.opts.Sleep.Check)
	defer t.Stop()
	for {
		select {
		case <-h.quit:
			return
		case <-t.C:
			h.checkSleep(time.Now())
		}
	}
}

// checkSleep stops the agents of each workspace that has been off screen,
// with nothing going on, long enough.
func (h *Hub) checkSleep(now time.Time) {
	h.mu.RLock()
	ids := make([]string, 0, len(h.ws))
	for id := range h.ws {
		ids = append(ids, id)
	}
	h.mu.RUnlock()
	idle := h.idleFor()
	for _, id := range ids {
		w := h.get(id)
		if w == nil {
			continue
		}
		h.pmu.Lock()
		p := h.presenceOfLocked(id)
		awake, viewers, since := p.awake, p.viewers, p.idle
		h.pmu.Unlock()
		_, loading := h.launches.progress(id)
		switch {
		case !awake:
			// Something started an agent anyway (a new terminal): it
			// stops by the same rule.
			if !w.app.AgentsRunning() {
				continue
			}
			h.pmu.Lock()
			p.awake, p.idle = true, now
			h.pmu.Unlock()
			continue
		case viewers > 0 || h.kept(w) || loading || !w.app.Work().None():
			h.pmu.Lock()
			p.idle = now
			h.pmu.Unlock()
			continue
		case now.Sub(since) < idle:
			continue
		}
		h.sleep(id, true)
	}
}

// Sleep stops a workspace's agents now (Inativar agora). Unless confirmed,
// it refuses while agents work or requests wait (a BusyError); a workspace
// on screen can't sleep.
func (h *Hub) Sleep(id string, confirm bool) (Summary, error) {
	w := h.get(id)
	if w == nil {
		return Summary{}, ErrNotFound
	}
	h.pmu.Lock()
	viewers := h.presenceOfLocked(id).viewers
	h.pmu.Unlock()
	if viewers > 0 {
		return Summary{}, apierr.With(invalidf("the workspace is on screen"), "workspace.on_screen")
	}
	if !confirm {
		if work := w.app.Work(); len(work.Agents) > 0 || work.Pending > 0 {
			return Summary{}, &BusyError{Working: work.Agents, Pending: work.Pending}
		}
	}
	h.sleep(id, false)
	return h.Get(id)
}

// handleSleep is POST /api/workspaces/{id}/sleep {"confirm"}.
func (h *Hub) handleSleep(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Confirm bool `json:"confirm"`
	}
	_ = readJSON(r, &b) // an empty body doesn't confirm
	s, err := h.Sleep(r.PathValue("id"), b.Confirm)
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// handleWakeOrder is POST /api/workspaces/{id}/wake/order {"ids"}: the
// window starts first the flights it shows (in focus, docked, visible).
func (h *Hub) handleWakeOrder(w http.ResponseWriter, r *http.Request) {
	var b struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.get(r.PathValue("id")) == nil {
		writeHubError(w, ErrNotFound)
		return
	}
	h.launches.first(r.PathValue("id"), b.IDs)
	w.WriteHeader(http.StatusNoContent)
}

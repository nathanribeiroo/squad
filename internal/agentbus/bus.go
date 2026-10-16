// Package agentbus implements the conversation between terminals (feature 1,
// docs/03): list, ask (blocking), ask --raw, ask --batch, check and reply,
// with pending requests, ask-back and deadlock-cycle release, hop and rate
// limits, and pause/resume. It is reached through POST /cli.
package agentbus

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Limits are the tunables of the conversation; zero values take defaults.
type Limits struct {
	MaxHops      int           // max chained requests (6); beyond it conversations pause
	PerMinute    int           // max requests per minute (30); beyond it requests wait
	Timeout      time.Duration // base wait for a reply (120 s), extended while the target works
	MaxWait      time.Duration // hard cap for a blocking ask (30 min)
	QuickAfter   time.Duration // first screen-settled check (5 s)
	QuickEvery   time.Duration // later checks (1 s)
	Stable       time.Duration // screen unchanged this long → reply by screen (5 s)
	ActiveWindow time.Duration // output within this → extend the timeout (10 s)
	RawDelay     time.Duration // wait before reading the screen after --raw (300 ms)
}

func (l *Limits) defaults() {
	set := func(d *time.Duration, v time.Duration) {
		if *d == 0 {
			*d = v
		}
	}
	if l.MaxHops == 0 {
		l.MaxHops = 6
	}
	if l.PerMinute == 0 {
		l.PerMinute = 30
	}
	set(&l.Timeout, 120*time.Second)
	set(&l.MaxWait, 30*time.Minute)
	set(&l.QuickAfter, 5*time.Second)
	set(&l.QuickEvery, time.Second)
	set(&l.Stable, 5*time.Second)
	set(&l.ActiveWindow, 10*time.Second)
	set(&l.RawDelay, 300*time.Millisecond)
}

// Result is what a waiting ask receives.
type Result struct {
	Code   int
	Body   string
	Source string // hook name, idle, screen, reply, ask-back, ring-break, timeout, cancelled
}

// Pending is an outstanding blocking request.
type Pending struct {
	ID       string    `json:"id"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	FromName string    `json:"fromName"`
	ToName   string    `json:"toName"`
	Preview  string    `json:"preview"`
	Hop      int       `json:"hop"`
	SentAt   time.Time `json:"sentAt"`

	done     chan Result
	resolved bool
}

// Bus coordinates requests between terminals.
type Bus struct {
	limits Limits
	store  *store.Store
	terms  *terminal.Manager
	events *events.Bus
	log    *slog.Logger
	mon    *activity.Monitor

	// hasHooks reports whether an agent type signals turn ends explicitly
	// (set by the adapters); for those the screen-settled fallback is off.
	hasHooks func(agent string) bool
	roleName func(id string) string

	mu      sync.Mutex
	pending map[string]*Pending
	paused  bool
	resume  chan struct{}
	sends   []time.Time
}

// New creates a bus. Call SetMonitor before serving.
func New(st *store.Store, terms *terminal.Manager, ev *events.Bus, log *slog.Logger, limits Limits) *Bus {
	limits.defaults()
	if log == nil {
		log = slog.Default()
	}
	return &Bus{limits: limits, store: st, terms: terms, events: ev, log: log,
		hasHooks: func(string) bool { return false },
		pending:  map[string]*Pending{}, resume: make(chan struct{})}
}

// SetMonitor attaches the activity monitor.
func (b *Bus) SetMonitor(m *activity.Monitor) { b.mon = m }

// SetHookedAgents tells which agent types report turn ends through hooks.
func (b *Bus) SetHookedAgents(fn func(agent string) bool) { b.hasHooks = fn }

// SetRoleNames sets how `squad list` names a role id ("test" → "Testador").
func (b *Bus) SetRoleNames(fn func(id string) string) { b.roleName = fn }

func (b *Bus) roleLabel(id string) string {
	if b.roleName != nil {
		if name := b.roleName(id); name != "" {
			return name
		}
	}
	return id
}

// Limits returns the effective limits.
func (b *Bus) Limits() Limits { return b.limits }

// IsWaiting reports whether terminal id has an outstanding request.
func (b *Bus) IsWaiting(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range b.pending {
		if p.From == id && !p.resolved {
			return true
		}
	}
	return false
}

// Pending returns a copy of the outstanding requests, oldest first.
func (b *Bus) Pending() []Pending {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Pending, 0, len(b.pending))
	for _, p := range b.pending {
		cp := *p
		cp.done = nil
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentAt.Before(out[j].SentAt) })
	return out
}

// Paused reports whether conversations are paused.
func (b *Bus) Paused() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.paused
}

// SetPaused pauses or resumes conversations; held requests continue on resume.
func (b *Bus) SetPaused(p bool, reason string) {
	b.mu.Lock()
	if b.paused == p {
		b.mu.Unlock()
		return
	}
	b.paused = p
	if !p {
		close(b.resume)
		b.resume = make(chan struct{})
	}
	b.mu.Unlock()
	b.events.Publish(events.ConversationsPaused, map[string]any{"paused": p, "reason": reason})
}

// StateExtra is merged into /api/state and the events hello message.
func (b *Bus) StateExtra() map[string]any {
	b.mu.Lock()
	recent := b.recentSendsLocked(time.Now())
	paused := b.paused
	b.mu.Unlock()
	return map[string]any{
		"paused":  paused,
		"limits":  map[string]int{"maxHops": b.limits.MaxHops, "perMinute": b.limits.PerMinute},
		"rate":    recent,
		"pending": b.Pending(),
	}
}

func (b *Bus) recentSendsLocked(now time.Time) int {
	cut := now.Add(-time.Minute)
	i := 0
	for i < len(b.sends) && b.sends[i].Before(cut) {
		i++
	}
	b.sends = b.sends[i:]
	return len(b.sends)
}

// waitSlot blocks while conversations are paused or the per-minute rate is
// exceeded, then records the send. It returns ctx.Err() if the caller gives up.
func (b *Bus) waitSlot(ctx context.Context, held func()) error {
	notified := false
	for {
		b.mu.Lock()
		now := time.Now()
		if !b.paused {
			if b.recentSendsLocked(now) < b.limits.PerMinute {
				b.sends = append(b.sends, now)
				b.mu.Unlock()
				return nil
			}
		}
		resume := b.resume
		var wait <-chan time.Time
		if !b.paused && len(b.sends) > 0 {
			wait = time.After(time.Until(b.sends[0].Add(time.Minute)) + 10*time.Millisecond)
		}
		b.mu.Unlock()
		if !notified {
			held()
			notified = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-resume:
		case <-wait:
		}
	}
}

func (b *Bus) newID() string {
	for {
		id := ids.Short(4)
		if _, taken := b.pending[id]; !taken {
			return id
		}
	}
}

// resolve completes a pending request once.
func (b *Bus) resolve(p *Pending, r Result) bool {
	b.mu.Lock()
	if p.resolved {
		b.mu.Unlock()
		return false
	}
	p.resolved = true
	delete(b.pending, p.ID)
	b.mu.Unlock()
	p.done <- r
	data := map[string]any{"id": p.ID, "from": p.From, "to": p.To, "fromName": p.FromName,
		"toName": p.ToName, "source": r.Source, "elapsedMs": time.Since(p.SentAt).Milliseconds()}
	if r.Source == "cancelled" {
		b.events.Publish(events.RequestCancelled, data)
	} else {
		b.events.Publish(events.ReplyReceived, data)
	}
	if b.mon != nil {
		b.mon.Refresh()
	}
	return true
}

// incomingHopLocked returns the hop count of the request that caller is
// currently handling (0 if none).
func (b *Bus) incomingHopLocked(caller string) int {
	hop := 0
	for _, p := range b.pending {
		if p.To == caller && !p.resolved && p.Hop > hop {
			hop = p.Hop
		}
	}
	return hop
}

// reachableLocked reports whether `to` is reachable from `from` following
// pending requests (origin → destination edges).
func (b *Bus) reachableLocked(from, to string) bool {
	seen := map[string]bool{}
	stack := []string{from}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == to {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for _, p := range b.pending {
			if p.From == cur && !p.resolved {
				stack = append(stack, p.To)
			}
		}
	}
	return false
}

// EdgeRemoved cancels requests travelling over a removed connection.
func (b *Bus) EdgeRemoved(e store.Edge) {
	b.cancelWhere(func(p *Pending) bool {
		return (p.From == e.A && p.To == e.B) || (p.From == e.B && p.To == e.A)
	}, "connection removed; request cancelled")
}

// NodeRemoved cancels requests involving a removed node.
func (b *Bus) NodeRemoved(n store.Node) {
	b.cancelWhere(func(p *Pending) bool { return p.From == n.ID || p.To == n.ID },
		"terminal removed; request cancelled")
}

func (b *Bus) cancelWhere(match func(*Pending) bool, msg string) {
	b.mu.Lock()
	var hit []*Pending
	for _, p := range b.pending {
		if match(p) {
			hit = append(hit, p)
		}
	}
	b.mu.Unlock()
	for _, p := range hit {
		b.resolve(p, Result{Code: 409, Body: msg, Source: "cancelled"})
	}
}

// connectedTerminal resolves a terminal node by name that must be connected
// to caller. It returns an HTTP status and message on failure.
func (b *Bus) connectedTerminal(caller, name string) (store.Node, int, string) {
	n, ok := b.store.TerminalByName(name)
	if !ok {
		return n, 404, "squad: no terminal named " + quote(name) + " (run `squad list` for exact names)"
	}
	if n.ID == caller {
		return n, 400, "squad: you can't message yourself"
	}
	if !b.store.Connected(caller, n.ID) {
		return n, 403, "squad: " + quote(name) + " is not connected to you; connect the two terminals on the canvas first"
	}
	return n, 0, ""
}

func quote(s string) string { return `"` + s + `"` }

func preview(s string) string {
	r := []rune(s)
	if len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return s
}

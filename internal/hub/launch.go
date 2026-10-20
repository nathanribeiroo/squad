package hub

import (
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/app"
)

// Starting agents: a workspace coming back on screen (or opening with Torre)
// starts its agents a few at a time, the next one as soon as one is up.
// Starting them all at once is no faster: past the number of CPUs every
// agent takes longer and the computer, the Torre window included, freezes
// meanwhile. One queue serves every workspace.

// An agent is up once it drew its screen (startedBytes, more than the shell's
// prompt and the launch line) and kept it still for a moment (Quiet); it
// gives its place up when it stops, or after a while however it is. Its
// session start hook comes too early to tell: with 8 real Claude Codes on 4
// CPUs, going on with the hook kept the CPU at 97% (3.6 s in all), a still
// screen for 0.5 s at 74% (4.2 s), and all at once at 99% (3 s).
const startedBytes = 1000

// defaultSlots is how many agents start at once: one per CPU but one, left
// for the window, from 2 to 6.
func defaultSlots() int { return min(max(runtime.NumCPU()-1, 2), 6) }

type launch struct {
	ws  string
	app *app.App
	id  string
	at  time.Time // when it started (zero: still waiting)
}

// round is the agents a workspace is starting: how many it started and how
// many are up.
type round struct{ ready, total int }

type launcher struct {
	slots        int
	quiet, limit time.Duration
	changed      func() // a workspace's progress changed

	// starting is held while agents start, so that an app forgotten (about
	// to close) never gets a terminal started after: taken before mu.
	starting sync.Mutex
	mu       sync.Mutex
	queue    []*launch
	running  []*launch
	rounds   map[string]*round
	kick     chan struct{}
}

func newLauncher(o SleepOptions, changed func()) *launcher {
	return &launcher{slots: o.Slots, quiet: o.Quiet, limit: o.Ready, changed: changed,
		rounds: map[string]*round{}, kick: make(chan struct{}, 1)}
}

func (l *launcher) poke() {
	select {
	case l.kick <- struct{}{}:
	default:
	}
}

// queuedLocked reports whether a terminal is waiting or starting.
func (l *launcher) queuedLocked(id string) bool {
	has := func(x *launch) bool { return x.id == id }
	return slices.ContainsFunc(l.queue, has) || slices.ContainsFunc(l.running, has)
}

// add queues a workspace's agents, in that order, after those already
// waiting.
func (l *launcher) add(ws string, a *app.App, ids []string) {
	l.mu.Lock()
	n := 0
	for _, id := range ids {
		if l.queuedLocked(id) {
			continue
		}
		l.queue = append(l.queue, &launch{ws: ws, app: a, id: id})
		n++
	}
	if n > 0 {
		r := l.rounds[ws]
		if r == nil {
			r = &round{}
			l.rounds[ws] = r
		}
		r.total += n
	}
	l.mu.Unlock()
	if n > 0 {
		l.poke()
		l.changed()
	}
}

// first puts a workspace's waiting agents in ids' order (those not named
// keep theirs, after them): the window starts what it shows first.
func (l *launcher) first(ws string, ids []string) {
	rank := map[string]int{}
	for i, id := range ids {
		if _, ok := rank[id]; !ok {
			rank[id] = i
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var mine []*launch
	var at []int
	for i, x := range l.queue {
		if x.ws == ws {
			mine = append(mine, x)
			at = append(at, i)
		}
	}
	slices.SortStableFunc(mine, func(a, b *launch) int {
		ra, oka := rank[a.id]
		rb, okb := rank[b.id]
		switch {
		case oka && okb:
			return ra - rb
		case oka:
			return -1
		case okb:
			return 1
		}
		return 0
	})
	for i, pos := range at {
		l.queue[pos] = mine[i]
	}
}

// drop takes a workspace's agents still waiting out of the queue (it left
// the screen) and returns them; those starting go on.
func (l *launcher) drop(ws string) []string {
	l.mu.Lock()
	var out []string
	l.queue = slices.DeleteFunc(l.queue, func(x *launch) bool {
		if x.ws == ws {
			out = append(out, x.id)
			return true
		}
		return false
	})
	if r := l.rounds[ws]; r != nil && len(out) > 0 {
		r.total -= len(out)
		l.endRoundLocked(ws)
	}
	l.mu.Unlock()
	if len(out) > 0 {
		l.changed()
	}
	return out
}

// forget drops everything of an app about to close: once it returns, none
// of its agents is started.
func (l *launcher) forget(a *app.App) {
	l.starting.Lock()
	defer l.starting.Unlock()
	l.mu.Lock()
	gone := map[string]bool{}
	drop := func(x *launch) bool {
		if x.app == a {
			gone[x.ws] = true
			return true
		}
		return false
	}
	l.queue = slices.DeleteFunc(l.queue, drop)
	l.running = slices.DeleteFunc(l.running, drop)
	for ws := range gone {
		delete(l.rounds, ws)
	}
	l.mu.Unlock()
	if len(gone) > 0 {
		l.changed()
	}
}

// progress returns the round of a workspace starting its agents (false:
// none).
func (l *launcher) progress(ws string) (round, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.rounds[ws]
	if r == nil {
		return round{}, false
	}
	return *r, true
}

// endRoundLocked closes a workspace's round once none of its agents waits or
// starts.
func (l *launcher) endRoundLocked(ws string) {
	mine := func(x *launch) bool { return x.ws == ws }
	if !slices.ContainsFunc(l.queue, mine) && !slices.ContainsFunc(l.running, mine) {
		delete(l.rounds, ws)
	}
}

// run starts the agents until quit.
func (l *launcher) run(quit <-chan struct{}) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-quit:
			return
		case <-tick.C:
		case <-l.kick:
		}
		l.step(time.Now())
	}
}

// step lets the agents that came up give their places to the next ones.
func (l *launcher) step(now time.Time) {
	l.mu.Lock()
	changed := false
	l.running = slices.DeleteFunc(l.running, func(x *launch) bool {
		if !l.up(x, now) {
			return false
		}
		if r := l.rounds[x.ws]; r != nil {
			r.ready++
		}
		changed = true
		return true
	})
	var next []*launch
	for len(l.running)+len(next) < l.slots && len(l.queue) > 0 {
		x := l.queue[0]
		l.queue = l.queue[1:]
		x.at = now
		next = append(next, x)
	}
	l.running = append(l.running, next...)
	for ws := range l.rounds {
		l.endRoundLocked(ws)
	}
	l.mu.Unlock()
	if len(next) > 0 {
		l.start(next)
		changed = true
	}
	if changed {
		l.changed()
	}
}

// start runs the agents just given a place, unless their app was forgotten
// meanwhile.
func (l *launcher) start(next []*launch) {
	l.starting.Lock()
	defer l.starting.Unlock()
	for _, x := range next {
		l.mu.Lock()
		live := slices.Contains(l.running, x)
		l.mu.Unlock()
		// Started meanwhile (by the user, or a new canvas): nothing to do.
		if live && !x.app.Server.Running(x.id) {
			_ = x.app.Server.StartTerminal(x.id) // failures are reported as terminal state
		}
	}
}

// up reports whether a starting agent no longer needs its place.
func (l *launcher) up(x *launch, now time.Time) bool {
	if now.Sub(x.at) >= l.limit {
		return true
	}
	t := x.app.Server.Terminals().Get(x.id)
	if t == nil || !x.app.Server.Running(x.id) {
		return now.Sub(x.at) > time.Second // gone, or failed to start (a moment to begin)
	}
	bytes, last := t.Output()
	return bytes >= startedBytes && now.Sub(last) >= l.quiet
}

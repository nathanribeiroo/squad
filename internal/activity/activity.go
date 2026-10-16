// Package activity watches terminals and decides when an agent is working
// and when its turn ended, from screen changes (see docs/03 §8), window-title
// spinners and explicit signals (agent hooks). It also derives the state the
// UI shows for each terminal node.
package activity

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/terminal"
)

// State is what the UI shows on a terminal node.
type State string

const (
	Boot State = "boot" // starting, no output yet
	Idle State = "idle"
	Work State = "work" // agent is producing output
	Wait State = "wait" // waiting for a reply to its own request
	Done State = "done" // finished a turn, nothing new since
	Err  State = "err"  // exited with an error
	Off  State = "off"  // stopped / not running
)

// Options tune the detection; zero values take the defaults from docs/03.
type Options struct {
	Tick         time.Duration // sampling interval (1 s)
	ByteThresh   int64         // bytes per sample to count as active (500)
	ActiveStreak int           // consecutive active samples to enter "working" (3)
	IdleAfter    time.Duration // no meaningful change while working → turn end (3 s)
	BootGrace    time.Duration // max time shown as Boot without output (30 s)
	DoneFor      time.Duration // how long Done lasts without new input (10 min)
	HookTimeout  time.Duration // silence that ends a hook-started turn without its end hook (60 s)
}

func (o *Options) defaults() {
	if o.Tick == 0 {
		o.Tick = time.Second
	}
	if o.ByteThresh == 0 {
		o.ByteThresh = 500
	}
	if o.ActiveStreak == 0 {
		o.ActiveStreak = 3
	}
	if o.IdleAfter == 0 {
		o.IdleAfter = 3 * time.Second
	}
	if o.BootGrace == 0 {
		o.BootGrace = 30 * time.Second
	}
	if o.DoneFor == 0 {
		o.DoneFor = 10 * time.Minute
	}
	if o.HookTimeout == 0 {
		o.HookTimeout = 60 * time.Second
	}
}

// TurnEnd describes the end of a turn.
type TurnEnd struct {
	ID      string
	Reason  string // "idle" (screen settled) or a hook name ("claude-stop", "codex-notify", "devin-stop")
	Message string // final assistant message when a hook provides it
	At      time.Time
	Start   time.Time // when the turn started (zero if unknown)
	Prompt  string    // what started it, when a hook reported it (the user's prompt)
}

// Monitor samples terminals periodically.
type Monitor struct {
	opts      Options
	terms     *terminal.Manager
	isWaiting func(id string) bool
	onState   func(id string, s State, since time.Time)

	mu sync.Mutex
	st map[string]*track

	listeners []func(TurnEnd) // every turn end (OnAnyTurnEnd)

	quit chan struct{}
	once sync.Once
}

type track struct {
	bytes          int64
	hash           uint64
	title          string
	titleChanged   bool
	streak         int
	working        bool
	lastMeaningful time.Time
	lastChange     time.Time // last hash change (stable-for)
	firstOutput    bool
	turnEndedAt    time.Time
	turnStart      time.Time // when the current (or last) turn started
	prompt         string    // the prompt that started it, from a hook
	hookTurn       bool      // turn started by a hook: only a hook (or HookTimeout of silence) ends it
	state          State
	since          time.Time
	waiters        map[int]func(TurnEnd)
	nextWaiter     int
}

// New creates a monitor. isWaiting reports whether a terminal has an
// outstanding request of its own; onState is called when a node's UI state
// changes. Both may be nil.
func New(terms *terminal.Manager, opts Options, isWaiting func(string) bool, onState func(string, State, time.Time)) *Monitor {
	opts.defaults()
	if isWaiting == nil {
		isWaiting = func(string) bool { return false }
	}
	if onState == nil {
		onState = func(string, State, time.Time) {}
	}
	return &Monitor{opts: opts, terms: terms, isWaiting: isWaiting, onState: onState,
		st: map[string]*track{}, quit: make(chan struct{})}
}

// Start begins sampling.
func (m *Monitor) Start() {
	go func() {
		t := time.NewTicker(m.opts.Tick)
		defer t.Stop()
		for {
			select {
			case <-m.quit:
				return
			case <-t.C:
				m.Poll()
			}
		}
	}()
}

// Stop ends sampling.
func (m *Monitor) Stop() { m.once.Do(func() { close(m.quit) }) }

func (m *Monitor) get(id string) *track {
	tr := m.st[id]
	if tr == nil {
		now := time.Now()
		tr = &track{waiters: map[int]func(TurnEnd){}, lastChange: now, since: now}
		m.st[id] = tr
	}
	return tr
}

// spinnerTitle reports whether a window title starts with a spinner glyph
// (braille dots, half circles, stars), which agent CLIs animate while busy.
func spinnerTitle(title string) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(title))
	switch {
	case r >= 0x2800 && r <= 0x28FF: // braille
		return true
	case strings.ContainsRune("◐◓◑◒⏳", r):
		return true
	}
	return false
}

// Poll samples every terminal once (called by the ticker; exported for tests).
func (m *Monitor) Poll() {
	now := time.Now()
	type fire struct {
		fns []func(TurnEnd)
		te  TurnEnd
	}
	var fires []fire
	type change struct {
		id    string
		state State
		since time.Time
	}
	var changes []change

	live := map[string]bool{}
	for _, t := range m.terms.List() {
		id := t.ID()
		live[id] = true
		bytes, _ := t.Output()
		scr := t.Screen()
		var hash uint64
		var title string
		if scr != nil {
			hash = scr.Hash()
			title = scr.Title()
		}
		waiting := m.isWaiting(id)

		m.mu.Lock()
		tr := m.get(id)
		delta := bytes - tr.bytes
		hashChanged := hash != tr.hash
		titleSpin := title != tr.title && spinnerTitle(title)
		tr.bytes, tr.hash, tr.title = bytes, hash, title
		if bytes > 0 {
			tr.firstOutput = true
		}
		if hashChanged {
			tr.lastChange = now
		}
		active := (delta >= m.opts.ByteThresh && hashChanged) || titleSpin
		if active {
			tr.streak++
		} else {
			tr.streak = 0
		}
		if tr.streak >= m.opts.ActiveStreak {
			if !tr.working {
				tr.working = true
				tr.turnEndedAt = time.Time{}
				tr.turnStart, tr.prompt = now, ""
			}
			tr.lastMeaningful = now
		} else if tr.working && (hashChanged || titleSpin) {
			tr.lastMeaningful = now
		}
		idleLimit := m.opts.IdleAfter
		if tr.hookTurn {
			idleLimit = m.opts.HookTimeout
		}
		if tr.working && now.Sub(tr.lastMeaningful) > idleLimit {
			tr.working, tr.hookTurn = false, false
			tr.turnEndedAt = now
			te := TurnEnd{ID: id, Reason: "idle", At: now, Start: tr.turnStart, Prompt: tr.prompt}
			fires = append(fires, fire{fns: append(m.takeWaiters(tr), m.listeners...), te: te})
		}

		st, code := t.State()
		next := m.uiState(tr, st, code, waiting, now)
		if next != tr.state {
			tr.state, tr.since = next, now
			changes = append(changes, change{id, next, now})
		}
		m.mu.Unlock()
	}
	m.mu.Lock()
	for id := range m.st {
		if !live[id] {
			delete(m.st, id)
		}
	}
	m.mu.Unlock()

	for _, f := range fires {
		for _, fn := range f.fns {
			fn(f.te)
		}
	}
	for _, c := range changes {
		m.onState(c.id, c.state, c.since)
	}
}

func (m *Monitor) uiState(tr *track, st terminal.Status, code int, waiting bool, now time.Time) State {
	switch st {
	case terminal.StatusFailed:
		return Err
	case terminal.StatusExited:
		if code != 0 {
			return Err
		}
		return Off
	case terminal.StatusStopped:
		return Off
	}
	switch {
	case waiting:
		return Wait
	case tr.working:
		return Work
	case !tr.firstOutput && now.Sub(tr.since) < m.opts.BootGrace && tr.state != Idle && tr.state != Done:
		return Boot
	case !tr.turnEndedAt.IsZero() && now.Sub(tr.turnEndedAt) < m.opts.DoneFor:
		return Done
	}
	return Idle
}

func (m *Monitor) takeWaiters(tr *track) []func(TurnEnd) {
	fns := make([]func(TurnEnd), 0, len(tr.waiters))
	for k, fn := range tr.waiters {
		fns = append(fns, fn)
		delete(tr.waiters, k)
	}
	return fns
}

// OnTurnEnd registers fn for the next turn end of terminal id.
func (m *Monitor) OnTurnEnd(id string, fn func(TurnEnd)) (cancel func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := m.get(id)
	k := tr.nextWaiter
	tr.nextWaiter++
	tr.waiters[k] = fn
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if tr := m.st[id]; tr != nil {
			delete(tr.waiters, k)
		}
	}
}

// ResetTurn forgets a terminal's current activity so only a fresh turn
// counts (called right before delivering a message).
func (m *Monitor) ResetTurn(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := m.get(id)
	tr.working, tr.hookTurn, tr.streak = false, false, 0
	tr.lastChange = time.Now()
	tr.turnEndedAt = time.Time{}
}

// OnAnyTurnEnd calls fn after every turn end of every terminal (after the
// waiters of that terminal). Register listeners before Start.
func (m *Monitor) OnAnyTurnEnd(fn func(TurnEnd)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

// SignalStart marks the start of a turn reported by a hook, with the prompt
// that started it when known; the turn then only ends with Signal (or after
// HookTimeout without screen changes).
func (m *Monitor) SignalStart(id, prompt string) {
	now := time.Now()
	m.mu.Lock()
	tr := m.get(id)
	tr.turnStart, tr.prompt = now, prompt
	tr.working, tr.hookTurn = true, true
	tr.streak = m.opts.ActiveStreak
	tr.lastMeaningful = now
	tr.turnEndedAt = time.Time{}
	m.mu.Unlock()
	m.Refresh()
}

// Signal ends the current turn of a terminal explicitly (agent hooks).
func (m *Monitor) Signal(te TurnEnd) {
	if te.At.IsZero() {
		te.At = time.Now()
	}
	m.mu.Lock()
	tr := m.get(te.ID)
	tr.working, tr.hookTurn, tr.streak = false, false, 0
	tr.turnEndedAt = te.At
	if te.Start.IsZero() {
		te.Start = tr.turnStart
	}
	if te.Prompt == "" {
		te.Prompt = tr.prompt
	}
	fns := append(m.takeWaiters(tr), m.listeners...)
	m.mu.Unlock()
	for _, fn := range fns {
		fn(te)
	}
	m.Refresh()
}

// StableFor returns how long the terminal's screen has not changed.
func (m *Monitor) StableFor(id string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Since(m.get(id).lastChange)
}

// Working reports whether the terminal is in a working turn.
func (m *Monitor) Working(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.get(id).working
}

// State returns the UI state of a terminal and since when.
func (m *Monitor) State(id string) (State, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := m.get(id)
	if tr.state == "" {
		return Boot, tr.since
	}
	return tr.state, tr.since
}

// Refresh recomputes the UI states now without taking a sample (sampling
// off-cycle would distort the per-tick byte counts). Called when a request
// starts or stops waiting.
func (m *Monitor) Refresh() {
	now := time.Now()
	type change struct {
		id    string
		state State
		since time.Time
	}
	var changes []change
	for _, t := range m.terms.List() {
		id := t.ID()
		waiting := m.isWaiting(id)
		st, code := t.State()
		m.mu.Lock()
		tr := m.get(id)
		next := m.uiState(tr, st, code, waiting, now)
		if next != tr.state {
			tr.state, tr.since = next, now
			changes = append(changes, change{id, next, now})
		}
		m.mu.Unlock()
	}
	for _, c := range changes {
		m.onState(c.id, c.state, c.since)
	}
}

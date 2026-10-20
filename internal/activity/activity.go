// Package activity watches terminals and decides when an agent is working
// and when its turn ended, from screen changes (see docs/03 §8), window-title
// spinners and explicit signals (agent hooks). It also derives the state the
// UI shows for each terminal node.
package activity

import (
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// State is what the UI shows on a terminal node.
type State string

const (
	Boot State = "boot" // starting, no output yet
	Idle State = "idle"
	Work State = "work" // agent is producing output
	Wait State = "wait" // waiting for a reply to its own request
	Ask  State = "ask"  // waiting for the user: a permission prompt or a question (PRIORIDADE)
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
	PromptAfter  time.Duration // a prompt read on a still screen for this long is an ask (1.5 s)
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
	if o.PromptAfter == 0 {
		o.PromptAfter = 1500 * time.Millisecond
	}
}

// TurnEnd describes the end of a turn.
type TurnEnd struct {
	ID      string
	Reason  string // "idle" (screen settled), "denied" (settled after the user denied a permission from the interface: final, as no hook ends it) or a hook name ("claude-stop", "codex-notify", "devin-stop")
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
	// agentOf gives a terminal's agent ("claude"…), false for shells and
	// own commands, whose screens are never read for prompts.
	agentOf func(id string) (string, bool)
	// onScreen gets an agent's screen when it changed (SetOnScreen).
	onScreen func(id, agent, snap string)

	mu sync.Mutex
	st map[string]*track

	listeners []func(TurnEnd) // every turn end (OnAnyTurnEnd)

	quit    chan struct{}
	once    sync.Once
	sampler sync.WaitGroup // the sampling goroutine (Start)
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
	open           bool      // a turn that only a hook ends has started and no hook ended it yet (survives HookTimeout)
	state          State
	since          time.Time
	waiters        map[int]func(TurnEnd)
	nextWaiter     int
	// The user is asked something (Ask): from a hook or read on screen.
	attn       *attention.Attention
	attnSeen   bool // its prompt showed on screen
	attnMissed int  // samples since the prompt left the screen
	attnQuiet  bool // the screen went still after it was raised
	// The user denied a permission from the interface: the agent stops
	// without the hook that ends a turn (Claude Code, Devin), so the turn
	// ends with the screen's quiet, as a turn without hooks does.
	denied bool
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

// SetAgentOf installs the lookup of a terminal's agent (before Start): only
// agents' screens are read for prompts.
func (m *Monitor) SetAgentOf(fn func(id string) (string, bool)) { m.agentOf = fn }

// SetOnScreen installs a reader of agents' screens (before Start): fn gets
// an agent's screen each time it changed.
func (m *Monitor) SetOnScreen(fn func(id, agent, snap string)) { m.onScreen = fn }

// Start begins sampling.
func (m *Monitor) Start() {
	m.sampler.Add(1)
	go func() {
		defer m.sampler.Done()
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

// Stop ends sampling. A sample in progress finishes first, so no turn end
// or state change is reported from sampling after Stop returns (its
// listeners write files the caller may be about to remove).
func (m *Monitor) Stop() {
	m.once.Do(func() { close(m.quit) })
	m.sampler.Wait()
}

// Reset forgets what was seen of a stopped terminal (its turn, prompt,
// output), as if it had just appeared stopped: for an agent stopped on
// purpose, which continues its conversation from scratch later. Waiters for
// its turn end are dropped.
func (m *Monitor) Reset(id string) {
	now := time.Now()
	m.mu.Lock()
	m.st[id] = &track{waiters: map[int]func(TurnEnd){}, lastChange: now, since: now, state: Off}
	m.mu.Unlock()
}

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
	type screen struct{ id, agent, snap string }
	var screens []screen

	live := map[string]bool{}
	for _, t := range m.terms.List() {
		id := t.ID()
		live[id] = true
		bytes, _ := t.Output()
		scr := t.Screen()
		var hash uint64
		var title, snap string
		if scr != nil {
			hash = scr.Hash()
			title = scr.Title()
		}
		agent, isAgent := "", false
		if m.agentOf != nil {
			agent, isAgent = m.agentOf(id)
		}
		st, code := t.State()
		if isAgent && scr != nil && st == terminal.StatusRunning {
			snap = scr.Snapshot()
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
			if snap != "" && m.onScreen != nil {
				screens = append(screens, screen{id, agent, snap})
			}
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
		attnChanged := false
		if isAgent && st == terminal.StatusRunning {
			attnChanged = m.watchPrompt(tr, agent, snap, now)
		} else if tr.attn != nil {
			m.clearAttention(tr, now)
			attnChanged = true
		}
		idleLimit := m.opts.IdleAfter
		if tr.hookTurn && !tr.denied {
			idleLimit = m.opts.HookTimeout
		}
		// A turn waiting for the user doesn't end for being quiet.
		if tr.working && tr.attn == nil && now.Sub(tr.lastMeaningful) > idleLimit {
			reason := "idle"
			if tr.denied { // no hook will end it
				reason, tr.open, tr.denied = "denied", false, false
			}
			tr.working, tr.hookTurn = false, false
			tr.turnEndedAt = now
			te := TurnEnd{ID: id, Reason: reason, At: now, Start: tr.turnStart, Prompt: tr.prompt}
			fires = append(fires, fire{fns: append(m.takeWaiters(tr), m.listeners...), te: te})
		}

		next := m.uiState(tr, st, code, waiting, now)
		if next != tr.state {
			tr.state, tr.since = next, now
			changes = append(changes, change{id, next, now})
		} else if attnChanged {
			changes = append(changes, change{id, next, tr.since})
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
	for _, sc := range screens {
		m.onScreen(sc.id, sc.agent, sc.snap)
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
	case tr.attn != nil:
		return Ask
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
	tr.working, tr.hookTurn, tr.open, tr.denied = true, true, true, false
	tr.attn, tr.attnSeen, tr.attnMissed, tr.attnQuiet = nil, false, 0, false
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
	tr.working, tr.hookTurn, tr.open, tr.denied, tr.streak = false, false, false, false, 0
	tr.attn, tr.attnSeen, tr.attnMissed, tr.attnQuiet = nil, false, 0, false
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

// Idle returns how long the terminal has been out of a turn: zero while it
// works or waits for the user.
func (m *Monitor) Idle(id string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := m.get(id)
	if tr.working || tr.attn != nil {
		return 0
	}
	last := tr.turnEndedAt
	if tr.lastMeaningful.After(last) {
		last = tr.lastMeaningful
	}
	if last.IsZero() {
		return math.MaxInt64 // never worked
	}
	return time.Since(last)
}

// ExpectTurn marks a turn as started on an agent whose turns end with a
// hook (Torre just delivered it a message): like a turn a hook started, it
// lasts until a hook ends it (InTurn). For agents without a start hook.
func (m *Monitor) ExpectTurn(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.get(id).open = true
}

// InTurn reports whether the terminal's turn hasn't ended yet: it is
// working, or a turn that only a hook ends hasn't ended (an agent silent at
// a permission prompt stops "working" after a while, but its turn goes on).
func (m *Monitor) InTurn(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := m.get(id)
	return tr.working || tr.open
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

// watchPrompt follows the prompt on an agent's screen (called with m.mu
// held): a prompt on a still screen raises an ask, its answer (the prompt
// leaving the screen) clears it. It reports whether the attention changed.
func (m *Monitor) watchPrompt(tr *track, agent, snap string, now time.Time) bool {
	p, found := attention.Parse(snap)
	a := tr.attn
	switch {
	case a != nil && found:
		changed := a.Prompt == nil || !a.Prompt.Same(p)
		a.SetPrompt(agent, &p)
		tr.attnSeen, tr.attnMissed = true, 0
		if a.Text == "" || a.Source == "screen" {
			a.Text = p.Question
		}
		return changed
	case a != nil && tr.attnSeen:
		// Answered, here or in the terminal: gone for two samples.
		if tr.attnMissed++; tr.attnMissed >= 2 {
			m.clearAttention(tr, now)
			return true
		}
	case a != nil:
		// Raised by a hook, and no prompt read yet (an agent whose prompt
		// isn't readable, or one answered at once): working again, after
		// the screen went still, ends it.
		if tr.streak == 0 {
			tr.attnQuiet = true
		} else if tr.attnQuiet && tr.streak >= m.opts.ActiveStreak {
			m.clearAttention(tr, now)
			return true
		}
	case found && now.Sub(tr.lastChange) >= m.opts.PromptAfter:
		tr.attn = &attention.Attention{Kind: attention.KindOf(p), Text: p.Question, Source: "screen", Since: now}
		tr.attn.SetPrompt(agent, &p)
		tr.attnSeen, tr.attnMissed = true, 0
		return true
	}
	return false
}

// clearAttention ends an ask; a turn it held open goes on from now.
func (m *Monitor) clearAttention(tr *track, now time.Time) {
	tr.attn, tr.attnSeen, tr.attnMissed, tr.attnQuiet = nil, false, 0, false
	tr.lastMeaningful = now
}

// Raise marks a terminal as waiting for the user, from a hook (a permission
// request): kind, tool and detail come from the hook, the choices from the
// prompt once it shows on screen.
func (m *Monitor) Raise(id string, a attention.Attention) {
	agent := ""
	if m.agentOf != nil {
		agent, _ = m.agentOf(id)
	}
	m.mu.Lock()
	tr := m.get(id)
	if old := tr.attn; old != nil {
		if old.Source == "hook" && old.Tool == a.Tool && old.Detail == a.Detail {
			m.mu.Unlock()
			return // the same request again
		}
		if old.Source == "screen" { // read on screen first: keep what it read
			a.Since = old.Since
			a.SetPrompt(agent, old.Prompt)
		}
	} else {
		tr.attnSeen = false
	}
	if a.Since.IsZero() {
		a.Since = time.Now()
	}
	a.Source = "hook"
	tr.attn, tr.attnMissed, tr.attnQuiet = &a, 0, false
	m.mu.Unlock()
	m.Refresh()
	st, since := m.State(id)
	m.onState(id, st, since) // the details changed even when the state didn't
}

// Denied records that the user denied, from the interface, the permission a
// terminal asked for: its turn ends when its screen goes quiet.
func (m *Monitor) Denied(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.get(id).denied = true
}

// Attention returns what a terminal waits for from the user (nil: nothing).
func (m *Monitor) Attention(id string) *attention.Attention {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a := m.get(id).attn; a != nil {
		c := *a
		return &c
	}
	return nil
}

// Asking reports whether a terminal waits for the user.
func (m *Monitor) Asking(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.get(id).attn != nil
}

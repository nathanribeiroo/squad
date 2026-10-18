package usage

import (
	"reflect"
	"sync"
	"time"
)

// Terminal is what the tracker needs to know about a terminal.
type Terminal struct {
	Agent        string // claude | codex | devin | gemini | …
	Conversation string // the agent's id for its conversation
}

// Options connect a Tracker to a workspace.
type Options struct {
	Accounts *Accounts
	// Terminal describes a terminal (false: gone or not an agent).
	Terminal func(id string) (Terminal, bool)
	// Publish sends an event to the interface.
	Publish func(typ string, data any)
	// Where each agent keeps its files ("" finds the usual place); tests
	// point them elsewhere.
	ClaudeHome, CodexHome, GeminiHome, DevinDB string
	Now                                        func() time.Time
}

// Tracker keeps what each flight of a workspace used.
type Tracker struct {
	o       Options
	mu      sync.Mutex
	flights map[string]Flight
	claude  map[string]*claudeTranscript // by terminal
}

// NewTracker makes a workspace's tracker.
func NewTracker(o Options) *Tracker {
	if o.Accounts == nil {
		o.Accounts = OpenAccounts("")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Publish == nil {
		o.Publish = func(string, any) {}
	}
	return &Tracker{o: o, flights: map[string]Flight{}, claude: map[string]*claudeTranscript{}}
}

// Flights returns what each flight used, as last read.
func (t *Tracker) Flights() []Flight {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Flight, 0, len(t.flights))
	for _, f := range t.flights {
		out = append(out, f)
	}
	return out
}

// Account returns an agent account's plan limits, as last seen.
func (t *Tracker) Account(agent string) (Account, bool) { return t.o.Accounts.Get(agent) }

// Forget drops a removed terminal.
func (t *Tracker) Forget(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.flights, id)
	delete(t.claude, id)
}

// StatusLine takes a Claude Code status line payload from terminal id: the
// context window and estimated cost, the plan's rate limits, and the
// transcript to read the tokens from.
func (t *Tracker) StatusLine(id string, body []byte) {
	s, err := ParseStatusLine(body)
	if err != nil {
		return
	}
	now := t.o.Now()
	if a, ok := s.Account(now); ok {
		t.o.Accounts.Set(a)
	}
	t.mu.Lock()
	tr := t.claude[id]
	if s.TranscriptPath != "" && (tr == nil || tr.path != s.TranscriptPath) {
		tr = &claudeTranscript{path: s.TranscriptPath}
		t.claude[id] = tr
	}
	prev := t.flights[id]
	t.mu.Unlock()
	fl := Flight{ID: id, Agent: "claude", Context: s.Context(), At: now}
	if fl.Context == nil {
		fl.Context = prev.Context
	}
	if tr != nil {
		tr.mu.Lock()
		_ = tr.read()
		fl.Tokens, fl.Models, fl.CostUSD = tr.totals()
		tr.mu.Unlock()
	}
	fl.CostUSD = max(fl.CostUSD, s.Cost.TotalCostUSD)
	if len(fl.Models) == 0 && s.Model.ID != "" {
		fl.Models = []string{s.Model.ID}
	}
	t.set(fl)
}

// Refresh reads a terminal's agent files again (at the end of each turn,
// and when the workspace opens).
func (t *Tracker) Refresh(id string) {
	term, ok := t.o.Terminal(id)
	if !ok || term.Conversation == "" {
		return
	}
	now := t.o.Now()
	t.mu.Lock()
	prev, had := t.flights[id]
	t.mu.Unlock()
	var fl Flight
	var acc *Account
	var err error
	switch term.Agent {
	case "claude":
		fl, err = t.claudeFlight(id, term.Conversation)
		if had {
			fl.Context = prev.Context
			fl.CostUSD = max(fl.CostUSD, prev.CostUSD)
		}
	case "codex":
		p := findCodexRollout(home(t.o.CodexHome, CodexHome), term.Conversation)
		if p == "" {
			return
		}
		fl, acc, err = readCodex(p, now)
	case "gemini":
		p := findGeminiSession(home(t.o.GeminiHome, GeminiHome), term.Conversation)
		if p == "" {
			return
		}
		fl, err = readGemini(p)
	case "devin":
		fl, err = readDevin(home(t.o.DevinDB, DevinDB), term.Conversation)
	default:
		return
	}
	if acc != nil {
		t.o.Accounts.Set(*acc)
	}
	if err != nil {
		return
	}
	fl.ID, fl.Agent = id, term.Agent
	if fl.At.IsZero() {
		fl.At = now
	}
	t.set(fl)
}

func (t *Tracker) claudeFlight(id, conv string) (Flight, error) {
	t.mu.Lock()
	tr := t.claude[id]
	if tr == nil {
		p := findClaudeTranscript(home(t.o.ClaudeHome, ClaudeHome), conv)
		if p == "" {
			t.mu.Unlock()
			return Flight{}, errNoData
		}
		tr = &claudeTranscript{path: p}
		t.claude[id] = tr
	}
	t.mu.Unlock()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if err := tr.read(); err != nil {
		return Flight{}, err
	}
	tok, models, cost := tr.totals()
	if tok.Total() == 0 {
		return Flight{}, errNoData
	}
	return Flight{Tokens: tok, Models: models, CostUSD: cost}, nil
}

// set keeps a flight's usage and tells the interface when it changed.
func (t *Tracker) set(fl Flight) {
	t.mu.Lock()
	prev, had := t.flights[fl.ID]
	t.flights[fl.ID] = fl
	t.mu.Unlock()
	if had && sameUsage(prev, fl) {
		return
	}
	t.o.Publish("usage.flight", fl)
}

func sameUsage(a, b Flight) bool {
	a.At, b.At = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}

func home(set string, def func() string) string {
	if set != "" {
		return set
	}
	return def()
}

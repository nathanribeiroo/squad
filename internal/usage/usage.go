// Package usage keeps what each flight's agent conversation used (tokens,
// the cache, Claude's estimated cost, Devin's ACUs, how full the context
// is) and each agent account's plan limits (Claude's 5-hour and weekly
// windows, Codex's, Devin's daily and weekly quotas or its ACUs).
//
// Everything is read from what the agents already write on this computer
// (Claude Code's status line and transcripts, Devin's session database,
// Codex's and Gemini's session files): nothing goes to the network and no
// credential is read.
package usage

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/config"
)

// Tokens a conversation used.
type Tokens struct {
	Input      int64 `json:"input"` // without the cache
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// Add returns t plus o.
func (t Tokens) Add(o Tokens) Tokens {
	return Tokens{t.Input + o.Input, t.Output + o.Output, t.CacheRead + o.CacheRead, t.CacheWrite + o.CacheWrite}
}

// Total is every token counted.
func (t Tokens) Total() int64 { return t.Input + t.Output + t.CacheRead + t.CacheWrite }

// Context is how full an agent's context window is.
type Context struct {
	Used    int64   `json:"used"`
	Size    int64   `json:"size"`
	Percent float64 `json:"percent"`
}

// Flight is what one terminal's conversation used so far (across resumes).
type Flight struct {
	ID     string `json:"id"` // the terminal
	Agent  string `json:"agent"`
	Tokens Tokens `json:"tokens"`
	// CostUSD is Claude Code's estimate at API prices (a subscription isn't
	// charged it); ACUs and credits are what Devin charged.
	CostUSD float64   `json:"costUsd,omitempty"`
	ACUs    float64   `json:"acus,omitempty"`
	Credits float64   `json:"credits,omitempty"`
	Context *Context  `json:"context,omitempty"`
	Models  []string  `json:"models,omitempty"`
	At      time.Time `json:"at"`
}

// Window kinds.
const (
	FiveHour = "five_hour" // Claude, Codex
	SevenDay = "seven_day" // Claude, Codex
	Daily    = "daily"     // Devin
	Weekly   = "weekly"    // Devin
)

// Window is a plan limit: the share used (0–100) and when it renews.
type Window struct {
	Kind     string    `json:"kind"`
	Used     float64   `json:"used"`
	ResetsAt time.Time `json:"resetsAt,omitzero"`
}

// ACUs are an account's Agent Compute Units (Devin), with its limit when
// the plan has one.
type ACUs struct {
	Used  float64 `json:"used"`
	Limit float64 `json:"limit,omitempty"`
}

// Account is an agent account's plan limits, as last seen. Windows is
// empty when the plan reports none (an API key, a company plan).
type Account struct {
	Agent   string    `json:"agent"`
	Windows []Window  `json:"windows,omitempty"`
	ACUs    *ACUs     `json:"acus,omitempty"`
	Seen    time.Time `json:"seen"`
}

// Accounts keeps the agents' accounts for every workspace (an account is
// the same in all of them) and saves them, so a restart shows the last
// ones seen.
type Accounts struct {
	mu   sync.Mutex
	path string
	m    map[string]Account
	subs []func(Account)
}

// OpenAccounts loads the accounts saved in path ("" keeps them in memory).
func OpenAccounts(path string) *Accounts {
	a := &Accounts{path: path, m: map[string]Account{}}
	if path == "" {
		return a
	}
	if b, err := os.ReadFile(path); err == nil {
		var list []Account
		if json.Unmarshal(b, &list) == nil {
			for _, x := range list {
				if x.Agent != "" {
					a.m[x.Agent] = x
				}
			}
		}
	}
	return a
}

// DefaultAccounts opens ~/.torre/usage-accounts.json.
func DefaultAccounts() *Accounts {
	dir, err := config.Dir()
	if err != nil {
		return OpenAccounts("")
	}
	return OpenAccounts(dir + string(os.PathSeparator) + "usage-accounts.json")
}

// Set records an account and tells the subscribers.
func (a *Accounts) Set(x Account) {
	if x.Agent == "" {
		return
	}
	a.mu.Lock()
	a.m[x.Agent] = x
	subs := slices.Clone(a.subs)
	a.saveLocked()
	a.mu.Unlock()
	for _, fn := range subs {
		fn(x)
	}
}

// Get returns an agent's account.
func (a *Accounts) Get(agent string) (Account, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	x, ok := a.m[agent]
	return x, ok
}

// All returns the accounts, by agent.
func (a *Accounts) All() []Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Account, 0, len(a.m))
	for _, x := range a.m {
		out = append(out, x)
	}
	slices.SortFunc(out, func(x, y Account) int {
		if x.Agent < y.Agent {
			return -1
		}
		if x.Agent > y.Agent {
			return 1
		}
		return 0
	})
	return out
}

// Subscribe calls fn with each account set; the returned func stops it.
func (a *Accounts) Subscribe(fn func(Account)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.subs = append(a.subs, fn)
	n := len(a.subs) - 1
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if n < len(a.subs) {
			a.subs[n] = func(Account) {}
		}
	}
}

func (a *Accounts) saveLocked() {
	if a.path == "" {
		return
	}
	list := make([]Account, 0, len(a.m))
	for _, x := range a.m {
		list = append(list, x)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err == nil {
		_ = config.WriteFileAtomic(a.path, b, 0o600)
	}
}

// errNoData: the agent hasn't written anything this package can read yet.
var errNoData = errors.New("no usage data yet")

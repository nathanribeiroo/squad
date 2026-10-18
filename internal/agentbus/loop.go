package agentbus

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
)

// Pause is why conversations are on hold: the user held them (Espera
// geral), or a chain of requests went past the hop limit (a loop).
type Pause struct {
	By string `json:"by"` // "user" or "loop"
	// Chain is the loop: the first asker first, each flight that asked the
	// next while answering a request, and last the one the refused request
	// was for.
	Chain []LoopStop `json:"chain,omitempty"`
	Hops  int        `json:"hops,omitempty"`
	Limit int        `json:"limit,omitempty"`
	At    time.Time  `json:"at"`
}

// LoopStop is a flight of a loop's chain.
type LoopStop struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Callsign string `json:"callsign,omitempty"`
}

// Label is how a chain names it: its callsign, or its name.
func (s LoopStop) Label() string {
	if s.Callsign != "" {
		return s.Callsign
	}
	return s.Name
}

// chainLocked is the path of requests that led caller to ask target: going
// back through the request each flight is answering (the one of the highest
// hop below the last), to the flight that started it.
func (b *Bus) chainLocked(caller, target string) []LoopStop {
	ids := []string{caller}
	cur, hop := caller, math.MaxInt
	for {
		var in *Pending
		for _, p := range b.pending {
			if p.To == cur && p.delivered && !p.resolved && p.Hop < hop && (in == nil || p.Hop > in.Hop) {
				in = p
			}
		}
		if in == nil {
			break
		}
		ids = append([]string{in.From}, ids...)
		cur, hop = in.From, in.Hop
	}
	ids = append(ids, target)
	chain := make([]LoopStop, 0, len(ids))
	for _, id := range ids {
		s := LoopStop{ID: id, Name: id}
		if n, ok := b.store.Node(id); ok && n.Terminal != nil {
			s.Name, s.Callsign = n.Terminal.Name, n.Terminal.Callsign
		}
		chain = append(chain, s)
	}
	return chain
}

// chainText writes a chain: "PLN → IMP → REV"; me, when set, is "you".
func chainText(chain []LoopStop, me string) string {
	labels := make([]string, len(chain))
	for i, s := range chain {
		labels[i] = s.Label()
		if s.ID == me {
			labels[i] = "you"
		}
	}
	return strings.Join(labels, " → ")
}

// holdLoop puts conversations on hold for a loop: caller went past the hop
// limit asking target. It returns what the caller gets.
func (b *Bus) holdLoop(caller, target string, hop, limit int) Result {
	b.mu.Lock()
	chain := b.chainLocked(caller, target)
	b.mu.Unlock()
	b.setPause(true, Pause{By: "loop", Chain: chain, Hops: hop, Limit: limit},
		fmt.Sprintf("hop limit %d reached %s", limit, chainText(chain, "")))
	return Result{Code: 409, Body: fmt.Sprintf("torre: hop limit (%d) reached: %s. Conversations are paused until the user releases them; tell the user.", limit, chainText(chain, caller))}
}

// setPause holds conversations (why says what held them) or releases them.
// A loop while they are already held replaces the reason.
func (b *Bus) setPause(on bool, why Pause, reason string) {
	b.mu.Lock()
	if b.paused == on && !(on && why.By == "loop") {
		b.mu.Unlock()
		return
	}
	b.paused = on
	if on {
		why.At = time.Now()
		b.pause = &why
	} else {
		b.pause = nil
		close(b.resume)
		b.resume = make(chan struct{})
	}
	b.mu.Unlock()
	data := map[string]any{"paused": on, "reason": reason}
	if on {
		data["pause"] = why
	}
	b.events.Publish(events.ConversationsPaused, data)
}

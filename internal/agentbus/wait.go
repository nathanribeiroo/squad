package agentbus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// waiter is a caller blocked in torre wait on another terminal's turn.
type waiter struct {
	From, To string
	ToName   string
	done     chan Result // buffered: receives one result (finishWait)
}

// finishWait ends a wait once; it reports whether this call ended it.
func (b *Bus) finishWait(w *waiter, r Result) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.waiters[w] {
		return false
	}
	delete(b.waiters, w)
	w.done <- r
	return true
}

// Wait waits for the current turn of a connected terminal to end and returns
// its final answer, without sending anything: for when an ask came back
// before the agent finished (a timeout) and the caller still needs the
// result. A terminal not in a turn answers at once with its last answer, or
// its screen. Like an ask, the wait goes on while the terminal shows
// activity, and gives up after a while without any (it may be waiting for
// the user). A message the awaited agent sends the caller meanwhile ends the
// wait (handToWaiter).
func (b *Bus) Wait(ctx context.Context, caller, target string) Result {
	if n, ok := b.store.Node(caller); !ok || n.Terminal == nil {
		return Result{Code: 400, Body: "torre: unknown calling terminal"}
	}
	tn, code, emsg := b.connectedTerminal(caller, target)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	t, code, emsg := b.runningTerminal(tn)
	if code != 0 {
		return Result{Code: code, Body: emsg}
	}
	name := tn.Terminal.Name
	w := &waiter{From: caller, To: tn.ID, ToName: name, done: make(chan Result, 1)}
	b.mu.Lock()
	if q := b.handOverLocked(caller, tn.ID); q != nil {
		b.mu.Unlock()
		return b.handedOver(q, fmt.Sprintf("Then run torre wait %s again.", quote(name)))
	}
	if b.reachableLocked(tn.ID, caller) {
		how := "through other agents"
		for _, p := range b.pending {
			if p.From == tn.ID && p.To == caller && !p.resolved {
				how = fmt.Sprintf("its request %s: answer with torre reply %s \"…\", or finish your turn", p.ID, p.ID)
			}
		}
		for o := range b.waiters {
			if o.From == tn.ID && o.To == caller {
				how = "it is in torre wait for you: finish your turn"
			}
		}
		b.mu.Unlock()
		return Result{Code: 409, Body: fmt.Sprintf("torre: %s is waiting for you (%s), so its turn can't end before yours", quote(name), how)}
	}
	b.waiters[w] = true
	b.mu.Unlock()
	if b.mon != nil {
		b.mon.Refresh()
		defer b.mon.Refresh()
	}
	defer b.finishWait(w, Result{Code: 499, Source: "cancelled"})

	if b.mon == nil {
		b.finishWait(w, b.lastAnswer(tn, t))
		return <-w.done
	}
	defer b.onFinalTurnEnd(tn.ID, b.hasHooks(tn.Terminal.Agent), func(te activity.TurnEnd) {
		if te.Message != "" {
			b.finishWait(w, Result{Code: 200, Body: fmt.Sprintf("[final answer from %s · end of its turn]\n%s", name, te.Message), Source: te.Reason})
			return
		}
		b.finishWait(w, Result{Code: 200, Body: fmt.Sprintf("[screen of %s after its turn]\n%s", name, t.Snapshot()), Source: te.Reason})
	})()
	// Checked after registering, so a turn that ends meanwhile isn't missed.
	if !b.mon.InTurn(tn.ID) {
		b.finishWait(w, b.lastAnswer(tn, t))
	}

	timeout := time.NewTimer(b.limits.Timeout)
	defer timeout.Stop()
	hardCap := time.Now().Add(b.limits.MaxWait)
	for {
		select {
		case r := <-w.done:
			return r
		case <-ctx.Done():
			b.finishWait(w, Result{Code: 499, Body: "torre: caller went away", Source: "cancelled"})
			return <-w.done
		case <-timeout.C:
			now := time.Now()
			if now.Before(hardCap) && b.active(tn.ID, t) {
				timeout.Reset(min(b.limits.Timeout, hardCap.Sub(now)))
				continue
			}
			b.finishWait(w, Result{Code: 200, Source: "timeout", Body: fmt.Sprintf(
				"[screen of %s · its turn hasn't ended]\n%s\n[torre: no activity from %s for a while. If its screen shows a question or a permission prompt, it is waiting for the user: tell the user. Otherwise run torre wait %s again.]",
				name, t.Snapshot(), quote(name), quote(name))})
		}
	}
}

// lastAnswer is what torre wait returns for a terminal that isn't in a
// turn: its last final answer, or its screen.
func (b *Bus) lastAnswer(tn store.Node, t *terminal.Terminal) Result {
	b.mu.Lock()
	te, ok := b.last[tn.ID]
	b.mu.Unlock()
	if ok && te.Message != "" {
		head := fmt.Sprintf("[last answer from %s · its turn ended %s ago", tn.Terminal.Name, ago(te.At))
		if prompt, _, _ := strings.Cut(strings.TrimSpace(te.Prompt), "\n"); prompt != "" {
			head += " · it began with: " + preview(prompt)
		}
		return Result{Code: 200, Body: head + "]\n" + te.Message, Source: "last"}
	}
	return Result{Code: 200, Body: fmt.Sprintf("[screen of %s · it isn't in a turn]\n%s", tn.Terminal.Name, t.Snapshot()), Source: "screen"}
}

// turnEnded keeps each terminal's last final turn end for torre wait (a turn
// end guessed from the screen isn't final for agents with hooks).
func (b *Bus) turnEnded(te activity.TurnEnd) {
	n, ok := b.store.Node(te.ID)
	if !ok || n.Terminal == nil || (te.Reason == "idle" && b.hasHooks(n.Terminal.Agent)) {
		return
	}
	b.mu.Lock()
	b.last[te.ID] = te
	yours := b.yourTurnLocked(te, n.Terminal.Agent)
	fn := b.onYourTurn
	b.mu.Unlock()
	if yours && fn != nil {
		fn(te.ID)
	}
}

// SetOnYourTurn installs fn, called when a flight's turn ended and that turn
// was the user's (Avisos → "Quando um voo termina e espera você").
func (b *Bus) SetOnYourTurn(fn func(id string)) { b.onYourTurn = fn }

// yourTurnLocked reports whether the turn that te ended was the user's: an
// agent's turn that answered no request (the asker gets that answer), with
// no request waiting in its line (it goes on at once) and not ended by the
// user's own "deny".
func (b *Bus) yourTurnLocked(te activity.TurnEnd, agent string) bool {
	answered := b.answered[te.ID]
	delete(b.answered, te.ID)
	if answered || !IsAgent(agent) || te.Reason == "denied" {
		return false
	}
	for _, p := range b.pending {
		if p.To == te.ID && !p.resolved {
			return false
		}
	}
	return true
}

// handToWaiter gives request p to its target when the target is blocked in
// torre wait for the sender (or for someone who, through a chain, waits for
// the sender), instead of pasting it into a terminal that can't read it yet.
func (b *Bus) handToWaiter(p *Pending, msg string) bool {
	b.mu.Lock()
	var w *waiter
	for c := range b.waiters {
		if c.From == p.To && (c.To == p.From || b.reachableLocked(c.To, p.From)) {
			w = c
			break
		}
	}
	b.mu.Unlock()
	if w == nil {
		return false
	}
	while := "while you were waiting for its answer"
	if w.To != p.From {
		while = "while you were waiting for " + quote(w.ToName)
	}
	return b.finishWait(w, Result{Code: 200, Source: "message", Body: fmt.Sprintf(
		"%s\n[torre: %s sent this %s, and waits for yours: torre reply %s \"…\". Then run torre wait %s again to get %s's final answer.]",
		Envelope(p.FromName, p.ID, msg, false), quote(p.FromName), while, p.ID, quote(w.ToName), w.ToName)})
}

// ago says how long ago t was, roughly.
func ago(t time.Time) string {
	switch d := time.Since(t); {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
}

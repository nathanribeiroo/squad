package agentbus

import (
	"context"
	"fmt"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// queueable reports whether requests to an agent of this type wait in its
// line: an agent reads a message pasted during a turn only after that turn,
// whose end would then answer the wrong request. A shell (or an own command)
// has no turns, and one running a server never looks idle.
func queueable(agent string) bool { return IsAgent(agent) }

// takeTurn holds request p until its target can take it. A target takes the
// next request in its line (oldest first) when it isn't working nor asking
// the user, has been out of its turn for a moment (a message the user typed
// meanwhile starts at once), and has no other request delivered and
// unanswered. A target blocked in torre wait for the sender takes it at once
// (handToWaiter). handed is true when p was given to the target's own ask or
// wait meanwhile (handOverLocked); done is true when p ended while waiting,
// with its result.
func (b *Bus) takeTurn(ctx context.Context, p *Pending, tn store.Node) (handed bool, r Result, done bool) {
	inLine := queueable(tn.Terminal.Agent) && b.mon != nil
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	limit := time.NewTimer(time.Until(p.SentAt.Add(b.limits.MaxWait)))
	defer limit.Stop()
	for {
		free := !inLine || b.mon.Idle(tn.ID) >= b.limits.Settle
		b.mu.Lock()
		if p.delivered {
			b.mu.Unlock()
			return true, Result{}, false
		}
		if !p.resolved && (!inLine || free && b.nextLocked(p) || b.waitsForLocked(p.To, p.From)) {
			p.delivered, p.Queued = true, false
			b.mu.Unlock()
			return false, Result{}, false
		}
		queued := false
		ahead := 0
		if !p.resolved && !p.Queued {
			p.Queued, queued = true, true
			ahead = b.aheadLocked(p)
		}
		b.mu.Unlock()
		if queued {
			b.events.Publish(events.RequestQueued, map[string]any{"id": p.ID, "from": p.From, "to": p.To, "ahead": ahead})
		}
		select {
		case r := <-p.done:
			return false, r, true
		case <-ctx.Done():
			b.resolve(p, Result{Code: 499, Body: "torre: caller went away", Source: "cancelled"})
			return false, <-p.done, true
		case <-limit.C:
			why := fmt.Sprintf("%s was busy the whole time (in a turn, or taking other requests)", quote(p.ToName))
			if b.mon != nil && b.mon.Asking(tn.ID) {
				why = fmt.Sprintf("%s waited for the user at a prompt the whole time", quote(p.ToName))
			}
			b.resolve(p, Result{Code: 200, Body: "[torre: " + why + "; the message was not delivered]", Source: "timeout"})
			return false, <-p.done, true
		case <-tick.C:
		}
	}
}

// nextLocked reports whether p is first in its target's line: no request
// to the target is delivered and unanswered, and none waits longer.
func (b *Bus) nextLocked(p *Pending) bool {
	for _, o := range b.pending {
		if o == p || o.To != p.To || o.resolved {
			continue
		}
		if o.delivered || o.SentAt.Before(p.SentAt) || o.SentAt.Equal(p.SentAt) && o.ID < p.ID {
			return false
		}
	}
	return true
}

// aheadLocked counts the requests before p in its target's line, the one
// the target is answering included.
func (b *Bus) aheadLocked(p *Pending) int {
	n := 0
	for _, o := range b.pending {
		if o != p && o.To == p.To && !o.resolved && (o.delivered || o.SentAt.Before(p.SentAt)) {
			n++
		}
	}
	return n
}

// waitsForLocked reports whether target is blocked in torre wait for from,
// or for someone who, through a chain, waits for from (handToWaiter).
func (b *Bus) waitsForLocked(target, from string) bool {
	for w := range b.waiters {
		if w.From == target && (w.To == from || b.reachableLocked(w.To, from)) {
			return true
		}
	}
	return false
}

// handOverLocked finds the oldest request waiting in caller's line from
// target, or from someone target waits for through a chain, and marks it
// delivered: caller is busy (it is asking or waiting for target), so that
// sender would wait for caller's turn while caller waits for target's, and
// neither would end. Caller gets the request instead, to answer it first.
func (b *Bus) handOverLocked(caller, target string) *Pending {
	var q *Pending
	for _, o := range b.pending {
		if o.To != caller || o.delivered || o.resolved || o.From != target && !b.reachableLocked(target, o.From) {
			continue
		}
		if q == nil || o.SentAt.Before(q.SentAt) {
			q = o
		}
	}
	if q != nil {
		q.delivered, q.Queued = true, false
	}
	return q
}

// handedOver is what a busy caller gets instead of sending its message or
// waiting: the request waiting in its line, and what to do (then is how to
// go on afterwards).
func (b *Bus) handedOver(q *Pending, then string) Result {
	b.mu.Lock()
	msg, ok := b.texts[q.ID]
	b.mu.Unlock()
	if !ok {
		msg = q.Preview
	}
	return Result{Code: 200, Source: "message", Body: fmt.Sprintf(
		"%s\n[torre: %s sent this while you were busy, and waits for your answer: torre reply %s \"…\". %s]",
		Envelope(q.FromName, q.ID, msg, false), quote(q.FromName), q.ID, then)}
}

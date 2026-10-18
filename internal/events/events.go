// Package events is the server's publish/subscribe bus. Events go to every
// connected UI over /ws/events; slow subscribers are dropped instead of
// blocking publishers.
package events

import (
	"sync"
	"time"
)

// Event types sent to clients (names follow docs/design_handoff_squad).
const (
	TerminalState       = "terminal.state"  // lifecycle + activity state of a terminal
	TerminalTitle       = "terminal.title"  // window title (agent spinner)
	TerminalNotice      = "terminal.notice" // something the user should know about a terminal (kind: not-resumed)
	NodeAdded           = "node.added"      //
	NodeUpdated         = "node.updated"    //
	NodeRemoved         = "node.removed"    //
	EdgeAdded           = "edge.added"      //
	EdgeUpdated         = "edge.updated"    //
	EdgeRemoved         = "edge.removed"    //
	RequestSent         = "request.sent"    // ask left the sender
	RequestQueued       = "request.queued"  // waits for a busy flight's turn to end
	RequestDelivered    = "request.delivered"
	RequestPending      = "request.pending"
	ReplySent           = "reply.sent"
	ReplyReceived       = "reply.received"
	RequestCancelled    = "request.cancelled"
	RequestHeld         = "request.held" // paused or rate limited
	ConversationsPaused = "conversations.paused"
	LimitsChanged       = "limits.changed"
	NoteEdited          = "note.edited"
	MessageSent         = "message.sent" // the user sent a Composer message to a terminal
	ServerStatus        = "server.status"
	WorkspaceUpdated    = "workspace.updated" // name, folder or color changed
	FloorsUpdated       = "floors.updated"    // the floors and their config (floors.State)
	FloorChecked        = "floor.checked"     // an agent marked a floor's tests or review
	TurnCard            = "turn.card"         // an agent's last turn, or its summary (summary.Card)
	SummarySettings     = "summary.settings"  // turn cards and summary engine (summary.Settings)
	RadioItem           = "radio.item"        // a line of the Rádio, new or changed (radio.Item)
	RadioCleared        = "radio.cleared"     // the user cleared the Rádio
	UsageFlight         = "usage.flight"      // what a flight's conversation used (usage.Flight)
	UsageAccount        = "usage.account"     // an agent account's plan limits (usage.Account)
)

// Event is one notification.
type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data any       `json:"data,omitempty"`
}

// Bus fans events out to subscribers.
type Bus struct {
	mu     sync.Mutex
	subs   map[int]chan Event
	next   int
	closed bool
}

// NewBus returns an empty bus.
func NewBus() *Bus { return &Bus{subs: map[int]chan Event{}} }

// Publish sends an event to every subscriber without blocking; a subscriber
// whose buffer is full is dropped (its channel is closed).
func (b *Bus) Publish(typ string, data any) {
	ev := Event{Type: typ, Time: time.Now().UTC(), Data: data}
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			close(ch)
			delete(b.subs, id)
		}
	}
}

// Close ends every subscription (their channels close) and the ones to
// come: the workspace behind the bus is gone. Publishing does nothing then.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for id, ch := range b.subs {
		close(ch)
		delete(b.subs, id)
	}
}

// Closed reports whether the bus was closed.
func (b *Bus) Closed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// Subscribe returns a channel of events and a cancel function.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 512)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	id := b.next
	b.next++
	b.subs[id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subs[id]; ok {
			close(c)
			delete(b.subs, id)
		}
	}
}

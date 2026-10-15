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
	TerminalState       = "terminal.state" // lifecycle + activity state of a terminal
	TerminalTitle       = "terminal.title" // window title (agent spinner)
	NodeAdded           = "node.added"     //
	NodeUpdated         = "node.updated"   //
	NodeRemoved         = "node.removed"   //
	EdgeAdded           = "edge.added"     //
	EdgeUpdated         = "edge.updated"   //
	EdgeRemoved         = "edge.removed"   //
	RequestSent         = "request.sent"   // ask left the sender
	RequestDelivered    = "request.delivered"
	RequestPending      = "request.pending"
	ReplySent           = "reply.sent"
	ReplyReceived       = "reply.received"
	RequestCancelled    = "request.cancelled"
	RequestHeld         = "request.held" // paused or rate limited
	ConversationsPaused = "conversations.paused"
	NoteEdited          = "note.edited"
	ServerStatus        = "server.status"
)

// Event is one notification.
type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data any       `json:"data,omitempty"`
}

// Bus fans events out to subscribers.
type Bus struct {
	mu   sync.Mutex
	subs map[int]chan Event
	next int
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

// Subscribe returns a channel of events and a cancel function.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 512)
	b.mu.Lock()
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

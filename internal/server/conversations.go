package server

import (
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Conversations: a terminal remembers the id of its agent's conversation
// (store.TerminalData.Conversation) and continues it when it starts again
// (the launch hook builds the line), once it has had a turn (Empty until
// then: an agent can't continue a conversation where nothing was said).
// Busy marks an agent in the middle of a turn, from its hooks; one that
// starts again while busy was interrupted (Squad closed or restarted) and
// shows as such until it works again, the user continues it or dismisses
// the notice.

// startedConversation records the conversation a terminal is starting in.
func (s *Server) startedConversation(n store.Node, before store.TerminalData, resumed bool) {
	conv := n.Terminal.Conversation
	busy := before.Busy && resumed
	// A new id from Squad (Claude, Gemini) is a new, empty conversation.
	empty := !resumed && conv != "" && (conv != before.Conversation || before.Empty)
	if conv != before.Conversation || busy != before.Busy || empty != before.Empty {
		if m, err := s.store.UpdateNode(n.ID, func(m *store.Node) error {
			m.Terminal.Conversation, m.Terminal.Busy, m.Terminal.Empty = conv, busy, empty
			return nil
		}); err == nil && (conv != before.Conversation || empty != before.Empty) {
			s.bus.Publish(events.NodeUpdated, m)
		}
	}
	s.mu.Lock()
	s.interrupted[n.ID] = busy
	if resumed {
		s.resuming[n.ID] = conv
	} else {
		delete(s.resuming, n.ID)
	}
	s.mu.Unlock()
}

// SetConversation records the conversation id an agent reported in a hook
// (the caller validates it). source is how its session began, on a session
// start: if the terminal asked to continue a conversation and the agent
// started another one, the user is told.
func (s *Server) SetConversation(id, conv, source string) {
	n, ok := s.store.Node(id)
	if !ok || n.Terminal == nil {
		return
	}
	s.mu.Lock()
	_, asked := s.resuming[id]
	if source != "" {
		delete(s.resuming, id)
	}
	s.mu.Unlock()
	if asked && source != "" && source != "resume" {
		s.bus.Publish(events.TerminalNotice, map[string]string{"id": id, "name": n.Terminal.Name, "kind": "not-resumed"})
	}
	empty := n.Terminal.Empty
	switch source {
	case "startup", "clear": // a new conversation: nothing said yet
		empty = true
	case "resume":
		empty = false
	}
	if conv == n.Terminal.Conversation && empty == n.Terminal.Empty {
		return
	}
	if m, err := s.store.UpdateNode(id, func(m *store.Node) error {
		m.Terminal.Conversation, m.Terminal.Empty = conv, empty
		return nil
	}); err == nil {
		s.bus.Publish(events.NodeUpdated, m)
	}
}

// SetBusy records whether an agent is in the middle of a turn (from its turn
// start and end hooks). Working again ends an interruption; a turn makes the
// conversation one to continue.
func (s *Server) SetBusy(id string, busy bool) {
	n, ok := s.store.Node(id)
	if !ok || n.Terminal == nil {
		return
	}
	if n.Terminal.Busy != busy || (busy && n.Terminal.Empty) {
		if m, err := s.store.UpdateNode(id, func(m *store.Node) error {
			m.Terminal.Busy = busy
			if busy {
				m.Terminal.Empty = false
			}
			return nil
		}); err == nil && n.Terminal.Empty && busy {
			s.bus.Publish(events.NodeUpdated, m) // the resume command applies now
		}
	}
	if busy {
		s.clearInterrupted(id)
	}
}

// Interrupted returns the terminals whose agent was interrupted mid-turn.
func (s *Server) Interrupted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for id, on := range s.interrupted {
		if on {
			out = append(out, id)
		}
	}
	return out
}

// DismissInterrupted forgets that a terminal's agent was interrupted.
func (s *Server) DismissInterrupted(id string) {
	if n, ok := s.store.Node(id); ok && n.Terminal != nil && n.Terminal.Busy {
		_, _ = s.store.UpdateNode(id, func(m *store.Node) error {
			m.Terminal.Busy = false
			return nil
		})
	}
	s.clearInterrupted(id)
}

func (s *Server) clearInterrupted(id string) {
	s.mu.Lock()
	was := s.interrupted[id]
	delete(s.interrupted, id)
	s.mu.Unlock()
	if was {
		s.bus.Publish(events.TerminalState, s.runtimeOf(id))
	}
}

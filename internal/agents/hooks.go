package agents

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/nathanribeiroo/squad/internal/activity"
)

// HookEvent is what an agent hook means for Squad.
type HookEvent struct {
	TurnStart bool   // the agent started working on a prompt
	TurnEnd   bool   // the agent finished its turn
	Message   string // final assistant message (when TurnEnd)
	Failure   bool   // the turn ended with an error
}

// ParseHook interprets the payload of a named hook ("claude-stop",
// "claude-stop-failure", "claude-prompt", "codex-notify").
func ParseHook(name string, body []byte) (HookEvent, bool) {
	switch name {
	case "claude-prompt":
		return HookEvent{TurnStart: true}, true
	case "claude-stop", "claude-stop-failure":
		var p struct {
			LastAssistantMessage string `json:"last_assistant_message"`
			TranscriptPath       string `json:"transcript_path"`
			StopHookActive       bool   `json:"stop_hook_active"`
			Error                string `json:"error"`
			ErrorDetails         string `json:"error_details"`
		}
		if json.Unmarshal(body, &p) != nil {
			return HookEvent{}, false
		}
		ev := HookEvent{TurnEnd: true, Message: p.LastAssistantMessage, Failure: name == "claude-stop-failure"}
		if ev.Message == "" && p.TranscriptPath != "" {
			// Older Claude Code versions: read the transcript instead.
			if msg, err := LastAssistantFromTranscript(p.TranscriptPath); err == nil {
				ev.Message = msg
			}
		}
		if ev.Failure && ev.Message == "" {
			ev.Message = strings.TrimSpace("error: " + p.Error + " " + p.ErrorDetails)
		}
		return ev, true
	case "codex-notify":
		var p struct {
			Type                 string `json:"type"`
			LastAssistantMessage string `json:"last-assistant-message"`
		}
		if json.Unmarshal(body, &p) != nil || p.Type != "agent-turn-complete" {
			return HookEvent{}, false
		}
		return HookEvent{TurnEnd: true, Message: p.LastAssistantMessage}, true
	}
	return HookEvent{}, false
}

// HookHandler serves POST /hooks/{agent}: it turns hook payloads into turn
// signals for the activity monitor. callerOf returns the authenticated
// terminal id.
func HookHandler(mon *activity.Monitor, callerOf func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		ev, ok := ParseHook(r.PathValue("agent"), body)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		id := callerOf(r)
		switch {
		case ev.TurnStart:
			mon.SignalStart(id)
		case ev.TurnEnd:
			reason := r.PathValue("agent")
			mon.Signal(activity.TurnEnd{ID: id, Reason: reason, Message: ev.Message})
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

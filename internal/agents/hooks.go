package agents

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/attention"
)

// HookEvent is what an agent hook means for Torre.
type HookEvent struct {
	TurnStart    bool   // the agent started working on a prompt
	Prompt       string // that prompt (when TurnStart)
	TurnEnd      bool   // the agent finished its turn
	Message      string // final assistant message (when TurnEnd)
	Failure      bool   // the turn ended with an error
	SessionStart bool   // a session began: the hook's answer may add context
	// Conversation is the agent's id for the conversation, when the payload
	// has it (session_id; Codex's thread-id). Source is how a session began
	// (SessionStart): startup, resume, clear, compact.
	Conversation string
	Source       string
	// Attention: the agent asks the user's permission for a tool.
	Attention *attention.Attention
}

// ParseHook interprets the payload of a named hook ("claude-stop",
// "claude-stop-failure", "claude-prompt", "claude-session-start",
// "claude-permission", "codex-notify", "devin-stop", "devin-prompt",
// "devin-session-start", "devin-permission").
func ParseHook(name string, body []byte) (HookEvent, bool) {
	var common struct {
		SessionID string `json:"session_id"`
		Source    string `json:"source"`
	}
	_ = json.Unmarshal(body, &common)
	switch name {
	case "claude-session-start", "devin-session-start":
		return HookEvent{SessionStart: true, Conversation: common.SessionID, Source: common.Source}, true
	case "claude-prompt", "devin-prompt":
		var p struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(body, &p) // the start counts even without a readable prompt
		return HookEvent{TurnStart: true, Prompt: p.Prompt, Conversation: common.SessionID}, true
	case "claude-stop", "claude-stop-failure", "devin-stop":
		// Devin's Stop payload follows Claude Code's: last_assistant_message
		// (Devin CLI v3000.5.20+), no transcript_path and no failure event.
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
		ev := HookEvent{TurnEnd: true, Message: p.LastAssistantMessage, Failure: name == "claude-stop-failure", Conversation: common.SessionID}
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
	case "claude-permission", "devin-permission":
		var p struct {
			ToolName  string         `json:"tool_name"`
			ToolInput map[string]any `json:"tool_input"`
		}
		if json.Unmarshal(body, &p) != nil || p.ToolName == "" {
			return HookEvent{}, false
		}
		kind := attention.Permission
		if p.ToolName == "AskUserQuestion" || p.ToolName == "ask_user_question" {
			kind = attention.Question
		}
		// Torre's own tools that need the user's approval (keys, new_shell…)
		// keep asking: the interface never answers them "always".
		tool, torre := strings.CutPrefix(p.ToolName, "mcp__torre__")
		return HookEvent{Conversation: common.SessionID,
			Attention: &attention.Attention{Kind: kind, Tool: p.ToolName, Detail: toolDetail(p.ToolInput),
				AskEveryUse: torre && !slices.Contains(SafeMCPTools, tool)}}, true
	case "codex-notify":
		var p struct {
			Type                 string `json:"type"`
			ThreadID             string `json:"thread-id"`
			LastAssistantMessage string `json:"last-assistant-message"`
		}
		if json.Unmarshal(body, &p) != nil || p.Type != "agent-turn-complete" {
			return HookEvent{}, false
		}
		return HookEvent{TurnEnd: true, Message: p.LastAssistantMessage, Conversation: p.ThreadID}, true
	}
	return HookEvent{}, false
}

// HookOptions connect HookHandler to the workspace.
type HookOptions struct {
	Monitor  *activity.Monitor
	CallerOf func(*http.Request) string // the authenticated terminal id
	// Role returns the instructions of a terminal's role (false: none),
	// answered to Devin's session start (Claude Code gets them at launch).
	Role func(id string) (string, bool)
	// Conversation gets the conversation id a hook reported (source: how
	// the session began, on a session start; "" otherwise).
	Conversation func(id, conversation, source string)
	// Busy gets the start (true) and end (false) of each turn.
	Busy func(id string, busy bool)
}

// HookHandler serves POST /hooks/{agent}: it turns hook payloads into turn
// signals for the activity monitor, keeps the conversation id, and answers
// Devin's session start with the terminal's role as context.
func HookHandler(o HookOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("agent")
		body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		ev, ok := ParseHook(name, body)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		id := o.CallerOf(r)
		if ev.Conversation != "" && o.Conversation != nil {
			o.Conversation(id, ev.Conversation, ev.Source)
		}
		switch {
		case ev.SessionStart:
			// A continued conversation already has its role.
			if name != "devin-session-start" || ev.Source == "resume" || o.Role == nil {
				break
			}
			text, ok := o.Role(id)
			if !ok {
				break
			}
			// Claude Code's hook output format, which Devin reads too.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"hookSpecificOutput": map[string]string{
				"hookEventName": "SessionStart", "additionalContext": text,
			}})
			return
		case ev.Attention != nil:
			// Shown to the user; the hook's answer stays empty, so the agent
			// asks as it always does (Torre never decides a permission).
			o.Monitor.Raise(id, *ev.Attention)
		case ev.TurnStart:
			o.Monitor.SignalStart(id, ev.Prompt)
			if o.Busy != nil {
				o.Busy(id, true)
			}
		case ev.TurnEnd:
			o.Monitor.Signal(activity.TurnEnd{ID: id, Reason: name, Message: ev.Message})
			if o.Busy != nil {
				o.Busy(id, false)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// toolDetail is what a permission request puts at stake: the command, the
// file, the address; else the input in short.
func toolDetail(in map[string]any) string {
	for _, k := range []string{"command", "file_path", "notebook_path", "path", "url", "pattern", "query"} {
		if v, ok := in[k].(string); ok && strings.TrimSpace(v) != "" {
			return clip(strings.TrimSpace(v))
		}
	}
	if len(in) == 0 {
		return ""
	}
	b, _ := json.Marshal(in)
	return clip(string(b))
}

// clip shortens s to 300 characters.
func clip(s string) string {
	if utf8.RuneCountInString(s) <= 300 {
		return s
	}
	r := []rune(s)
	return string(r[:299]) + "…"
}

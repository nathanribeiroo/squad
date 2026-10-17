package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/attention"
)

// handleRespond answers the prompt on a flight's screen with the user's
// choice (the RESPONDER bar). It is an interface route: agents reach /cli
// and MCP, never this. It types only into agents (never a shell or an own
// command), only while the prompt the interface showed is still on the
// screen, and only the keys verified for that agent.
func (s *Server) handleRespond(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Choice attention.Choice `json:"choice"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	switch req.Choice {
	case attention.Allow, attention.Always, attention.Deny:
	default:
		writeError(w, http.StatusBadRequest, `choice is "allow", "always" or "deny"`)
		return
	}
	n, ok := s.store.Node(id)
	if !ok || n.Terminal == nil {
		writeError(w, http.StatusNotFound, "terminal not found")
		return
	}
	switch n.Terminal.Agent {
	case "claude", "codex", "devin", "gemini":
	default:
		writeErr(w, http.StatusBadRequest, apierr.With(errors.New("only agents' prompts are answered from here"), "attention.not_agent"))
		return
	}
	t := s.terms.Get(id)
	var a *attention.Attention
	if s.attentionOf != nil {
		a = s.attentionOf(id)
	}
	if t == nil || a == nil || a.Prompt == nil {
		writeErr(w, http.StatusConflict, apierr.With(errors.New("the flight isn't asking anything now"), "attention.none"))
		return
	}
	now, ok := attention.Parse(t.Snapshot())
	if !ok || !now.Same(*a.Prompt) {
		writeErr(w, http.StatusConflict, apierr.With(errors.New("the prompt on screen changed: open the terminal"), "attention.changed"))
		return
	}
	keys, ok := attention.Keys(n.Terminal.Agent, now, req.Choice)
	if !ok {
		writeErr(w, http.StatusBadRequest, apierr.With(errors.New("this answer can't be sent to this agent: open the terminal"), "attention.no_keys"))
		return
	}
	if err := t.Input([]byte(keys)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

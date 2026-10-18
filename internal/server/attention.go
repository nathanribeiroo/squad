package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/attention"
)

// handleRespond answers the prompt on a flight's screen with the user's
// choice (the RESPONDER bar): allow, always or deny on a permission, or one
// of the agent's own options on a question or a plan, with the user's text
// when the option is a text field. It is an interface route: agents reach
// /cli and MCP, never this. It types only into agents (never a shell or an
// own command), only while the prompt the interface showed is still on the
// screen, only the keys verified for that agent, and only an answer the
// interface offered.
func (s *Server) handleRespond(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Choice attention.Choice `json:"choice"`
		Option string           `json:"option"`
		Text   string           `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	text := oneLine(req.Text)
	switch {
	case req.Option != "":
		if utf8.RuneCountInString(text) > maxAnswer {
			writeErr(w, http.StatusBadRequest, apierr.With(fmt.Errorf("the answer has more than %d characters", maxAnswer), "attention.too_long"))
			return
		}
	case req.Choice == attention.Allow, req.Choice == attention.Always, req.Choice == attention.Deny:
	default:
		writeError(w, http.StatusBadRequest, `choice is "allow", "always" or "deny", or option is one of the prompt's`)
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
	noKeys := apierr.With(errors.New("this answer can't be sent to this agent: open the terminal"), "attention.no_keys")
	if req.Option != "" {
		// Only an option the interface offered, with text only on the text field.
		keys, ok := attention.OptionKeys(n.Terminal.Agent, now, req.Option, text)
		o := slices.IndexFunc(a.Options, func(o attention.Option) bool { return o.Key == req.Option })
		if !ok || o < 0 {
			writeErr(w, http.StatusBadRequest, noKeys)
			return
		}
		for i, k := range keys {
			if i > 0 {
				time.Sleep(keyGap) // the agent moves to the text field, then takes the text
			}
			if err := t.Input([]byte(k)); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		answer := a.Options[o].Text
		if a.Options[o].Input {
			answer = text
		}
		if s.onAnswer != nil {
			s.onAnswer(id, attention.Picked, answer)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Only what the interface offered: never "always" for a tool that asks
	// at every use.
	keys, ok := attention.Keys(n.Terminal.Agent, now, req.Choice)
	if !ok || !slices.Contains(a.Choices, req.Choice) {
		writeErr(w, http.StatusBadRequest, noKeys)
		return
	}
	if err := t.Input([]byte(keys)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.onAnswer != nil {
		s.onAnswer(id, req.Choice, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxAnswer is the longest answer the user may write on a prompt's text field.
const maxAnswer = 2000

// keyGap is the pause between the keys of an answer: picking the text field,
// the text, Enter.
const keyGap = 300 * time.Millisecond

// oneLine is the user's text for a prompt's text field: Enter would send it
// early, so line breaks become spaces.
func oneLine(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
}

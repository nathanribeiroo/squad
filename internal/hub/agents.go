package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/agents"
	"github.com/nathanribeiroo/squad/internal/apierr"
)

// handleAgents is GET /api/agents (Configurações → Agentes, Novo voo): the
// agents and the browser found on this computer. ?refresh=1 looks again.
func (h *Hub) handleAgents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, agents.Detect(r.URL.Query().Get("refresh") != ""))
}

// handleAgentCommand is PUT /api/agents/{agent} {"defaultCommand"}: the
// command that starts the agent in terminals without their own ("" resets).
// Only the interface reaches it: it decides what runs in every agent
// terminal started afterwards.
func (h *Hub) handleAgentCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DefaultCommand string `json:"defaultCommand"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	agent := r.PathValue("agent")
	if !agents.IsAgentType(agent) {
		writeError(w, http.StatusNotFound, "unknown agent "+agent)
		return
	}
	if err := agents.SetDefaultCommand(agent, body.DefaultCommand); err != nil {
		if errors.Is(err, agents.ErrBadCommand) {
			err = apierr.With(err, "agents.bad_command")
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, agents.Detect(true))
}

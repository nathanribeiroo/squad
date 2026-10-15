package server

import (
	"net/http"

	"github.com/nathanribeiroo/squad/internal/terminal"
)

// handleShells lists the shells a terminal can use (GET /api/shells).
func (s *Server) handleShells(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]terminal.ShellOption{"shells": terminal.AvailableShells()})
}

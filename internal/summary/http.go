package summary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/store"
)

// Register adds the summary API to a workspace's routes:
//
//	GET  /api/summary                   the cards ({"cards": [...]})
//	POST /api/nodes/{id}/summarize      summarize a terminal's last turn (202; turn.card follows)
//	POST /api/summary/workspace         an overview of the workspace (waits for it)
//	GET  /api/summary/settings          the config and the engines found
//	PUT  /api/summary/settings          {cards, engine}
func (s *Service) Register(handle func(pattern string, h http.Handler)) {
	handle("GET /api/summary", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string][]Card{"cards": s.Cards()})
	}))
	handle("POST /api/nodes/{id}/summarize", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Summarize(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, c)
	}))
	handle("POST /api/summary/workspace", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o, err := s.Overview(context.WithoutCancel(r.Context()))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, o)
	}))
	handle("GET /api/summary/settings", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.Settings())
	}))
	handle("PUT /api/summary/settings", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c store.SummaryConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body: " + err.Error()})
			return
		}
		st, err := s.SetSettings(c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNoTurn):
		code = http.StatusNotFound
	case errors.Is(err, ErrNoEngine):
		code = http.StatusConflict
	case errors.Is(err, store.ErrInvalid):
		code = http.StatusBadRequest
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

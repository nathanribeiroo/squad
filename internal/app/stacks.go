package app

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/notes"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Stacks of notes, from the canvas only (stacking gives the flights wired
// to either side the notes of the other):
//
//	POST /api/notes/{id}/stack    {onto} → the stack's notes
//	POST /api/notes/{id}/unstack  {all?} → the notes that changed
//
// A note that leaves a stack keeps the stack's wires.
func registerStacks(srv *server.Server, notesSvc *notes.Service) {
	srv.HandleSession("POST /api/notes/{id}/stack", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Onto string `json:"onto"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, "invalid body")
			return
		}
		// A draft gets its file before it joins a stack (agents read notes by file).
		for _, id := range []string{r.PathValue("id"), body.Onto} {
			if _, err := notesSvc.Finalize(id); err != nil {
				writeStackError(w, err)
				return
			}
		}
		ch, err := srv.Store().Stack(r.PathValue("id"), body.Onto)
		if err != nil {
			writeStackError(w, err)
			return
		}
		srv.PublishStack(ch)
		writeJSON(w, srv.Store().StackMembers(body.Onto))
	}))
	srv.HandleSession("POST /api/notes/{id}/unstack", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			All bool `json:"all"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, "invalid body")
				return
			}
		}
		ch, err := srv.Store().Unstack(r.PathValue("id"), body.All, true)
		if err != nil {
			writeStackError(w, err)
			return
		}
		srv.PublishStack(ch)
		writeJSON(w, ch.Nodes)
	}))
}

func writeStackError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, store.ErrInvalid):
		code = http.StatusBadRequest
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(apierr.Body(err))
}

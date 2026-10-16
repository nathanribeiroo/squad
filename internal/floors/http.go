package floors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/store"
)

// Register adds the floors API to a workspace's routes:
//
//	GET    /api/floors               floors and config (State)
//	POST   /api/floors               create {name, branch?, existing?}
//	PUT    /api/floors/config        {copy: [...], setup: "..."}
//	GET    /api/floors/{id}/changes  uncommitted files and commits (Changes)
//	GET    /api/floors/{id}/log      the setup command's output (text)
//	POST   /api/floors/{id}/land     {merge?, into?} → LandResult
//	DELETE /api/floors/{id}          discard
//
// A refused land is a 409 with "files" (uncommitted) or "conflicts".
func (s *Service) Register(handle func(pattern string, h http.Handler)) {
	handle("GET /api/floors", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, s.State())
	}))
	handle("POST /api/floors", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if !readJSON(w, r, &req) {
			return
		}
		f, err := s.Create(context.WithoutCancel(r.Context()), req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, f)
	}))
	handle("PUT /api/floors/config", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c store.FloorConfig
		if !readJSON(w, r, &c) {
			return
		}
		c, err := s.SetConfig(c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}))
	handle("GET /api/floors/{id}/changes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Changes(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}))
	handle("GET /api/floors/{id}/log", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.o.Canvas.Store().Floor(r.PathValue("id")); !ok {
			writeErr(w, ErrNotFound)
			return
		}
		log, err := s.SetupLog(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(log))
	}))
	handle("POST /api/floors/{id}/land", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req LandRequest
		if !readJSON(w, r, &req) {
			return
		}
		res, err := s.Land(context.WithoutCancel(r.Context()), r.PathValue("id"), req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
	handle("DELETE /api/floors/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Discard(context.WithoutCancel(r.Context()), r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.ContentLength == 0 {
		return true // every field is optional somewhere (land {})
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body: " + err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var dirtyErr *DirtyError
	var conflict *ConflictError
	switch {
	case errors.As(err, &dirtyErr):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "files": dirtyErr.Files})
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "conflicts": conflict.Files})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrInvalid), errors.Is(err, store.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

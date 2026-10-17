package floors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Register adds the floors API to a workspace's routes:
//
//	GET    /api/floors               floors and config (State)
//	POST   /api/floors               create {name, branch?, existing?}
//	PUT    /api/floors/config        {copy: [...], setup: "..."}
//	GET    /api/floors/status        branches and uncommitted changes (Status)
//	GET    /api/branches             the repository's branches
//	GET    /api/floors/{id}/changes  uncommitted files and commits (Changes)
//	GET    /api/floors/{id}/log      the setup command's output (text)
//	POST   /api/floors/{id}/land     {merge?, into?} → LandResult
//	DELETE /api/floors/{id}          discard
//	GET    /api/copy/changes         the session's copy: uncommitted files and commits
//	GET    /api/copy/log             the copy's setup output (text)
//	POST   /api/copy/merge           {into?} → LandResult; the copy stays
//
// A refused land or merge is a 409 with "files" (uncommitted: on the floor,
// in the copy, or with "folder" and "branch" where the target branch is
// checked out) or "conflicts".
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
	handle("GET /api/floors/status", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Status(r.Context()))
	}))
	handle("GET /api/branches", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := s.Branches(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string][]string{"branches": b})
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
	handle("GET /api/copy/changes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := s.CopyChanges(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	}))
	handle("GET /api/copy/log", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		log, err := s.CopyLog()
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(log))
	}))
	handle("POST /api/copy/merge", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req LandRequest
		if !readJSON(w, r, &req) {
			return
		}
		res, err := s.MergeCopy(context.WithoutCancel(r.Context()), req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
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
		body := apierr.Body(err)
		body["files"] = dirtyErr.Files
		if dirtyErr.Folder != "" {
			body["folder"], body["branch"] = dirtyErr.Folder, dirtyErr.Branch
		}
		writeJSON(w, http.StatusConflict, body)
	case errors.As(err, &conflict):
		body := apierr.Body(err)
		body["conflicts"] = conflict.Files
		writeJSON(w, http.StatusConflict, body)
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, apierr.Body(err))
	case errors.Is(err, ErrInvalid), errors.Is(err, store.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, apierr.Body(err))
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

package cargo

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Register installs the cargo routes, for the canvas only (the session's
// cookie or token, never the agents' /cli). A path is absolute, starts with
// ~, or is relative to the folder the floor (?floor=) or the session works
// in; "" is that folder.
//
//	GET   /api/cargo/list?path=&floor=&hidden=1      a folder: its entries and where to go from it
//	GET   /api/cargo/search?path=&q=&floor=&hidden=1  entries under a folder whose names contain q
//	GET   /api/cargo/file?path=&floor=                a file, for the preview
//	PATCH /api/cargo/{id}                             {name, path}: rename it or open another folder
//	PUT   /api/cargo/{id}/watch                       {dirs}: the folders its tree shows, to watch
func (s *Service) Register(handle func(pattern string, h http.Handler)) {
	handle("GET /api/cargo/list", http.HandlerFunc(s.handleList))
	handle("GET /api/cargo/search", http.HandlerFunc(s.handleSearch))
	handle("GET /api/cargo/file", http.HandlerFunc(s.handleFile))
	handle("PATCH /api/cargo/{id}", http.HandlerFunc(s.handleUpdate))
	handle("PUT /api/cargo/{id}/watch", http.HandlerFunc(s.handleWatch))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) { writeJSON(w, code, apierr.Body(err)) }

func errCode(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrDuplicate):
		return http.StatusConflict
	case errors.Is(err, store.ErrInvalid):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// Folder is GET /api/cargo/list: a folder and where the explorer can go
// from it.
type Folder struct {
	Listing
	Parent    string   `json:"parent,omitempty"` // "" at a root
	Home      string   `json:"home,omitempty"`
	Workspace string   `json:"workspace"` // the folder the floor (or session) works in
	Roots     []string `json:"roots,omitempty"`
	Sep       string   `json:"sep"` // the system's path separator
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	floor := q.Get("floor")
	dir, err := s.folder(floor, q.Get("path"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	l, err := List(dir, q.Get("hidden") == "1", MaxEntries)
	if err != nil {
		writeErr(w, http.StatusForbidden, apierr.With(errors.New("can't read the folder: "+dir), "cargo.unreadable", "folder", dir))
		return
	}
	f := Folder{Listing: l, Workspace: s.canvas.ResolvePath(floor, ""), Roots: roots(), Sep: string(filepath.Separator)}
	if parent := filepath.Dir(dir); parent != dir {
		f.Parent = parent
	}
	if home, err := os.UserHomeDir(); err == nil {
		f.Home = home
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir, err := s.folder(q.Get("floor"), q.Get("path"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	hits, more, err := Search(dir, q.Get("q"), q.Get("hidden") == "1")
	if err != nil {
		writeErr(w, http.StatusForbidden, apierr.With(errors.New("can't read the folder: "+dir), "cargo.unreadable", "folder", dir))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": dir, "hits": hits, "more": more})
}

// handleFile serves a file for the explorer's preview, with ranges (a video
// seeks without downloading it all). The canvas only shows it as text, an
// image, a video, audio or a PDF; opened alone it can't run scripts (a
// sandbox), and nothing is sniffed into HTML or a script.
func (s *Service) handleFile(w http.ResponseWriter, r *http.Request) {
	p := s.canvas.ResolvePath(r.URL.Query().Get("floor"), strings.TrimSpace(r.URL.Query().Get("path")))
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.With(errors.New("file not found: "+p), "cargo.file_not_found"))
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		writeErr(w, http.StatusBadRequest, errors.New("not a file: "+p))
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "no-cache")
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
	switch {
	case KindOf(p) == "text" && !strings.HasPrefix(ct, "image/"):
		ct = "text/plain; charset=utf-8" // never HTML or a script
	case ct == "", strings.HasPrefix(ct, "text/html"), strings.Contains(ct, "javascript"):
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

func (s *Service) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	n, err := s.Update(r.PathValue("id"), body.Name, body.Path)
	if err != nil {
		writeErr(w, errCode(err), err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Service) handleWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dirs []string `json:"dirs"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if n, ok := s.store.Node(r.PathValue("id")); !ok || n.Cargo == nil {
		writeErr(w, http.StatusNotFound, store.ErrNotFound)
		return
	}
	s.Watch(r.PathValue("id"), body.Dirs)
	w.WriteHeader(http.StatusNoContent)
}

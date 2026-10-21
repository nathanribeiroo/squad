package cargo

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// canvas resolves paths like the server does on the ground floor: relative
// to the workspace folder.
type canvas struct{ ws string }

func (c canvas) ResolvePath(_, p string) string {
	switch {
	case p == "":
		return c.ws
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	}
	return filepath.Join(c.ws, p)
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// project is a workspace folder with a few files.
func project(t *testing.T) string {
	ws := t.TempDir()
	write(t, filepath.Join(ws, "videos", "parte 10.mp4"), "x")
	write(t, filepath.Join(ws, "videos", "parte 2.mp4"), "x")
	write(t, filepath.Join(ws, "videos", "capa.png"), "x")
	write(t, filepath.Join(ws, "src", "api", "routes.go"), "package api")
	write(t, filepath.Join(ws, "src", "README.md"), "# src")
	write(t, filepath.Join(ws, ".env"), "SECRET=1")
	write(t, filepath.Join(ws, "node_modules", "lib", "routes.js"), "")
	write(t, filepath.Join(ws, ".git", "routes"), "")
	write(t, filepath.Join(ws, "Makefile"), "all:")
	return ws
}

func newService(t *testing.T, ws string) (*Service, *store.Store, *events.Bus) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "workspace.json"), store.Workspace{ID: "w", Name: "app", Cwd: ws})
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	return New(st, bus, canvas{ws}, nil), st, bus
}

// Folders first, then files, in natural order; dot entries only when asked;
// what each file previews as; how many items each folder holds.
func TestList(t *testing.T) {
	ws := project(t)
	l, err := List(ws, false, MaxEntries)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range l.Entries {
		names = append(names, e.Name)
	}
	if want := []string{"node_modules", "src", "videos", "Makefile"}; !slices.Equal(names, want) || l.Hidden != 2 {
		t.Fatalf("list: %v (hidden %d), want %v", names, l.Hidden, want)
	}
	if e := l.Entries[2]; !e.Dir || e.Items != 3 {
		t.Fatalf("videos: %+v", e)
	}
	if e := l.Entries[3]; e.Dir || e.Kind != "text" || e.Size != 4 {
		t.Fatalf("Makefile: %+v", e)
	}
	v, _ := List(filepath.Join(ws, "videos"), false, MaxEntries)
	if v.Entries[0].Name != "capa.png" || v.Entries[0].Kind != "image" || v.Entries[1].Name != "parte 2.mp4" || v.Entries[2].Name != "parte 10.mp4" || v.Entries[2].Kind != "video" {
		t.Fatalf("natural order and kinds: %+v", v.Entries)
	}
	h, _ := List(ws, true, 2)
	if len(h.Entries) != 2 || h.More != 4 || h.Hidden != 0 || h.Entries[0].Name != ".git" {
		t.Fatalf("hidden and limit: %+v", h)
	}
}

// Search finds names under a folder, nearest first, never inside
// node_modules or .git, and dot entries only when asked.
func TestSearch(t *testing.T) {
	ws := project(t)
	hits, more, err := Search(ws, "ROUTES", false)
	if err != nil || more || len(hits) != 1 || hits[0].Path != "src/api/routes.go" || hits[0].Kind != "text" {
		t.Fatalf("search: %+v more=%v err=%v", hits, more, err)
	}
	hits, _, _ = Search(ws, "parte", false)
	if len(hits) != 2 || hits[0].Path != "videos/parte 2.mp4" {
		t.Fatalf("natural order: %+v", hits)
	}
	if hits, _, _ = Search(ws, "env", false); len(hits) != 0 {
		t.Fatalf("a dot file without hidden: %+v", hits)
	}
	if hits, _, _ = Search(ws, "env", true); len(hits) != 1 || hits[0].Path != ".env" {
		t.Fatalf("a dot file with hidden: %+v", hits)
	}
	if hits, _, _ = Search(ws, "src", false); len(hits) != 1 || !hits[0].Dir {
		t.Fatalf("a folder: %+v", hits)
	}
}

// A new cargo opens the workspace folder unless told otherwise, keeps a
// folder of the project relative to it (it follows floors and travels in
// plans) and others absolute, and takes its folder's name.
func TestPrepareNode(t *testing.T) {
	ws := project(t)
	svc, st, _ := newService(t, ws)
	n := store.Node{Kind: store.KindCargo}
	if err := svc.PrepareNode(&n); err != nil || n.Cargo.Path != "." || n.Cargo.Name != filepath.Base(ws) {
		t.Fatalf("default: %+v %v", n.Cargo, err)
	}
	n = store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Path: filepath.Join(ws, "videos"), Name: "Vídeos parte 2"}}
	if err := svc.PrepareNode(&n); err != nil || n.Cargo.Path != "videos" || n.Cargo.Name != "Vídeos parte 2" {
		t.Fatalf("inside the project: %+v %v", n.Cargo, err)
	}
	out := t.TempDir()
	n = store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Path: out}}
	if err := svc.PrepareNode(&n); err != nil || n.Cargo.Path != out || n.Cargo.Name != filepath.Base(out) {
		t.Fatalf("outside: %+v %v", n.Cargo, err)
	}
	n = store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Path: filepath.Join(ws, "Makefile")}}
	var ae *apierr.Error
	if err := svc.PrepareNode(&n); !errors.Is(err, store.ErrInvalid) || !errors.As(err, &ae) || ae.Code != "dir.not_found" {
		t.Fatalf("a file is not a folder: %v", err)
	}
	// Stored, renamed and pointed elsewhere; no wires.
	c, err := st.AddNode(store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Name: "Resultados", Path: "videos"}})
	if err != nil {
		t.Fatal(err)
	}
	if u, err := svc.Update(c.ID, "Vídeos", "src"); err != nil || u.Cargo.Name != "Vídeos" || u.Cargo.Path != "src" || svc.Resolve(u) != filepath.Join(ws, "src") {
		t.Fatalf("update: %+v %v", u.Cargo, err)
	}
	if _, err := svc.Update(c.ID, "", filepath.Join(ws, "nope")); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("update to a missing folder: %v", err)
	}
	other, _ := st.AddNode(store.Node{Kind: store.KindNote, Note: &store.NoteData{Title: "N", File: "n.md"}})
	if _, err := st.AddEdge(c.ID, other.ID, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("a wire to a cargo: %v", err)
	}
	if _, err := st.AddNode(store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Path: "videos"}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("a cargo without a name: %v", err)
	}
}

func get(t *testing.T, h http.Handler, target string, header ...string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", target, nil)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// The routes: a folder with where to go from it, a search, a file for the
// preview (never as HTML or a script, with ranges), renaming.
func TestRoutes(t *testing.T) {
	ws := project(t)
	write(t, filepath.Join(ws, "page.html"), "<script>alert(1)</script>")
	svc, st, _ := newService(t, ws)
	mux := http.NewServeMux()
	svc.Register(func(p string, h http.Handler) { mux.Handle(p, h) })

	w := get(t, mux, "/api/cargo/list?path=videos")
	var f Folder
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &f) != nil || f.Path != filepath.Join(ws, "videos") || f.Parent != ws || f.Workspace != ws || len(f.Entries) != 3 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if w = get(t, mux, "/api/cargo/list"); w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"src"`) {
		t.Fatalf("list the workspace: %d %s", w.Code, w.Body)
	}
	if w = get(t, mux, "/api/cargo/list?path=nope"); w.Code != 404 || !strings.Contains(w.Body.String(), "dir.not_found") {
		t.Fatalf("list a missing folder: %d %s", w.Code, w.Body)
	}
	if w = get(t, mux, "/api/cargo/search?q=routes"); w.Code != 200 || !strings.Contains(w.Body.String(), `"path":"src/api/routes.go"`) {
		t.Fatalf("search: %d %s", w.Code, w.Body)
	}

	w = get(t, mux, "/api/cargo/file?path="+url.QueryEscape(filepath.Join(ws, "page.html")))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" || w.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("an HTML file: %d %v", w.Code, w.Header())
	}
	if w = get(t, mux, "/api/cargo/file?path=src/README.md", "Range", "bytes=0-1"); w.Code != 206 || w.Body.String() != "# " {
		t.Fatalf("a range: %d %q", w.Code, w.Body)
	}
	if w = get(t, mux, "/api/cargo/file?path=videos/capa.png"); w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("an image: %v", w.Header())
	}
	if w = get(t, mux, "/api/cargo/file?path=src"); w.Code != 400 {
		t.Fatalf("a folder as a file: %d", w.Code)
	}

	c, _ := st.AddNode(store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Name: "C", Path: "."}})
	r := httptest.NewRequest("PATCH", "/api/cargo/"+c.ID, strings.NewReader(`{"name":"Resultados"}`))
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, r)
	if n, _ := st.Node(c.ID); rw.Code != 200 || n.Cargo.Name != "Resultados" || n.Cargo.Path != "." {
		t.Fatalf("rename: %d %s", rw.Code, rw.Body)
	}
	r = httptest.NewRequest("PUT", "/api/cargo/nope/watch", strings.NewReader(`{"dirs":[]}`))
	rw = httptest.NewRecorder()
	mux.ServeHTTP(rw, r)
	if rw.Code != 404 {
		t.Fatalf("watch an unknown cargo: %d", rw.Code)
	}
}

// A folder a cargo's tree shows tells the canvas when something in it
// changes; not after the cargo stops showing it.
func TestWatch(t *testing.T) {
	ws := project(t)
	svc, _, bus := newService(t, ws)
	svc.Start()
	defer svc.Close()
	ch, unsub := bus.Subscribe()
	defer unsub()
	videos := filepath.Join(ws, "videos")
	svc.Watch("c1", []string{videos, filepath.Join(ws, "nope"), "relative"})
	write(t, filepath.Join(videos, "parte 3.mp4"), "x")
	deadline := time.After(3 * time.Second)
	for got := false; !got; {
		select {
		case ev := <-ch:
			if m, ok := ev.Data.(map[string]string); ok && ev.Type == events.CargoChanged {
				if m["id"] != "c1" || m["dir"] != videos {
					t.Fatalf("change: %v", m)
				}
				got = true
			}
		case <-deadline:
			t.Fatal("no change event")
		}
	}
	svc.Watch("c1", nil)
	write(t, filepath.Join(videos, "parte 4.mp4"), "x")
	select {
	case ev := <-ch:
		if ev.Type == events.CargoChanged {
			t.Fatalf("a change after unwatching: %v", ev.Data)
		}
	case <-time.After(600 * time.Millisecond):
	}
}

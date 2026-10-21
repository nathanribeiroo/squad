//go:build !windows

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/plans"
	"github.com/nathanribeiroo/squad/internal/store"
)

func (e *env) call(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Host = strings.TrimPrefix(e.app.Server.URL(), "http://")
	r.Header.Set("Authorization", "Bearer tok")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.app.Server.Handler().ServeHTTP(w, r)
	return w
}

// A team saved as a flight plan flies again beside itself: new names where
// taken, the NOTAM with its content, the wires; its commands are reviewed.
func TestPlanSaveAndApply(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	note, err := e.app.Server.AddNode(store.Node{Kind: store.KindNote, X: 0, Y: 600, Note: &store.NoteData{Title: "Plano"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.app.Notes.Write(note, "- [ ] tela de login\n", "Você"); err != nil {
		t.Fatal(err)
	}
	e.connect(a, note)

	w := e.call("POST", "/api/plans", map[string]any{"name": "Trio", "scope": "session", "notesContent": true})
	if w.Code != http.StatusCreated {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	var p plans.Plan
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID != "trio" || len(p.Flights) != 2 || len(p.Notes) != 1 || p.Notes[0].Content != "- [ ] tela de login\n" || len(p.Links) != 2 || len(p.Commands()) == 0 {
		t.Fatalf("plan: %+v", p)
	}
	if p.Flights[0].X != 0 || p.Flights[0].Y != 0 {
		t.Fatalf("positions start at the plan's corner: %+v", p.Flights[0].Box)
	}

	// Without the review, nothing flies.
	if w := e.call("POST", "/api/plans/trio/apply", map[string]any{}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "plan.review") {
		t.Fatalf("apply unreviewed: %d %s", w.Code, w.Body)
	}
	w = e.call("POST", "/api/plans/trio/apply", map[string]any{"commands": p.Commands()})
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	var res PlanResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.Nodes) != 3 || len(res.Warnings) != 0 {
		t.Fatalf("result: %+v", res)
	}
	st := e.app.Server.Store()
	a2, ok1 := st.TerminalByName("A 2")
	b2, ok2 := st.TerminalByName("B 2")
	if !ok1 || !ok2 || !st.Connected(a2.ID, b2.ID) || a2.X <= b.X {
		t.Fatalf("new flights: %+v %+v (beside the old ones: %v > %v)", a2, b2, a2.X, b.X)
	}
	var note2 store.Node
	for _, n := range st.Snapshot().Nodes {
		if n.Note != nil && n.Note.Title == "Plano 2" {
			note2 = n
		}
	}
	if c, _ := e.app.Notes.Read(note2.Note.File); c != "- [ ] tela de login\n" || !st.Connected(a2.ID, note2.ID) {
		t.Fatalf("note: %+v %q", note2, c)
	}
	e.waitScreen(a2.ID, "fakeagent ready", 10e9)
}

// A plan's level needs git; without it, the level's items land on the level
// asked for, with a warning.
func TestPlanLevelWithoutGit(t *testing.T) {
	e := newEnv(t, fastLimits)
	p, err := e.app.plans.Save(plans.Plan{Name: "Frentes", Levels: []plans.Level{{Key: "l1", Name: "Login"}},
		Notes: []plans.Note{{Key: "n1", Title: "Escopo", Level: "l1", Box: plans.Box{W: 400, H: 350}}}})
	if err != nil {
		t.Fatal(err)
	}
	w := e.call("POST", "/api/plans/"+p.ID+"/apply", map[string]any{})
	var res PlanResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || len(res.Nodes) != 1 || len(res.Floors) != 0 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "Login") {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	if n, _ := e.app.Server.Store().Node(res.Nodes[0]); n.Floor != "" {
		t.Fatalf("on floor %q", n.Floor)
	}
	// Bad requests.
	if w := e.call("POST", "/api/plans", map[string]any{"name": "", "scope": "session"}); w.Code != 400 {
		t.Fatalf("no name: %d", w.Code)
	}
	if w := e.call("POST", "/api/plans", map[string]any{"name": "Nada", "scope": "selection"}); w.Code != 400 || !strings.Contains(w.Body.String(), "plan.empty") {
		t.Fatalf("empty: %d %s", w.Code, w.Body)
	}
	if w := e.call("POST", "/api/plans/nope/apply", map[string]any{}); w.Code != 404 {
		t.Fatalf("unknown plan: %d", w.Code)
	}
}

// In a repository, a plan's level becomes a new level; flying the plan
// again makes "Login 2".
func TestPlanLevels(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := newEnv(t, fastLimits)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "T"}, {"config", "user.email", "t@e.com"}, {"commit", "-q", "--allow-empty", "-m", "first"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = e.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	p, err := e.app.plans.Save(plans.Plan{Name: "Frentes", Levels: []plans.Level{{Key: "l1", Name: "Login", Branch: "login"}},
		Notes: []plans.Note{{Key: "n1", Title: "Escopo", Level: "l1", Box: plans.Box{W: 400, H: 350}}}})
	if err != nil {
		t.Fatal(err)
	}
	st := e.app.Server.Store()
	for i, want := range []string{"Login", "Login 2"} {
		w := e.call("POST", "/api/plans/"+p.ID+"/apply", map[string]any{})
		var res PlanResult
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != 200 || len(res.Floors) != 1 || len(res.Warnings) != 0 {
			t.Fatalf("apply %d: %d %s", i, w.Code, w.Body)
		}
		f, _ := st.Floor(res.Floors[0])
		n, _ := st.Node(res.Nodes[0])
		if f.Name != want || n.Floor != f.ID {
			t.Fatalf("apply %d: floor %+v, note on %q", i, f, n.Floor)
		}
	}
	t.Cleanup(func() {
		for _, f := range st.Floors() {
			_ = e.app.Floors.Discard(context.Background(), f.ID)
		}
	})
}

// A cargo travels in a flight plan with its folder (relative inside the
// project) and its name; one whose folder isn't there is left out, with a
// warning.
func TestPlanKeepsCargo(t *testing.T) {
	e := newEnv(t, fastLimits)
	ws := e.app.Server.Store().Meta().Cwd
	if err := os.MkdirAll(filepath.Join(ws, "out", "videos"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := e.app.Server.AddNode(store.Node{Kind: store.KindCargo, Cargo: &store.CargoData{Name: "Vídeos parte 2", Path: filepath.Join(ws, "out", "videos")}})
	if err != nil || c.Cargo.Path != "out/videos" || c.W == 0 {
		t.Fatalf("cargo: %+v %v", c, err)
	}
	if w := e.call("PATCH", "/api/cargo/"+c.ID, map[string]any{"name": "Resultados"}); w.Code != 200 {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}
	w := e.call("POST", "/api/plans", map[string]any{"name": "Carga", "scope": "session"})
	var p plans.Plan
	if _ = json.Unmarshal(w.Body.Bytes(), &p); w.Code != http.StatusCreated || len(p.Cargo) != 1 || p.Cargo[0].Path != "out/videos" || p.Cargo[0].Name != "Resultados" {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if w = e.call("POST", "/api/plans/carga/apply", map[string]any{"commands": p.Commands()}); w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	var cargo []store.CargoData
	for _, n := range e.app.Server.Store().Snapshot().Nodes {
		if n.Cargo != nil {
			cargo = append(cargo, *n.Cargo)
		}
	}
	if len(cargo) != 2 || cargo[1] != (store.CargoData{Name: "Resultados", Path: "out/videos"}) {
		t.Fatalf("cargo after the plan: %+v", cargo)
	}
	if err := os.RemoveAll(filepath.Join(ws, "out")); err != nil {
		t.Fatal(err)
	}
	w = e.call("POST", "/api/plans/carga/apply", map[string]any{"commands": p.Commands()})
	var res PlanResult
	if _ = json.Unmarshal(w.Body.Bytes(), &res); w.Code != 200 || len(res.Nodes) != 0 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "out/videos") {
		t.Fatalf("a missing folder: %d %+v", w.Code, res)
	}
}

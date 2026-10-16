//go:build !windows

package app

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
)

// gitRepo turns the workspace folder into <dir>/repo, a repository on main
// with one commit.
func (e *env) gitRepo() string {
	e.t.Helper()
	repo := filepath.Join(e.dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "web"), 0o755); err != nil {
		e.t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "web", "app.js"), []byte("1\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("setup-done\n"), 0o644)
	e.git(repo, "init", "-q", "-b", "main")
	e.git(repo, "config", "user.name", "Test")
	e.git(repo, "config", "user.email", "test@example.com")
	e.git(repo, "add", ".")
	e.git(repo, "commit", "-q", "-m", "first")
	os.WriteFile(filepath.Join(repo, ".env"), []byte("KEY=1\n"), 0o644)
	if _, err := e.app.Server.Store().UpdateMeta(func(m *store.Meta) error { m.Cwd = repo; return nil }); err != nil {
		e.t.Fatal(err)
	}
	return repo
}

func (e *env) git(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// api calls the workspace's HTTP API as the browser does.
func (e *env) api(method, path, body string) (int, map[string]any) {
	e.t.Helper()
	req := httptest.NewRequest(method, e.app.Server.URL()+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.app.Server.Handler().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestManagerUsesFloors(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := newEnv(t, fastLimits)
	repo := e.gitRepo()
	if code, _ := e.api("PUT", "/api/floors/config", `{"copy":[".env"],"setup":"touch setup-done"}`); code != 200 {
		t.Fatalf("config: %d", code)
	}
	plan := e.agent("Plano")
	e.setRole(plan, "plan")
	ana := e.agent("Ana")

	if r := e.run(ana, "floor", "create", "Login"); r.Code != 403 {
		t.Fatalf("a non-manager created a floor: %d %s", r.Code, r.Body)
	}
	if r := e.run(ana, "floor", "list"); r.Code != 200 || !strings.Contains(r.Body, "No floors yet") {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	r := e.run(plan, "floor", "create", "Login")
	if r.Code != 200 || !strings.Contains(r.Body, `Created floor "Login": branch login, from the last commit of main`) ||
		!strings.Contains(r.Body, "Copied from the ground floor (not to be committed): .env") || !strings.Contains(r.Body, "Setup (touch setup-done) done") {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	f, ok := e.app.Server.Store().FindFloor("login")
	if !ok || f.Path != filepath.Join(e.dir, ".squad", "floors", "repo--login") {
		t.Fatalf("floor: %+v", f)
	}

	// An agent on the floor works in the floor's copy, even with a folder.
	r = e.run(plan, "new", "Bia", "--floor", "LOGIN", "--agent", "custom", "--command", e.fakeCommand("Bia"))
	if r.Code != 200 || !strings.Contains(r.Body, `on floor "Login"`) {
		t.Fatalf("new on the floor: %d %s", r.Code, r.Body)
	}
	r = e.run(plan, "new", "Sh", "--floor", "Login", "--agent", "shell", "--cwd", "web", "--command", "pwd")
	if r.Code != 200 {
		t.Fatalf("shell on the floor: %d %s", r.Code, r.Body)
	}
	bia, _ := e.node("Bia")
	sh, _ := e.node("Sh")
	if bia.Floor != "login" || sh.Floor != "login" || !e.app.Server.Store().Connected(plan.ID, bia.ID) {
		t.Fatalf("Bia: %+v", bia)
	}
	e.waitScreen(sh.ID, filepath.Join(f.Path, "web"), 10e9)
	// A ground floor folder given in full maps to its copy.
	if r := e.run(plan, "new", "Sh2", "--floor", "Login", "--agent", "shell", "--cwd", filepath.Join(repo, "web"), "--command", "pwd"); r.Code != 200 {
		t.Fatalf("shell with a full path: %d %s", r.Code, r.Body)
	}
	sh2, _ := e.node("Sh2")
	e.waitScreen(sh2.ID, filepath.Join(f.Path, "web"), 10e9)
	if r := e.run(plan, "list"); !strings.Contains(r.Body, `name: "Bia", agent: custom, floor: "Login"`) {
		t.Fatalf("manager's list:\n%s", r.Body)
	}
	if r := e.run(bia, "list"); !strings.Contains(r.Body, `floor: "Login" (branch login`) || !strings.Contains(r.Body, `name: "Plano", agent: custom, role: "Planejador", floor: "ground"`) {
		t.Fatalf("Bia's list:\n%s", r.Body)
	}
	if r := e.run(bia, "floor", "list"); !strings.Contains(r.Body, `You are on floor "Login"`) || !strings.Contains(r.Body, `"Login": branch login (from main), ready, 3 terminal(s)`) {
		t.Fatalf("floor list:\n%s", r.Body)
	}
	// Nodes an agent on the floor creates stay on it.
	if r := e.run(bia, "portal", "create", "localhost:5173", "App"); r.Code != 200 {
		t.Fatalf("portal: %d %s", r.Code, r.Body)
	}
	if app, _ := e.node("App"); app.Floor != "login" {
		t.Fatalf("portal floor %q", app.Floor)
	}

	// Uncommitted work blocks landing; the API shows it.
	os.WriteFile(filepath.Join(f.Path, "login.js"), []byte("login()\n"), 0o644)
	if code, body := e.api("GET", "/api/floors/login/changes", ""); code != 200 || len(body["files"].([]any)) != 1 {
		t.Fatalf("changes: %d %v", code, body)
	}
	if r := e.run(plan, "floor", "land", "Login", "--merge"); r.Code != 409 || !strings.Contains(r.Body, "login.js") {
		t.Fatalf("land with uncommitted work: %d %s", r.Code, r.Body)
	}
	if code, body := e.api("POST", "/api/floors/login/land", `{"merge":true}`); code != 409 || body["files"] == nil {
		t.Fatalf("API land: %d %v", code, body)
	}
	// Committing the copied .env (it may hold secrets) blocks a merge.
	e.git(f.Path, "add", ".")
	e.git(f.Path, "commit", "-q", "-m", "login page")
	if r := e.run(plan, "floor", "land", "Login", "--merge"); r.Code != 400 || !strings.Contains(r.Body, "copied from the ground floor") || !strings.Contains(r.Body, ".env") {
		t.Fatalf("land with .env committed: %d %s", r.Code, r.Body)
	}
	e.git(f.Path, "rm", "-q", "--cached", ".env")
	e.git(f.Path, "commit", "-q", "-m", "keep .env out")
	if r := e.run(bia, "floor", "land", "Login"); r.Code != 403 {
		t.Fatalf("Bia landed: %d %s", r.Code, r.Body)
	}
	r = e.run(plan, "floor", "land", "Login", "--merge")
	if r.Code != 200 || !strings.Contains(r.Body, "merged login into main") {
		t.Fatalf("land: %d %s", r.Code, r.Body)
	}
	if _, err := os.Stat(filepath.Join(repo, "login.js")); err != nil {
		t.Fatal("merged work missing on the ground floor")
	}
	for _, name := range []string{"Bia", "Sh", "Sh2", "App"} {
		if _, ok := e.node(name); ok {
			t.Fatalf("%s survived the landing", name)
		}
	}
	if _, err := os.Stat(f.Path); !os.IsNotExist(err) {
		t.Fatal("floor folder left behind")
	}

	// The user discards through the API.
	if code, body := e.api("POST", "/api/floors", `{"name":"Rascunho"}`); code != 201 || body["branch"] != "rascunho" {
		t.Fatalf("API create: %d %v", code, body)
	}
	if code, body := e.api("GET", "/api/floors", ""); code != 200 || len(body["floors"].([]any)) != 1 {
		t.Fatalf("API list: %d %v", code, body)
	}
	if code, _ := e.api("DELETE", "/api/floors/rascunho", ""); code != 204 {
		t.Fatalf("discard: %d", code)
	}
	if e.git(repo, "branch", "--list", "rascunho") != "" {
		t.Fatal("discard kept the branch")
	}
	if code, _ := e.api("DELETE", "/api/floors/rascunho", ""); code != 404 {
		t.Fatalf("discard twice: %d", code)
	}
}

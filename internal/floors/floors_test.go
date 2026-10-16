package floors

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

type canvas struct {
	st  *store.Store
	bus *events.Bus
}

func (c canvas) Store() *store.Store { return c.st }
func (c canvas) Bus() *events.Bus    { return c.bus }
func (c canvas) RemoveNode(id string) error {
	_, err := c.st.RemoveNode(id)
	return err
}

// sh runs a git command in dir and returns its output.
func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes <tmp>/code/app, a repository on main with one commit, and a
// workspace for it whose files live elsewhere.
func newRepo(t *testing.T) (repo string, svc *Service, st *store.Store) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	repo = filepath.Join(tmp, "code", "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	sh(t, repo, "init", "-q", "-b", "main")
	sh(t, repo, "config", "user.name", "Test")
	sh(t, repo, "config", "user.email", "test@example.com")
	sh(t, repo, "config", "commit.gpgsign", "false")
	write(t, filepath.Join(repo, "README.md"), "one\ntwo\nthree\n")
	write(t, filepath.Join(repo, ".gitignore"), "setup.txt\nnode_modules/\n")
	write(t, filepath.Join(repo, "web", "index.html"), "<h1>hi</h1>\n")
	sh(t, repo, "add", ".")
	sh(t, repo, "commit", "-q", "-m", "first")
	write(t, filepath.Join(repo, ".env"), "SECRET=1\n") // untracked, not ignored
	st, err := store.Open(filepath.Join(tmp, "workspace.json"), store.Workspace{ID: "w", Name: "app", Cwd: repo})
	if err != nil {
		t.Fatal(err)
	}
	svc = New(Options{Canvas: canvas{st, events.NewBus()}})
	t.Cleanup(svc.Close)
	return repo, svc, st
}

func TestCreateCommitAndLandWithMerge(t *testing.T) {
	repo, svc, st := newRepo(t)
	ctx := context.Background()
	if _, err := st.SetFloorConfig(store.FloorConfig{Copy: []string{".env*"}, Setup: "echo prepared > setup.txt"}); err != nil {
		t.Fatal(err)
	}
	f, err := svc.Create(ctx, CreateRequest{Name: "Página de Login", By: "T1"})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(filepath.Dir(repo), ".squad", "floors", "app--pagina-de-login")
	if f.ID != "pagina-de-login" || f.Branch != "pagina-de-login" || f.Base != "main" || f.Path != wantPath || f.Cwd != wantPath || !f.NewBranch || f.CreatedBy != "T1" {
		t.Fatalf("floor: %+v", f)
	}
	if f.Status != store.FloorPreparing {
		t.Fatalf("status: %s", f.Status)
	}
	if !svc.WaitReady(ctx, f.ID, 10*time.Second) {
		t.Fatal("setup never finished")
	}
	if f, _ = st.Floor(f.ID); f.Status != store.FloorReady || f.Error != "" {
		t.Fatalf("after setup: %+v", f)
	}
	if b, _ := os.ReadFile(filepath.Join(f.Path, "setup.txt")); strings.TrimSpace(string(b)) != "prepared" {
		t.Fatalf("setup output: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(f.Path, ".env")); string(b) != "SECRET=1\n" || len(f.Copied) != 1 || f.Copied[0] != ".env" {
		t.Fatalf(".env: %q, copied %v", b, f.Copied)
	}
	if log, _ := svc.SetupLog(f.ID); !strings.Contains(log, "echo prepared") {
		t.Fatalf("setup log: %q", log)
	}
	if _, err := svc.Create(ctx, CreateRequest{Name: "página de login"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("same name: %v", err)
	}

	// The copied .env doesn't count as a change.
	ch, err := svc.Changes(ctx, f.ID)
	if err != nil || len(ch.Files) != 0 || ch.Ahead != 0 {
		t.Fatalf("changes: %+v %v", ch, err)
	}
	write(t, filepath.Join(f.Path, "login.go"), "package login\n")
	var dirtyErr *DirtyError
	if _, err := svc.Land(ctx, f.ID, LandRequest{Merge: true}); !errors.As(err, &dirtyErr) || dirtyErr.Files[0] != "login.go" {
		t.Fatalf("land with uncommitted work: %v", err)
	}
	sh(t, f.Path, "add", "login.go")
	sh(t, f.Path, "commit", "-q", "-m", "login")
	if ch, _ = svc.Changes(ctx, f.ID); ch.Ahead != 1 || len(ch.Commits) != 1 || ch.Commits[0].Subject != "login" {
		t.Fatalf("changes after commit: %+v", ch)
	}

	// The ground floor (main is checked out there) must be clean to merge.
	write(t, filepath.Join(repo, "README.md"), "changed\n")
	if _, err := svc.Land(ctx, f.ID, LandRequest{Merge: true}); !errors.As(err, &dirtyErr) || !strings.Contains(err.Error(), "branch main") {
		t.Fatalf("land into a dirty ground floor: %v", err)
	}
	sh(t, repo, "checkout", "-q", "README.md")

	// Notes go down to the ground floor; terminals close.
	term, _ := st.AddNode(store.Node{Kind: store.KindTerminal, Floor: f.ID, Terminal: &store.TerminalData{Name: "Bia", Agent: "shell"}})
	note, _ := st.AddNode(store.Node{Kind: store.KindNote, Floor: f.ID, W: 400, H: 350, Note: &store.NoteData{Title: "Plano", File: "Plano.md"}})

	res, err := svc.Land(ctx, f.ID, LandRequest{Merge: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Merged || res.Into != "main" || res.Commit == "" {
		t.Fatalf("result: %+v", res)
	}
	if parents := sh(t, repo, "log", "-1", "--format=%P"); len(strings.Fields(parents)) != 2 {
		t.Fatalf("not a merge commit: %q", parents)
	}
	if sh(t, repo, "log", "-1", "--format=%s") != "Merge branch 'pagina-de-login' into main" {
		t.Fatal("merge message")
	}
	if _, err := os.Stat(filepath.Join(repo, "login.go")); err != nil {
		t.Fatal("merged file missing on the ground floor")
	}
	if isDir(f.Path) || isDir(filepath.Join(filepath.Dir(repo), ".squad")) {
		t.Fatal("floor folder (or the empty .squad) left behind")
	}
	if sh(t, repo, "branch", "--list", f.Branch) == "" {
		t.Fatal("landing deleted the branch")
	}
	if _, ok := st.Floor(f.ID); ok {
		t.Fatal("floor still listed")
	}
	if _, ok := st.Node(term.ID); ok {
		t.Fatal("the floor's terminal survived")
	}
	if n, ok := st.Node(note.ID); !ok || n.Floor != "" {
		t.Fatalf("note: %+v %v", n, ok)
	}
	if strings.Contains(sh(t, repo, "worktree", "list"), "pagina") {
		t.Fatal("worktree still registered")
	}
}

func TestMergeIntoBranchNotCheckedOut(t *testing.T) {
	repo, svc, _ := newRepo(t)
	ctx := context.Background()
	f, err := svc.Create(ctx, CreateRequest{Name: "api"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.Path, "api.go"), "package api\n")
	sh(t, f.Path, "add", "api.go")
	sh(t, f.Path, "commit", "-q", "-m", "api")
	sh(t, repo, "checkout", "-q", "-b", "other") // nobody has main checked out now
	res, err := svc.Land(ctx, f.ID, LandRequest{Into: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Merged || sh(t, repo, "rev-parse", "main") != res.Commit {
		t.Fatalf("main not moved: %+v", res)
	}
	if files := sh(t, repo, "ls-tree", "--name-only", "main"); !strings.Contains(files, "api.go") {
		t.Fatalf("main's tree: %s", files)
	}
	if sh(t, repo, "branch", "--show-current") != "other" || sh(t, repo, "status", "--porcelain", "-uno") != "" {
		t.Fatal("the ground floor's folder changed")
	}
}

func TestConflictsChangeNothing(t *testing.T) {
	for _, checkedOut := range []bool{true, false} {
		repo, svc, st := newRepo(t)
		ctx := context.Background()
		f, err := svc.Create(ctx, CreateRequest{Name: "readme"})
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(f.Path, "README.md"), "one\nTWO (floor)\nthree\n")
		sh(t, f.Path, "commit", "-q", "-am", "floor edit")
		write(t, filepath.Join(repo, "README.md"), "one\nTWO (ground)\nthree\n")
		sh(t, repo, "commit", "-q", "-am", "ground edit")
		if !checkedOut {
			sh(t, repo, "checkout", "-q", "-b", "other")
		}
		before := sh(t, repo, "rev-parse", "main")
		var conflict *ConflictError
		_, err = svc.Land(ctx, f.ID, LandRequest{Merge: true})
		if !errors.As(err, &conflict) || len(conflict.Files) != 1 || conflict.Files[0] != "README.md" {
			t.Fatalf("checked out %v: %v", checkedOut, err)
		}
		if sh(t, repo, "rev-parse", "main") != before || sh(t, repo, "status", "--porcelain") != "?? .env" {
			t.Fatalf("checked out %v: the merge left traces: %s", checkedOut, sh(t, repo, "status", "--porcelain"))
		}
		if _, ok := st.Floor(f.ID); !ok || !isDir(f.Path) {
			t.Fatal("a conflict removed the floor")
		}
	}
}

func TestDiscardAndExistingBranch(t *testing.T) {
	repo, svc, st := newRepo(t)
	ctx := context.Background()
	sh(t, repo, "branch", "feature")
	if _, err := svc.Create(ctx, CreateRequest{Name: "Feature"}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "--existing") {
		t.Fatalf("existing branch without --existing: %v", err)
	}
	f, err := svc.Create(ctx, CreateRequest{Name: "Feature", Existing: true})
	if err != nil || f.NewBranch {
		t.Fatalf("open an existing branch: %+v %v", f, err)
	}
	write(t, filepath.Join(f.Path, "wip.txt"), "unsaved")
	if err := svc.Discard(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if isDir(f.Path) || sh(t, repo, "branch", "--list", "feature") == "" {
		t.Fatal("discard removed a branch the floor didn't create (or kept the folder)")
	}

	g, err := svc.Create(ctx, CreateRequest{Name: "Scratch", Branch: "tmp/scratch"})
	if err != nil || g.Branch != "tmp/scratch" {
		t.Fatalf("custom branch: %+v %v", g, err)
	}
	st.AddNode(store.Node{Kind: store.KindTerminal, Floor: g.ID, Terminal: &store.TerminalData{Name: "Caio", Agent: "shell"}})
	if err := svc.Discard(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if sh(t, repo, "branch", "--list", "tmp/scratch") != "" || len(st.Snapshot().Nodes) != 0 || len(st.Floors()) != 0 {
		t.Fatal("discard left the branch or the floor")
	}
	if _, err := svc.Create(ctx, CreateRequest{Name: "x", Branch: "-bad"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad branch: %v", err)
	}
	if _, err := svc.Create(ctx, CreateRequest{Name: "Térreo"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ground name: %v", err)
	}
}

func TestSubfolderSetupFailureAndNoRepo(t *testing.T) {
	repo, svc, st := newRepo(t)
	ctx := context.Background()
	if _, err := st.UpdateMeta(func(m *store.Meta) error { m.Cwd = filepath.Join(repo, "web"); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetFloorConfig(store.FloorConfig{Setup: "echo broken >&2; exit 3"}); err != nil {
		t.Fatal(err)
	}
	f, err := svc.Create(ctx, CreateRequest{Name: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Cwd != filepath.Join(f.Path, "web") {
		t.Fatalf("cwd %s", f.Cwd)
	}
	svc.WaitReady(ctx, f.ID, 10*time.Second)
	if f, _ = st.Floor(f.ID); f.Status != store.FloorFailed || !strings.Contains(f.Error, "broken") {
		t.Fatalf("failed setup: %+v", f)
	}
	// A setup cut short by a restart is marked failed.
	st.UpdateFloor(f.ID, func(f *store.Floor) { f.Status = store.FloorPreparing })
	svc.Start()
	if f, _ = st.Floor(f.ID); f.Status != store.FloorFailed || !strings.Contains(f.Error, "interrupted") {
		t.Fatalf("interrupted: %+v", f)
	}

	plain := t.TempDir()
	st2, _ := store.Open(filepath.Join(t.TempDir(), "w.json"), store.Workspace{ID: "p", Cwd: plain})
	svc2 := New(Options{Canvas: canvas{st2, events.NewBus()}})
	if _, err := svc2.Create(ctx, CreateRequest{Name: "x"}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "not in a git repository") {
		t.Fatalf("no repo: %v", err)
	}
}

func TestStatusAndBranches(t *testing.T) {
	repo, svc, _ := newRepo(t)
	ctx := context.Background()
	s := svc.Status(ctx)
	// The untracked .env of newRepo is the ground floor's one change.
	if !s.Repo || s.Ground.Branch != "main" || s.Ground.Changes != 1 || len(s.Floors) != 0 {
		t.Fatalf("ground: %+v", s)
	}
	f, err := svc.Create(ctx, CreateRequest{Name: "ui"})
	if err != nil {
		t.Fatal(err)
	}
	if fs := svc.Status(ctx).Floors["ui"]; fs.Branch != "ui" || fs.Changes != 0 || fs.Ahead != 0 {
		t.Fatalf("new floor (the copied .env doesn't count): %+v", fs)
	}
	write(t, filepath.Join(f.Path, "a.txt"), "a")
	write(t, filepath.Join(f.Path, "b.txt"), "b")
	if fs := svc.Status(ctx).Floors["ui"]; fs.Changes != 2 {
		t.Fatalf("two files: %+v", fs)
	}
	sh(t, f.Path, "add", "a.txt", "b.txt")
	sh(t, f.Path, "commit", "-q", "-m", "ab")
	if fs := svc.Status(ctx).Floors["ui"]; fs.Changes != 0 || fs.Ahead != 1 {
		t.Fatalf("committed: %+v", fs)
	}
	b, err := svc.Branches(ctx)
	if err != nil || strings.Join(b, ",") != "main,ui" {
		t.Fatalf("branches: %v %v", b, err)
	}
	os.RemoveAll(f.Path)
	if fs := svc.Status(ctx).Floors["ui"]; !fs.Missing {
		t.Fatalf("missing folder: %+v", fs)
	}
	_ = repo

	st2, _ := store.Open(filepath.Join(t.TempDir(), "w.json"), store.Workspace{ID: "p", Cwd: t.TempDir()})
	if s := New(Options{Canvas: canvas{st2, events.NewBus()}}).Status(ctx); s.Repo {
		t.Fatal("a plain folder is not a repository")
	}
}

package floors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// A session works in a copy made from another branch, without the work left
// uncommitted in the workspace folder; its floors come from the copy, and
// merging keeps it.
func TestSessionCopy(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	sh(t, repo, "checkout", "-q", "-b", "dev")
	write(t, filepath.Join(repo, "dev.txt"), "dev\n")
	sh(t, repo, "add", "dev.txt")
	sh(t, repo, "commit", "-q", "-m", "dev")
	sh(t, repo, "checkout", "-q", "main")
	write(t, filepath.Join(repo, "README.md"), "one\ntwo\nthree\nhalf done\n") // another session's work

	cfg := store.FloorConfig{Copy: []string{".env*"}, Setup: "echo prepared > setup.txt"}
	f, err := NewCopy(ctx, CopyRequest{Cwd: repo, Branch: "login-novo", From: "dev", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(repo), ".torre", "sessions", "app--login-novo")
	if f.Path != want || f.Cwd != want || f.Branch != "login-novo" || f.Base != "dev" || !f.NewBranch || f.Status != store.FloorPreparing ||
		len(f.Copied) != 1 || f.Copied[0] != ".env" {
		t.Fatalf("copy: %+v", f)
	}
	if b, _ := os.ReadFile(filepath.Join(f.Path, "README.md")); strings.Contains(string(b), "half done") {
		t.Fatal("the copy took the workspace folder's uncommitted work")
	}
	if _, err := os.Stat(filepath.Join(f.Path, "dev.txt")); err != nil {
		t.Fatalf("the copy didn't start from dev: %v", err)
	}
	for _, req := range []CopyRequest{{Cwd: repo, Branch: "dev"}, {Cwd: repo, Branch: "x", From: "nope"}, {Cwd: repo, Branch: "bad..name"}} {
		if _, err := NewCopy(ctx, req); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v: %v", req, err)
		}
	}

	// The session's canvas works in the copy; opening it runs the setup.
	st, err := store.Open(filepath.Join(t.TempDir(), "workspace.json"), store.Workspace{ID: "w", Name: "app", Cwd: repo, Copy: &f})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetFloorConfig(cfg); err != nil {
		t.Fatal(err)
	}
	svc := New(Options{Canvas: canvas{st, events.NewBus()}})
	t.Cleanup(svc.Close)
	if st.Workdir() != f.Cwd {
		t.Fatalf("workdir %s", st.Workdir())
	}
	svc.Start()
	if !svc.WaitReady(ctx, copyKey, 10*time.Second) {
		t.Fatal("the copy's setup never finished")
	}
	if c, _ := st.Copy(); c.Status != store.FloorReady {
		t.Fatalf("after setup: %+v", c)
	}
	if b, _ := os.ReadFile(filepath.Join(f.Path, "setup.txt")); strings.TrimSpace(string(b)) != "prepared" {
		t.Fatalf("setup output: %q", b)
	}
	if log, _ := svc.CopyLog(); !strings.Contains(log, "echo prepared") {
		t.Fatalf("setup log: %q", log)
	}
	if s := svc.Status(ctx); !s.Repo || s.Ground.Branch != "login-novo" || s.Ground.Changes != 0 || s.Ground.Ahead != 0 {
		t.Fatalf("status: %+v", s)
	}

	// Its floors come from the copy, named after it.
	fl, err := svc.Create(ctx, CreateRequest{Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	if fl.Path != filepath.Join(filepath.Dir(repo), ".torre", "floors", "app--login-novo--api") || fl.Base != "login-novo" {
		t.Fatalf("floor of the copy: %+v", fl)
	}
	if err := svc.Discard(ctx, fl.ID); err != nil {
		t.Fatal(err)
	}

	// Merging: not with uncommitted work; then into the branch it came from,
	// and the copy stays.
	write(t, filepath.Join(f.Path, "login.go"), "package login\n")
	var dirtyErr *DirtyError
	if _, err := svc.MergeCopy(ctx, LandRequest{}); !errors.As(err, &dirtyErr) || dirtyErr.Files[0] != "login.go" {
		t.Fatalf("merge with uncommitted work: %v", err)
	}
	sh(t, f.Path, "add", "login.go")
	sh(t, f.Path, "commit", "-q", "-m", "login")
	if s := svc.Status(ctx); s.Ground.Ahead != 1 {
		t.Fatalf("ahead: %+v", s.Ground)
	}
	res, err := svc.MergeCopy(ctx, LandRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Merged || res.Into != "dev" || sh(t, repo, "show", "dev:login.go") != "package login" {
		t.Fatalf("merge: %+v", res)
	}
	if _, ok := st.Copy(); !ok || !isDir(f.Path) {
		t.Fatal("merging removed the copy")
	}
	// main is checked out in the workspace folder, with another session's work.
	if _, err := svc.MergeCopy(ctx, LandRequest{Into: "main"}); !errors.As(err, &dirtyErr) || dirtyErr.Folder != repo || dirtyErr.Branch != "main" {
		t.Fatalf("merge into a dirty folder: %v", err)
	}

	// Removing it: the uncommitted work shows first; the branch stays.
	write(t, filepath.Join(f.Path, "wip.txt"), "wip\n")
	if files, err := Uncommitted(ctx, f); err != nil || len(files) != 1 || files[0] != "wip.txt" {
		t.Fatalf("uncommitted: %v %v", files, err)
	}
	if err := RemoveCopy(ctx, repo, f, false); err != nil {
		t.Fatal(err)
	}
	if isDir(f.Path) || !hasRef(ctx, repo, "refs/heads/login-novo") {
		t.Fatalf("after removing: folder %v, branch %v", isDir(f.Path), hasRef(ctx, repo, "refs/heads/login-novo"))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(repo), ".torre")); !os.IsNotExist(err) {
		t.Fatalf(".torre left behind: %v", err)
	}
}

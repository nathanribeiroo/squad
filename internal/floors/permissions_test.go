package floors

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

const (
	claudeFile = ".claude/settings.local.json"
	devinFile  = ".devin/config.local.json"
)

// rules reads a permissions file: its rules of kind, and the whole document.
func rules(t *testing.T, dir, file, kind string) ([]string, map[string]any) {
	t.Helper()
	doc, r, err := readRules(filepath.Join(dir, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	return r[kind], doc
}

// What the user allowed "always" on the ground floor goes to a floor when
// it is made and as its flights start; what the floor gains doesn't block
// landing, and landing takes it back only when asked.
func TestPermissionsFollowFloors(t *testing.T) {
	repo, svc, _ := newRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(repo, claudeFile), `{"permissions": {"allow": ["Bash(npm test *)"]}, "model": "x"}`)
	write(t, filepath.Join(repo, devinFile), `{"permissions": {"allow": ["Exec(touch)"]}}`)

	f, err := svc.Create(ctx, CreateRequest{Name: "Login"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Copied, claudeFile) || !slices.Contains(f.Copied, devinFile) {
		t.Fatalf("copied: %v", f.Copied)
	}
	if r, doc := rules(t, f.Cwd, claudeFile, "allow"); !slices.Equal(r, []string{"Bash(npm test *)"}) || doc["model"] != "x" {
		t.Fatalf("floor's claude file: %v %v", r, doc)
	}

	// A rule the ground floor gains reaches the floor when a flight starts.
	write(t, filepath.Join(repo, claudeFile), `{"permissions": {"allow": ["Bash(npm test *)", "Bash(git tag *)"]}, "model": "x"}`)
	svc.SyncRules(f.ID)
	if r, doc := rules(t, f.Cwd, claudeFile, "allow"); !slices.Equal(r, []string{"Bash(npm test *)", "Bash(git tag *)"}) || doc["model"] != "x" {
		t.Fatalf("synced: %v %v", r, doc)
	}

	// The floor gains rules: they aren't changes, and landing lists them.
	write(t, filepath.Join(f.Cwd, claudeFile), `{"permissions": {"allow": ["Bash(npm test *)", "Bash(git tag *)", "Bash(make *)"]}, "model": "x"}`)
	write(t, filepath.Join(f.Cwd, devinFile), `{"permissions": {"allow": ["Exec(touch)", "Exec(npm run build)"], "deny": ["Exec(rm)"]}}`)
	ch, err := svc.Changes(ctx, f.ID)
	if err != nil || len(ch.Files) != 0 {
		t.Fatalf("changes: %+v %v", ch, err)
	}
	want := []Rule{{claudeFile, "allow", "Bash(make *)"}, {devinFile, "allow", "Exec(npm run build)"}, {devinFile, "deny", "Exec(rm)"}}
	if !slices.Equal(ch.Rules, want) {
		t.Fatalf("rules: %+v", ch.Rules)
	}
	if s := svc.Status(ctx); len(s.Floors) != 1 || s.Floors[f.ID].Changes != 0 {
		t.Fatalf("status: %+v", s.Floors)
	}

	res, err := svc.Land(ctx, f.ID, LandRequest{Permissions: true})
	if err != nil || res.Permissions != 3 {
		t.Fatalf("land: %+v %v", res, err)
	}
	if r, doc := rules(t, repo, claudeFile, "allow"); !slices.Equal(r, []string{"Bash(npm test *)", "Bash(git tag *)", "Bash(make *)"}) || doc["model"] != "x" {
		t.Fatalf("ground's claude file: %v %v", r, doc)
	}
	if d, _ := rules(t, repo, devinFile, "deny"); !slices.Equal(d, []string{"Exec(rm)"}) {
		t.Fatalf("ground's devin deny: %v", d)
	}

	// Not asked: the ground floor keeps what it had.
	g, err := svc.Create(ctx, CreateRequest{Name: "Busca"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(g.Cwd, devinFile), `{"permissions": {"allow": ["Exec(touch)", "Exec(npm run build)", "Exec(curl)"]}}`)
	if res, err := svc.Land(ctx, g.ID, LandRequest{}); err != nil || res.Permissions != 0 {
		t.Fatalf("land without: %+v %v", res, err)
	}
	if a, _ := rules(t, repo, devinFile, "allow"); slices.Contains(a, "Exec(curl)") {
		t.Fatalf("taken unasked: %v", a)
	}

	// A floor's branch that commits them doesn't merge (like a copied .env).
	h, err := svc.Create(ctx, CreateRequest{Name: "Vazado"})
	if err != nil {
		t.Fatal(err)
	}
	sh(t, h.Path, "add", "-f", claudeFile)
	sh(t, h.Path, "commit", "-q", "-m", "oops")
	if _, err := svc.Land(ctx, h.ID, LandRequest{Merge: true}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), claudeFile) {
		t.Fatalf("leaked: %v", err)
	}
}

// A ground floor without them: a floor's agents write them, which isn't a
// change either; a file that isn't JSON offers no rules and blocks nothing.
func TestPermissionsWrittenOnTheFloor(t *testing.T) {
	_, svc, _ := newRepo(t)
	ctx := context.Background()
	f, err := svc.Create(ctx, CreateRequest{Name: "Login"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Copied) != 1 { // .env only
		t.Fatalf("copied: %v", f.Copied)
	}
	write(t, filepath.Join(f.Cwd, devinFile), `{"permissions": {"allow": ["Exec(touch)"]}}`)
	write(t, filepath.Join(f.Cwd, claudeFile), `not json`)
	ch, err := svc.Changes(ctx, f.ID)
	if err != nil || len(ch.Files) != 0 || len(ch.Rules) != 0 && ch.Rules[0].Rule != "Exec(touch)" {
		t.Fatalf("changes: %+v %v", ch, err)
	}
	if _, err := svc.Land(ctx, f.ID, LandRequest{}); err != nil {
		t.Fatalf("land: %v", err)
	}
}

// A session's copy gets them too, and merging it can take back what it
// gained, keeping the copy.
func TestPermissionsSessionCopy(t *testing.T) {
	repo, _, _ := newRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(repo, devinFile), `{"permissions": {"allow": ["Exec(touch)"]}}`)
	f, err := NewCopy(ctx, CopyRequest{Cwd: repo, Branch: "sessao"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Copied, devinFile) {
		t.Fatalf("copied: %v", f.Copied)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "workspace.json"), store.Workspace{ID: "w", Name: "app", Cwd: repo, Copy: &f})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(Options{Canvas: canvas{st, events.NewBus()}})
	t.Cleanup(svc.Close)

	// The workspace folder gains a rule: the copy's flights get it.
	write(t, filepath.Join(repo, devinFile), `{"permissions": {"allow": ["Exec(touch)", "Exec(ls)"]}}`)
	svc.SyncRules("")
	if a, _ := rules(t, f.Cwd, devinFile, "allow"); !slices.Equal(a, []string{"Exec(touch)", "Exec(ls)"}) {
		t.Fatalf("synced: %v", a)
	}

	write(t, filepath.Join(f.Cwd, claudeFile), `{"permissions": {"allow": ["Bash(make *)"]}}`)
	ch, err := svc.CopyChanges(ctx)
	if err != nil || len(ch.Files) != 0 || len(ch.Rules) != 1 || ch.Rules[0].Rule != "Bash(make *)" {
		t.Fatalf("changes: %+v %v", ch, err)
	}
	write(t, filepath.Join(f.Cwd, "novo.txt"), "x\n")
	sh(t, f.Path, "add", "novo.txt")
	sh(t, f.Path, "commit", "-q", "-m", "novo")
	res, err := svc.MergeCopy(ctx, LandRequest{Into: "main", Permissions: true})
	if err != nil || !res.Merged || res.Permissions != 1 {
		t.Fatalf("merge: %+v %v", res, err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, filepath.FromSlash(claudeFile)))
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil || !strings.Contains(string(b), "Bash(make *)") {
		t.Fatalf("workspace folder's claude file: %s", b)
	}
}

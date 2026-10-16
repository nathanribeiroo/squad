// Package floors manages a workspace's floors (docs/PLAN.md R4): isolated
// copies of its git repository, each a git worktree on its own branch with
// its own canvas, so agents can work in parallel without touching the same
// files. A floor lives outside the project, like Maestri's, in
// <parent of the repository>/.squad/floors/<repository>--<floor>.
//
// A new floor gets the tracked files of the ground floor's last commit (no
// copy-on-write clone: Windows has none), the untracked files the workspace
// lists (.env…) and then runs the workspace's setup command (npm install…).
// Landing a floor closes its terminals and removes its folder, keeping its
// branch; it can merge the branch first, without touching any folder when the
// target branch isn't checked out. It is refused while the floor has
// uncommitted changes. Discarding removes everything, the branch included.
package floors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// MaxName is the longest floor name.
const MaxName = 40

// Errors returned by the service.
var (
	ErrNotFound = errors.New("floor not found")
	ErrInvalid  = errors.New("invalid floor request")
)

type invalidError string

func (e invalidError) Error() string        { return string(e) }
func (e invalidError) Is(target error) bool { return target == ErrInvalid }

// DirtyError refuses a land: uncommitted changes on the floor, or in the
// folder where the target branch is checked out.
type DirtyError struct {
	Where string   // "the floor" or a folder
	Files []string // first ones
	More  int      // how many more
}

func (e *DirtyError) Error() string {
	list := strings.Join(e.Files, ", ")
	if e.More > 0 {
		list += fmt.Sprintf(" and %d more", e.More)
	}
	return fmt.Sprintf("%s has uncommitted changes (commit or discard them first): %s", e.Where, list)
}

// ConflictError refuses a merge; nothing was changed.
type ConflictError struct {
	Branch, Into string
	Files        []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("merging %s into %s conflicts in: %s. Nothing was changed: merge %s into %s on the floor, resolve and commit there, then land again.",
		e.Branch, e.Into, strings.Join(e.Files, ", "), e.Into, e.Branch)
}

// Canvas is the workspace the floors belong to.
type Canvas interface {
	Store() *store.Store
	RemoveNode(id string) error
	Bus() *events.Bus
}

// Options configure the service; zero values take the defaults.
type Options struct {
	Canvas Canvas
	Logger *slog.Logger
	// IsManager tells which terminals may create and land floors.
	IsManager func(store.Node) bool
	// SetupTimeout stops a setup command (30 min); ReadyWait is how long
	// squad floor create waits for it (45 s, within the CLI's 60 s).
	SetupTimeout time.Duration
	ReadyWait    time.Duration
}

// Service runs the floors of one workspace.
type Service struct {
	o   Options
	log *slog.Logger

	mu sync.Mutex // one create, land or discard at a time

	smu    sync.Mutex
	setups map[string]*setupRun
	wg     sync.WaitGroup
}

type setupRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// New returns the service; call Start before serving.
func New(o Options) *Service {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.IsManager == nil {
		o.IsManager = func(store.Node) bool { return false }
	}
	if o.SetupTimeout == 0 {
		o.SetupTimeout = 30 * time.Minute
	}
	if o.ReadyWait == 0 {
		o.ReadyWait = 45 * time.Second
	}
	return &Service{o: o, log: o.Logger, setups: map[string]*setupRun{}}
}

// Start marks setups cut short by a previous run as failed.
func (s *Service) Start() {
	st := s.o.Canvas.Store()
	for _, f := range st.Floors() {
		if f.Status == store.FloorPreparing {
			_, _ = st.UpdateFloor(f.ID, func(f *store.Floor) {
				f.Status, f.Error = store.FloorFailed, "the setup command was interrupted (Squad stopped)"
			})
		}
	}
}

// Close stops the running setup commands.
func (s *Service) Close() {
	s.smu.Lock()
	for _, r := range s.setups {
		r.cancel()
	}
	s.smu.Unlock()
	s.wg.Wait()
}

// View is a floor as the UI and the CLI show it.
type View struct {
	store.Floor
	Terminals int  `json:"terminals"`
	Missing   bool `json:"missing,omitempty"` // its folder is gone
}

// State is the floors of the workspace and how new ones are seeded (GET
// /api/floors and the floors.updated event).
type State struct {
	Floors []View            `json:"floors"`
	Config store.FloorConfig `json:"config"`
}

// State returns the floors and their config.
func (s *Service) State() State {
	st := s.o.Canvas.Store()
	nodes := st.Snapshot().Nodes
	out := State{Floors: []View{}, Config: st.FloorConfig()}
	for _, f := range st.Floors() {
		v := View{Floor: f, Missing: !isDir(f.Path)}
		for _, n := range nodes {
			if n.Floor == f.ID && n.Terminal != nil {
				v.Terminals++
			}
		}
		out.Floors = append(out.Floors, v)
	}
	return out
}

func (s *Service) publish() { s.o.Canvas.Bus().Publish(events.FloorsUpdated, s.State()) }

// SetConfig changes how new floors are seeded.
func (s *Service) SetConfig(c store.FloorConfig) (store.FloorConfig, error) {
	for _, p := range c.Copy {
		if p = filepath.Clean(filepath.FromSlash(strings.TrimSpace(p))); filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
			return store.FloorConfig{}, invalidError(fmt.Sprintf("copy patterns are relative to the workspace folder: %q", p))
		}
	}
	c, err := s.o.Canvas.Store().SetFloorConfig(c)
	if err == nil {
		s.publish()
	}
	return c, err
}

// CreateRequest asks for a new floor.
type CreateRequest struct {
	Name     string `json:"name"`
	Branch   string `json:"branch,omitempty"`   // default: the slug of the name
	Existing bool   `json:"existing,omitempty"` // open an existing branch instead of creating it
	By       string `json:"-"`                  // the terminal whose agent asked
}

// Create makes a floor from the ground floor's last commit. The setup
// command, if any, runs in the background (the floor is "preparing").
func (s *Service) Create(ctx context.Context, req CreateRequest) (store.Floor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := strings.TrimSpace(req.Name)
	switch {
	case name == "":
		return store.Floor{}, invalidError("a floor needs a name")
	case utf8.RuneCountInString(name) > MaxName:
		return store.Floor{}, invalidError(fmt.Sprintf("floor names have at most %d characters", MaxName))
	case store.IsGround(name):
		return store.Floor{}, invalidError(fmt.Sprintf("%q is the ground floor; pick another name", name))
	}
	id := ids.Slug(name)
	if id == "" {
		return store.Floor{}, invalidError("a floor name needs a letter or a digit")
	}
	st := s.o.Canvas.Store()
	for _, f := range st.Floors() {
		if f.ID == id || strings.EqualFold(f.Name, name) {
			return store.Floor{}, invalidError(fmt.Sprintf("there is already a floor named %q", f.Name))
		}
	}
	cwd := st.Meta().Cwd
	root, err := repoRoot(ctx, cwd)
	if err != nil {
		return store.Floor{}, err
	}
	parent := filepath.Dir(root)
	if parent == root {
		return store.Floor{}, invalidError("floors live next to the repository, which can't be the root of a drive")
	}
	if !hasRef(ctx, root, "HEAD") {
		return store.Floor{}, invalidError("the repository has no commit yet: a floor starts from the last commit")
	}
	baseCommit, err := git(ctx, cwd, "rev-parse", "HEAD")
	if err != nil {
		return store.Floor{}, err
	}
	base, err := currentBranch(ctx, cwd)
	if err != nil {
		return store.Floor{}, err
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = id
	}
	if err := checkBranchName(ctx, root, branch); err != nil {
		return store.Floor{}, err
	}
	exists := hasRef(ctx, root, "refs/heads/"+branch)
	switch {
	case exists && !req.Existing:
		return store.Floor{}, invalidError(fmt.Sprintf("a branch named %q already exists: open it on a floor with --existing, or pick another name", branch))
	case !exists && req.Existing:
		return store.Floor{}, invalidError(fmt.Sprintf("there is no branch named %q", branch))
	}
	path := filepath.Join(parent, ".squad", "floors", filepath.Base(root)+"--"+id)
	if _, err := os.Stat(path); err == nil {
		return store.Floor{}, invalidError(fmt.Sprintf("the folder %s already exists (left by an older floor?): remove it or pick another name", path))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return store.Floor{}, err
	}
	args := []string{"worktree", "add", "--quiet"}
	if exists {
		args = append(args, path, branch)
	} else {
		args = append(args, "-b", branch, path, baseCommit)
	}
	if _, err := git(ctx, root, args...); err != nil {
		removeEmptyParents(path)
		return store.Floor{}, err
	}

	floorCwd := filepath.Join(path, relInside(root, cwd))
	if !isDir(floorCwd) { // a folder git doesn't track (empty, or ignored)
		floorCwd = path
	}
	cfg := st.FloorConfig()
	copied, copyErr := copyUntracked(cwd, floorCwd, path, cfg.Copy)
	f := store.Floor{
		ID: id, Name: name, Branch: branch, Base: base, BaseCommit: baseCommit,
		Path: path, Cwd: floorCwd, NewBranch: !exists, Copied: copied,
		Status: store.FloorReady, CreatedBy: req.By, CreatedAt: time.Now().UTC(),
	}
	if copyErr != nil {
		s.log.Warn("floor: copy", "floor", name, "err", copyErr)
		f.Error = "some files were not copied: " + copyErr.Error()
	}
	if cfg.Setup != "" {
		f.Status = store.FloorPreparing
	}
	if f, err = st.AddFloor(f); err != nil {
		s.removeWorktree(ctx, root, path)
		if !exists {
			_, _ = git(ctx, root, "branch", "-D", branch)
		}
		return store.Floor{}, err
	}
	if cfg.Setup != "" {
		s.startSetup(f, cfg.Setup)
	}
	s.publish()
	return f, nil
}

// Changes is what a floor has that its base doesn't, for the land dialog.
type Changes struct {
	Files   []change `json:"files"`             // uncommitted (copied files left out)
	Ahead   int      `json:"ahead"`             // commits on the floor's branch not on its base
	Behind  int      `json:"behind"`            // commits on the base not on the floor's branch
	Commits []Commit `json:"commits"`           // the floor's commits, newest first (20 at most)
	Missing bool     `json:"missing,omitempty"` // the floor's folder is gone
}

// Commit is one line of git log.
type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// Changes reports a floor's uncommitted changes and its commits.
func (s *Service) Changes(ctx context.Context, id string) (Changes, error) {
	f, ok := s.o.Canvas.Store().Floor(id)
	if !ok {
		return Changes{}, ErrNotFound
	}
	out := Changes{Files: []change{}, Commits: []Commit{}}
	if !isDir(f.Path) {
		out.Missing = true
		return out, nil
	}
	files, err := status(ctx, f.Path, false)
	if err != nil {
		return Changes{}, err
	}
	out.Files = append(out.Files, withoutCopied(files, f.Copied)...)
	base := f.BaseCommit
	if f.Base != "" && hasRef(ctx, f.Path, "refs/heads/"+f.Base) {
		base = "refs/heads/" + f.Base
	}
	head := "refs/heads/" + f.Branch
	out.Ahead = count(ctx, f.Path, base+".."+head)
	out.Behind = count(ctx, f.Path, head+".."+base)
	log, err := git(ctx, f.Path, "log", "--format=%h%x00%s", "-n", "20", base+".."+head)
	if err == nil {
		for _, line := range strings.Split(log, "\n") {
			if h, subj, ok := strings.Cut(line, "\x00"); ok {
				out.Commits = append(out.Commits, Commit{Hash: h, Subject: subj})
			}
		}
	}
	return out, nil
}

// LandRequest says whether landing merges the floor's branch, and where.
type LandRequest struct {
	Merge bool   `json:"merge,omitempty"` // into Into, or the branch the floor started from
	Into  string `json:"into,omitempty"`  // implies Merge
}

// LandResult tells what landing did.
type LandResult struct {
	Floor    string `json:"floor"`
	Branch   string `json:"branch"`
	Merged   bool   `json:"merged"`
	Into     string `json:"into,omitempty"`
	Commit   string `json:"commit,omitempty"`   // the merge commit
	UpToDate bool   `json:"upToDate,omitempty"` // the target already had every commit
}

// Land removes a floor (its terminals and its folder), keeping its branch,
// after merging the branch if asked. It is refused while the floor has
// uncommitted changes, and a merge with conflicts changes nothing.
func (s *Service) Land(ctx context.Context, id string, req LandRequest) (LandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.o.Canvas.Store()
	f, ok := st.Floor(id)
	if !ok {
		return LandResult{}, ErrNotFound
	}
	if s.preparing(f.ID) {
		return LandResult{}, invalidError("the floor's setup command is still running; wait for it to finish (or discard the floor)")
	}
	root, err := repoRoot(ctx, st.Meta().Cwd)
	if err != nil {
		return LandResult{}, err
	}
	if isDir(f.Path) {
		if op := operationInProgress(ctx, f.Path); op != "" {
			return LandResult{}, invalidError(fmt.Sprintf("the floor has %s in progress: finish or abort it first", op))
		}
		if br, err := currentBranch(ctx, f.Path); err != nil {
			return LandResult{}, err
		} else if br != f.Branch {
			return LandResult{}, invalidError(fmt.Sprintf("the floor's folder is not on its branch %s: switch back before landing", f.Branch))
		}
		files, err := status(ctx, f.Path, false)
		if err != nil {
			return LandResult{}, err
		}
		if files = withoutCopied(files, f.Copied); len(files) > 0 {
			return LandResult{}, dirty("the floor", files)
		}
	}
	res := LandResult{Floor: f.Name, Branch: f.Branch}
	if req.Merge || strings.TrimSpace(req.Into) != "" {
		into := strings.TrimSpace(req.Into)
		if into == "" {
			into = f.Base
		}
		if into == "" {
			return LandResult{}, invalidError("the floor started from a detached HEAD: say which branch to merge into")
		}
		if leaked := committedCopies(ctx, root, f); len(leaked) > 0 {
			return LandResult{}, invalidError(fmt.Sprintf("the floor's branch commits files copied from the ground floor, which may hold secrets: %s. Remove them from the branch on the floor (git rm --cached, then commit) before merging", strings.Join(leaked, ", ")))
		}
		commit, upToDate, err := mergeInto(ctx, root, f, into)
		if err != nil {
			return LandResult{}, err
		}
		res.Merged, res.UpToDate, res.Into, res.Commit = !upToDate, upToDate, into, commit
	}
	if err := s.remove(ctx, root, f, false); err != nil {
		if res.Merged {
			return res, fmt.Errorf("merged into %s, but removing the floor failed: %w", res.Into, err)
		}
		return res, err
	}
	return res, nil
}

// Discard removes a floor with everything in it: its terminals, its folder
// (uncommitted work included) and the branch it created.
func (s *Service) Discard(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.o.Canvas.Store()
	f, ok := st.Floor(id)
	if !ok {
		return ErrNotFound
	}
	root, err := repoRoot(ctx, st.Meta().Cwd)
	if err != nil {
		return err
	}
	return s.remove(ctx, root, f, true)
}

func dirty(where string, files []change) *DirtyError {
	e := &DirtyError{Where: where}
	for i, c := range files {
		if i == 10 {
			e.More = len(files) - i
			break
		}
		e.Files = append(e.Files, c.Path)
	}
	return e
}

// committedCopies lists the files copied from the ground floor (untracked
// there: .env…) that the floor's branch has committed.
func committedCopies(ctx context.Context, root string, f store.Floor) []string {
	if len(f.Copied) == 0 {
		return nil
	}
	args := append([]string{"ls-tree", "-r", "--name-only", "-z", "refs/heads/" + f.Branch, "--"}, f.Copied...)
	out, err := git(ctx, root, args...)
	if err != nil {
		return nil
	}
	var files []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			files = append(files, p)
		}
	}
	return files
}

// mergeInto merges the floor's branch into another one: in the folder where
// that branch is checked out, or without any folder (merge-tree).
func mergeInto(ctx context.Context, root string, f store.Floor, into string) (commit string, upToDate bool, err error) {
	if into == f.Branch {
		return "", false, invalidError("a floor can't merge its branch into itself")
	}
	if err := checkBranchName(ctx, root, into); err != nil {
		return "", false, err
	}
	if !hasRef(ctx, root, "refs/heads/"+into) {
		return "", false, invalidError(fmt.Sprintf("there is no branch %q to merge into", into))
	}
	from := "refs/heads/" + f.Branch
	if count(ctx, root, "refs/heads/"+into+".."+from) == 0 {
		return "", true, nil
	}
	msg := fmt.Sprintf("Merge branch '%s' into %s", f.Branch, into)
	wt, err := worktreeOf(ctx, root, into)
	if err != nil {
		return "", false, err
	}
	if wt != "" {
		if op := operationInProgress(ctx, wt); op != "" {
			return "", false, invalidError(fmt.Sprintf("%s has %s in progress: finish or abort it first", wt, op))
		}
		files, err := status(ctx, wt, true)
		if err != nil {
			return "", false, err
		}
		if len(files) > 0 {
			return "", false, dirty(wt+" (branch "+into+")", files)
		}
		if _, err := git(ctx, wt, "merge", "--no-ff", "--no-edit", "-m", msg, from); err != nil {
			files := unmerged(ctx, wt)
			if operationInProgress(ctx, wt) == "a merge" {
				_, _ = git(ctx, wt, "merge", "--abort")
			}
			if len(files) > 0 {
				return "", false, &ConflictError{Branch: f.Branch, Into: into, Files: files}
			}
			return "", false, err
		}
		head, err := git(ctx, wt, "rev-parse", "HEAD")
		return head, false, err
	}
	// Nobody has the target checked out: merge the trees and move the branch.
	r, err := run(ctx, root, []int{1}, "merge-tree", "--write-tree", "--name-only", "--no-messages", "refs/heads/"+into, from)
	if err != nil {
		return "", false, fmt.Errorf("%w (merging into a branch that isn't checked out needs git 2.38 or newer: check %s out in the workspace folder and land again)", err, into)
	}
	lines := strings.Split(strings.TrimRight(r.out, "\n"), "\n")
	if r.code == 1 {
		var files []string
		for _, l := range lines[1:] {
			if l == "" {
				break
			}
			files = append(files, l)
		}
		return "", false, &ConflictError{Branch: f.Branch, Into: into, Files: files}
	}
	old, err := git(ctx, root, "rev-parse", "refs/heads/"+into)
	if err != nil {
		return "", false, err
	}
	commit, err = git(ctx, root, "commit-tree", lines[0], "-p", old, "-p", from, "-m", msg)
	if err != nil {
		return "", false, err
	}
	if _, err := git(ctx, root, "update-ref", "-m", "squad: land floor "+f.Name, "refs/heads/"+into, commit, old); err != nil {
		return "", false, err
	}
	return commit, false, nil
}

// remove closes a floor's terminals and portals (its notes go down to the
// ground floor), removes its folder and forgets it.
func (s *Service) remove(ctx context.Context, root string, f store.Floor, deleteBranch bool) error {
	s.stopSetup(f.ID)
	st := s.o.Canvas.Store()
	nodes := st.Snapshot().Nodes
	ground := store.OnFloor(nodes, "")
	for _, n := range store.OnFloor(nodes, f.ID) {
		if n.Kind == store.KindNote {
			x, y := store.FreeSpot(ground, n, n.W, n.H)
			m, err := st.MoveNode(n.ID, "", x, y)
			if err != nil {
				return err
			}
			ground = append(ground, m)
			s.o.Canvas.Bus().Publish(events.NodeUpdated, m)
			continue
		}
		if err := s.o.Canvas.RemoveNode(n.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	if err := s.removeWorktree(ctx, root, f.Path); err != nil {
		return err
	}
	if deleteBranch && f.NewBranch {
		if _, err := git(ctx, root, "branch", "-D", f.Branch); err != nil {
			s.log.Warn("floor: delete branch", "branch", f.Branch, "err", err)
		}
	}
	_ = os.Remove(s.logPath(f.ID))
	if err := st.RemoveFloor(f.ID); err != nil {
		return err
	}
	s.publish()
	return nil
}

// removeWorktree removes a floor's folder and git's record of it. Its
// terminals were just stopped: on Windows their processes may hold the
// folder for a moment.
func (s *Service) removeWorktree(ctx context.Context, root, path string) error {
	var err error
	for i := 0; i < 10 && isDir(path); i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		_, err = git(ctx, root, "worktree", "remove", "--force", path)
	}
	if isDir(path) {
		if rerr := os.RemoveAll(path); rerr != nil {
			if err == nil {
				err = rerr
			}
			return fmt.Errorf("could not remove the floor's folder %s: %w", path, err)
		}
	}
	_, _ = git(ctx, root, "worktree", "prune")
	removeEmptyParents(path)
	return nil
}

// removeEmptyParents removes .squad/floors and .squad next to the
// repository when nothing else is in them.
func removeEmptyParents(path string) {
	floors := filepath.Dir(path)
	_ = os.Remove(floors)
	_ = os.Remove(filepath.Dir(floors))
}

// relInside returns dir relative to root ("." when dir isn't inside it).
func relInside(root, dir string) string {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "."
	}
	return rel
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// copyUntracked copies what the patterns match in the ground floor's folder
// to the floor's, skipping what the checkout already has (tracked files).
// It returns the copied paths relative to the floor's worktree, with "/".
func copyUntracked(from, to, worktree string, patterns []string) ([]string, error) {
	var copied []string
	var errs []error
	for _, p := range patterns {
		p = filepath.Clean(filepath.FromSlash(strings.TrimSpace(p)))
		if p == "." || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(from, p))
		if err != nil {
			errs = append(errs, fmt.Errorf("pattern %q: %w", p, err))
			continue
		}
		for _, m := range matches {
			rel, err := filepath.Rel(from, m)
			if err != nil || isGitPath(rel) {
				continue
			}
			dst := filepath.Join(to, rel)
			if _, err := os.Lstat(dst); err == nil {
				continue // tracked: the checkout has it
			}
			if err := copyTree(m, dst); err != nil {
				errs = append(errs, err)
				continue
			}
			if r, err := filepath.Rel(worktree, dst); err == nil {
				copied = append(copied, filepath.ToSlash(r))
			}
		}
	}
	return copied, errors.Join(errs...)
}

func isGitPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".git" {
			return true
		}
	}
	return false
}

// copyTree copies a file or a folder; symbolic links are skipped.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return nil
		case d.IsDir():
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		case !d.Type().IsRegular():
			return nil
		}
		if _, err := os.Lstat(target); err == nil {
			return nil
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// withoutCopied drops the files copied from the ground floor.
func withoutCopied(files []change, copied []string) []change {
	var out []change
	for _, c := range files {
		skip := false
		for _, p := range copied {
			if c.Path == p || strings.HasPrefix(c.Path, p+"/") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, c)
		}
	}
	return out
}

// --- setup command ---

func (s *Service) logPath(id string) string {
	return filepath.Join(filepath.Dir(s.o.Canvas.Store().Path()), "floors", id+".log")
}

// SetupLog returns the end of a floor's setup output.
func (s *Service) SetupLog(id string) (string, error) {
	b, err := os.ReadFile(s.logPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if len(b) > 64<<10 {
		b = b[len(b)-64<<10:]
	}
	return string(b), err
}

func (s *Service) preparing(id string) bool {
	s.smu.Lock()
	defer s.smu.Unlock()
	return s.setups[id] != nil
}

// WaitReady waits up to d for a floor's setup command; it reports whether
// the floor is no longer preparing.
func (s *Service) WaitReady(ctx context.Context, id string, d time.Duration) bool {
	s.smu.Lock()
	r := s.setups[id]
	s.smu.Unlock()
	if r == nil {
		return true
	}
	select {
	case <-r.done:
		return true
	case <-ctx.Done():
	case <-time.After(d):
	}
	return false
}

func (s *Service) stopSetup(id string) {
	s.smu.Lock()
	r := s.setups[id]
	s.smu.Unlock()
	if r != nil {
		r.cancel()
		<-r.done
	}
}

func (s *Service) startSetup(f store.Floor, cmdline string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.o.SetupTimeout)
	r := &setupRun{cancel: cancel, done: make(chan struct{})}
	s.smu.Lock()
	s.setups[f.ID] = r
	s.smu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := runSetup(ctx, f, cmdline, s.logPath(f.ID), s.o.SetupTimeout)
		cancelled := errors.Is(ctx.Err(), context.Canceled)
		cancel()
		if !cancelled { // a discarded floor (or Squad stopping) leaves it be
			_, uerr := s.o.Canvas.Store().UpdateFloor(f.ID, func(f *store.Floor) {
				f.Status, f.Error = store.FloorReady, ""
				if err != nil {
					f.Status, f.Error = store.FloorFailed, err.Error()
				}
			})
			if uerr == nil {
				s.publish()
			}
		}
		s.smu.Lock()
		delete(s.setups, f.ID)
		s.smu.Unlock()
		close(r.done)
	}()
}

// runSetup runs the setup command in the floor's folder with the user's
// shell (Git Bash on Windows), writing its output to logPath.
func runSetup(ctx context.Context, f store.Floor, cmdline, logPath string, timeout time.Duration) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	logf, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logf.Close()
	fmt.Fprintf(logf, "$ %s\n(in %s)\n\n", cmdline, f.Cwd)
	tail := &tailBuffer{max: 1500}
	out := io.MultiWriter(logf, tail)
	var startErr error
	for _, sh := range terminal.ShellCandidates("") {
		argv := commandArgv(sh, cmdline)
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = f.Cwd
		env := map[string]string{"SQUAD_FLOOR": f.Name}
		for k, v := range sh.Env {
			env[k] = v
		}
		cmd.Env = terminal.BuildEnv(os.Environ(), env, "")
		cmd.Stdout, cmd.Stderr = out, out
		cmd.WaitDelay = 5 * time.Second // children holding the output don't block Wait
		prepareTree(cmd)
		if err := cmd.Start(); err != nil {
			startErr = err
			continue // e.g. PowerShell blocked by policy: try the next shell
		}
		tree := watchTree(cmd)
		defer tree.release()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-ctx.Done():
			killTree(cmd, tree)
			<-done
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("the setup command ran for more than %s and was stopped:\n%s", timeout, tail)
			}
			return ctx.Err()
		}
		if err != nil {
			return fmt.Errorf("the setup command failed (%v):\n%s", err, tail)
		}
		return nil
	}
	return fmt.Errorf("no shell could run the setup command: %v", startErr)
}

// commandArgv runs a command line with a shell, as a login shell where that
// sets up PATH (nvm, Homebrew…).
func commandArgv(sh terminal.Shell, cmdline string) []string {
	p := sh.Argv[0]
	switch sh.Family {
	case terminal.ShellPowerShell:
		return []string{p, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", cmdline}
	case terminal.ShellCmd:
		return []string{p, "/d", "/c", cmdline}
	}
	return []string{p, "-l", "-c", cmdline}
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(strings.ToValidUTF8(string(t.b), ""))
}

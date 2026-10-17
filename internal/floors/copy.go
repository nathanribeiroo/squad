package floors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/store"
)

// A session's copy (docs/PLAN.md S4): a session may work in a copy of the
// repository instead of the workspace folder, so it doesn't touch what
// another session left uncommitted there. It is made like a floor (a git
// worktree on a new branch, the untracked files the workspace lists, the
// setup command), from the last commit of a branch the user picks, and lives
// next to the repository in .squad/sessions/<repository>--<branch>. Merging
// it keeps it: the session goes on working there and can merge again.
// Removing the session removes the copy and keeps the branch.

// copyKey names the copy's setup command among the floors' (floor ids are
// slugs, which can't start with "@").
const copyKey = "@copy"

// CopyRequest asks for a session's copy.
type CopyRequest struct {
	Cwd    string            // the workspace folder
	Branch string            // the copy's new branch
	From   string            // the branch it starts from ("": the one checked out in Cwd)
	Config store.FloorConfig // the untracked files to copy and the setup command
}

// NewCopy makes a session's copy. With a setup command, the copy is
// "preparing" until the session's floors service runs it (Start).
func NewCopy(ctx context.Context, req CopyRequest) (store.Floor, error) {
	root, err := repoRoot(ctx, req.Cwd)
	if err != nil {
		return store.Floor{}, err
	}
	main := mainRoot(ctx, root)
	parent := filepath.Dir(main)
	if parent == main {
		return store.Floor{}, invalidError("a session's copy lives next to the repository, which can't be the root of a drive")
	}
	from, start := strings.TrimSpace(req.From), "HEAD"
	if from == "" {
		if from, err = currentBranch(ctx, req.Cwd); err != nil {
			return store.Floor{}, err
		}
	}
	if from != "" {
		if err := checkBranchName(ctx, root, from); err != nil {
			return store.Floor{}, err
		}
		start = "refs/heads/" + from
	}
	if !hasRef(ctx, root, start) {
		if from == "" {
			return store.Floor{}, invalidError("the repository has no commit yet: a copy starts from the last commit")
		}
		return store.Floor{}, invalidError(fmt.Sprintf("there is no branch %q to start from", from))
	}
	baseCommit, err := git(ctx, root, "rev-parse", start)
	if err != nil {
		return store.Floor{}, err
	}
	branch := strings.TrimSpace(req.Branch)
	if err := checkBranchName(ctx, root, branch); err != nil {
		return store.Floor{}, err
	}
	if hasRef(ctx, root, "refs/heads/"+branch) {
		return store.Floor{}, invalidError(fmt.Sprintf("a branch named %q already exists: pick another name", branch))
	}
	id := ids.Slug(branch)
	if id == "" {
		return store.Floor{}, invalidError("a branch name needs a letter or a digit")
	}
	path := filepath.Join(parent, ".squad", "sessions", filepath.Base(main)+"--"+id)
	if _, err := os.Stat(path); err == nil {
		return store.Floor{}, invalidError(fmt.Sprintf("the folder %s already exists (left by an older session?): remove it or pick another branch name", path))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return store.Floor{}, err
	}
	if _, err := git(ctx, root, "worktree", "add", "--quiet", "-b", branch, path, baseCommit); err != nil {
		removeEmptyParents(path)
		return store.Floor{}, err
	}
	cwd := filepath.Join(path, relInside(root, req.Cwd))
	if !isDir(cwd) { // a folder git doesn't track (empty, or ignored)
		cwd = path
	}
	copied, copyErr := copyUntracked(req.Cwd, cwd, path, req.Config.Copy)
	f := store.Floor{
		ID: "copy", Name: branch, Branch: branch, Base: from, BaseCommit: baseCommit,
		Path: path, Cwd: cwd, NewBranch: true, Copied: copied,
		Status: store.FloorReady, CreatedAt: time.Now().UTC(),
	}
	if copyErr != nil {
		f.Error = "some files were not copied: " + copyErr.Error()
	}
	if req.Config.Setup != "" {
		f.Status = store.FloorPreparing
	}
	return f, nil
}

// RemoveCopy removes a session's copy: its folder (uncommitted work in it
// included: see Uncommitted) and git's record of it. Its branch stays, with
// what was committed there, unless deleteBranch (a copy whose session never
// opened). cwd is the workspace folder.
func RemoveCopy(ctx context.Context, cwd string, f store.Floor, deleteBranch bool) error {
	root, err := repoRoot(ctx, cwd)
	if err != nil {
		return err
	}
	if err := removeWorktree(ctx, root, f.Path); err != nil {
		return err
	}
	if deleteBranch && f.NewBranch {
		if _, err := git(ctx, root, "branch", "-D", f.Branch); err != nil {
			return err
		}
	}
	return nil
}

// Uncommitted lists what a session's copy has that isn't committed (the
// files copied into it left out): what removing it would lose.
func Uncommitted(ctx context.Context, f store.Floor) ([]string, error) {
	if !isDir(f.Path) {
		return nil, nil
	}
	files, err := status(ctx, f.Path, untrackedAll)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range withoutCopied(files, f.Copied) {
		out = append(out, c.Path)
	}
	return out, nil
}

// prepareCopy runs the setup command of the session's copy while it is
// "preparing": just made, or interrupted when the session closed.
func (s *Service) prepareCopy() {
	st := s.o.Canvas.Store()
	c, ok := st.Copy()
	if !ok || c.Status != store.FloorPreparing {
		return
	}
	setup := st.FloorConfig().Setup
	if setup == "" { // the workspace dropped its setup command meanwhile
		_, _ = st.UpdateCopy(func(f *store.Floor) { f.Status = store.FloorReady })
		return
	}
	s.startSetup(copyKey, c, setup, s.copyLogPath(), func(fn func(*store.Floor)) error {
		_, err := st.UpdateCopy(fn)
		return err
	})
}

func (s *Service) copyLogPath() string {
	return filepath.Join(filepath.Dir(s.o.Canvas.Store().Path()), "copy.log")
}

// CopyLog returns the end of the copy's setup output.
func (s *Service) CopyLog() (string, error) {
	b, err := os.ReadFile(s.copyLogPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if len(b) > 64<<10 {
		b = b[len(b)-64<<10:]
	}
	return string(b), err
}

// errNoCopy refuses copy requests in a session that works in the workspace
// folder.
var errNoCopy = invalidError("this session works in the workspace folder: it has no copy (nor a branch of its own) to merge")

// CopyChanges reports what the session's copy has that its base doesn't.
func (s *Service) CopyChanges(ctx context.Context) (Changes, error) {
	c, ok := s.o.Canvas.Store().Copy()
	if !ok {
		return Changes{}, errNoCopy
	}
	return changes(ctx, c)
}

// MergeCopy merges the session's branch into another one (by default the
// branch the copy started from) and keeps the copy. It is refused while the
// copy has uncommitted changes, and a merge with conflicts changes nothing.
func (s *Service) MergeCopy(ctx context.Context, req LandRequest) (LandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.o.Canvas.Store().Copy()
	if !ok {
		return LandResult{}, errNoCopy
	}
	if s.preparing(copyKey) {
		return LandResult{}, invalidError("the copy's setup command is still running; wait for it to finish")
	}
	if !isDir(f.Path) {
		return LandResult{}, invalidError(fmt.Sprintf("the session's copy is gone (%s)", f.Path))
	}
	root, err := repoRoot(ctx, f.Path)
	if err != nil {
		return LandResult{}, err
	}
	if op := operationInProgress(ctx, f.Path); op != "" {
		return LandResult{}, invalidError(fmt.Sprintf("the session's copy has %s in progress: finish or abort it first", op))
	}
	if br, err := currentBranch(ctx, f.Path); err != nil {
		return LandResult{}, err
	} else if br != f.Branch {
		return LandResult{}, invalidError(fmt.Sprintf("the session's copy is not on its branch %s: switch back before merging", f.Branch))
	}
	files, err := status(ctx, f.Path, untrackedAll)
	if err != nil {
		return LandResult{}, err
	}
	if files = withoutCopied(files, f.Copied); len(files) > 0 {
		return LandResult{}, dirty("the session's copy", files)
	}
	into := strings.TrimSpace(req.Into)
	if into == "" {
		into = f.Base
	}
	if into == "" {
		return LandResult{}, invalidError("the copy started from a detached HEAD: say which branch to merge into")
	}
	if leaked := committedCopies(ctx, root, f); len(leaked) > 0 {
		return LandResult{}, invalidError(fmt.Sprintf("the session's branch commits files copied from the workspace folder, which may hold secrets: %s. Remove them from the branch in the copy (git rm --cached, then commit) before merging", strings.Join(leaked, ", ")))
	}
	commit, upToDate, err := mergeInto(ctx, root, f, into)
	var conflict *ConflictError
	if errors.As(err, &conflict) {
		conflict.Copy = true
	}
	if err != nil {
		return LandResult{}, err
	}
	s.publish()
	return LandResult{Floor: f.Name, Branch: f.Branch, Merged: !upToDate, UpToDate: upToDate, Into: into, Commit: commit}, nil
}

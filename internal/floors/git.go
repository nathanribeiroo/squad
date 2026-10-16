package floors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// gitTimeout bounds one git command (a checkout of a large repository).
const gitTimeout = 5 * time.Minute

// gitResult is a finished git command.
type gitResult struct {
	out  string // stdout
	code int    // exit code (-1: didn't run)
}

// gitEnv is the environment of git commands: never prompting, and without
// variables that would point git at another repository.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
}

// run runs git in dir. A non-zero exit is an error carrying git's message,
// except for the codes in ok, which come back in the result.
func run(ctx context.Context, dir string, ok []int, args ...string) (gitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res := gitResult{out: stdout.String(), code: -1}
	var exit *exec.ExitError
	switch {
	case err == nil:
		res.code = 0
		return res, nil
	case errors.As(err, &exit):
		res.code = exit.ExitCode()
		for _, c := range ok {
			if c == res.code {
				return res, nil
			}
		}
	case errors.Is(err, exec.ErrNotFound):
		return res, errors.New("git is not installed (or not on PATH)")
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = strings.TrimSpace(stdout.String())
	}
	if msg == "" {
		msg = err.Error()
	}
	return res, fmt.Errorf("git %s: %s", args[0], msg)
}

// git runs a git command and returns its trimmed output.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	r, err := run(ctx, dir, nil, args...)
	return strings.TrimSpace(r.out), err
}

// repoRoot returns the top folder of the repository holding dir.
func repoRoot(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return "", invalidError("the workspace folder is not in a git repository (floors are copies of one)")
		}
		return "", err
	}
	return filepath.Clean(filepath.FromSlash(out)), nil
}

// hasRef reports whether a ref (refs/heads/x, a commit…) exists.
func hasRef(ctx context.Context, dir, ref string) bool {
	r, err := run(ctx, dir, []int{1}, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil && r.code == 0
}

// currentBranch returns the branch checked out in dir ("" when detached).
func currentBranch(ctx context.Context, dir string) (string, error) {
	r, err := run(ctx, dir, []int{1}, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(r.out), nil
}

// checkBranchName validates a branch name the way git does.
func checkBranchName(ctx context.Context, dir, name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return invalidError(fmt.Sprintf("invalid branch name %q", name))
	}
	if _, err := git(ctx, dir, "check-ref-format", "--branch", name); err != nil {
		return invalidError(fmt.Sprintf("invalid branch name %q", name))
	}
	return nil
}

// change is one line of git status.
type change struct {
	Path   string `json:"path"`   // relative to the worktree, with "/"
	Status string `json:"status"` // the two porcelain letters: " M", "??", "A "…
}

// status lists uncommitted changes in a worktree (untracked files included
// unless tracked is set).
func status(ctx context.Context, dir string, trackedOnly bool) ([]change, error) {
	untracked := "--untracked-files=all"
	if trackedOnly {
		untracked = "--untracked-files=no"
	}
	r, err := run(ctx, dir, nil, "status", "--porcelain=v1", "-z", untracked)
	if err != nil {
		return nil, err
	}
	var out []change
	fields := strings.Split(r.out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		c := change{Status: f[:2], Path: f[3:]}
		if c.Status[0] == 'R' || c.Status[0] == 'C' {
			i++ // the original path follows
		}
		out = append(out, c)
	}
	return out, nil
}

// operationInProgress names a merge, rebase, cherry-pick or revert left
// half done in a worktree ("" when there is none).
func operationInProgress(ctx context.Context, dir string) string {
	for _, op := range [][2]string{{"MERGE_HEAD", "a merge"}, {"rebase-merge", "a rebase"}, {"rebase-apply", "a rebase"},
		{"CHERRY_PICK_HEAD", "a cherry-pick"}, {"REVERT_HEAD", "a revert"}} {
		p, err := git(ctx, dir, "rev-parse", "--git-path", op[0])
		if err != nil {
			continue
		}
		p = filepath.FromSlash(p)
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		if _, err := os.Stat(p); err == nil {
			return op[1]
		}
	}
	return ""
}

// worktreeOf returns the folder where branch is checked out ("" if none).
func worktreeOf(ctx context.Context, root, branch string) (string, error) {
	out, err := git(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	var path string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = filepath.Clean(filepath.FromSlash(strings.TrimPrefix(line, "worktree ")))
		case line == "branch refs/heads/"+branch:
			return path, nil
		}
	}
	return "", nil
}

// count runs git rev-list --count on a range.
func count(ctx context.Context, dir, rng string) int {
	out, err := git(ctx, dir, "rev-list", "--count", rng)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(out)
	return n
}

// unmerged lists the conflicted files of a merge in progress.
func unmerged(ctx context.Context, dir string) []string {
	out, err := git(ctx, dir, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

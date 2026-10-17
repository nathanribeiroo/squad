package hub

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/server"
)

// DirInfo is GET /api/dirs/info?path=: what the first open (step 2, Pasta)
// shows about a folder before it becomes a workspace.
type DirInfo struct {
	Path     string   `json:"path"` // resolved
	Exists   bool     `json:"exists"`
	Git      bool     `json:"git"`
	Branch   string   `json:"branch,omitempty"`
	Commits  bool     `json:"commits"`          // a level starts from the last commit
	Changes  int      `json:"changes"`          // files with uncommitted changes
	EnvFiles []string `json:"envFiles"`         // .env* at the top, copied to the levels
	Folders  []string `json:"recent,omitempty"` // the workspaces' folders
}

func (h *Hub) handleDirInfo(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	info := DirInfo{EnvFiles: []string{}}
	for _, s := range h.List() {
		info.Folders = append(info.Folders, s.Cwd)
	}
	if p != "" {
		info.Path = server.ResolveCwd(p, home)
		if abs, err := filepath.Abs(info.Path); err == nil {
			info.Path = abs
		}
		describeDir(&info)
	}
	writeJSON(w, http.StatusOK, info)
}

// describeDir fills what git says about the folder (quickly: 3 s at most).
func describeDir(info *DirInfo) {
	fi, err := os.Stat(info.Path)
	if err != nil || !fi.IsDir() {
		return
	}
	info.Exists = true
	if matches, _ := filepath.Glob(filepath.Join(info.Path, ".env*")); len(matches) > 0 {
		for _, m := range matches {
			info.EnvFiles = append(info.EnvFiles, filepath.Base(m))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	git := func(args ...string) (string, bool) {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", info.Path}, args...)...).Output()
		return strings.TrimSpace(string(out)), err == nil
	}
	if _, ok := git("rev-parse", "--show-toplevel"); !ok {
		return
	}
	info.Git = true
	info.Branch, _ = git("branch", "--show-current")
	_, info.Commits = git("rev-parse", "--verify", "-q", "HEAD")
	if out, ok := git("status", "--porcelain"); ok && out != "" {
		info.Changes = len(strings.Split(out, "\n"))
	}
}

// handleWelcome is GET /api/welcome: whether this is a new installation
// (no workspace was saved when the server started), for the first steps.
func (h *Hub) handleWelcome(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	first := h.firstRun
	h.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]bool{"firstRun": first})
}

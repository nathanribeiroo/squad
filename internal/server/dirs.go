package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// maxDirSuggestions caps GET /api/dirs.
const maxDirSuggestions = 50

// handleDirs suggests folders for a path being typed (GET /api/dirs?path=…):
// the subfolders of the typed parent whose names start with the last
// segment, written the way the user typed them (relative paths stay relative
// to the workspace folder). Hidden folders only show for a "." prefix.
func (s *Server) handleDirs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"suggestions": DirSuggestions(r.URL.Query().Get("path"), s.store.Snapshot().Cwd),
	})
}

// DirSuggestions lists completions for typed, resolved against wsCwd.
func DirSuggestions(typed, wsCwd string) []string {
	seps := "/"
	if runtime.GOOS == "windows" {
		seps = `/\`
	}
	head, prefix := "", typed
	if i := strings.LastIndexAny(typed, seps); i >= 0 {
		head, prefix = typed[:i+1], typed[i+1:]
	}
	parent := ResolveCwd(head, wsCwd)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if len(name) < len(prefix) || !strings.EqualFold(name[:len(prefix)], prefix) {
			continue
		}
		if !e.IsDir() {
			// Follow symlinks to folders.
			fi, err := os.Stat(filepath.Join(parent, name))
			if err != nil || !fi.IsDir() {
				continue
			}
		}
		out = append(out, head+name)
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	if len(out) > maxDirSuggestions {
		out = out[:maxDirSuggestions]
	}
	return out
}

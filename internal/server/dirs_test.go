//go:build !windows

package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

func TestResolveCwd(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := []struct{ cwd, want string }{
		{"", "/ws"},
		{"api", "/ws/api"},
		{"../other", "/other"},
		{"/abs/dir/", "/abs/dir"},
		{"~", home},
		{"~/code", filepath.Join(home, "code")},
	}
	for _, c := range cases {
		if got := ResolveCwd(c.cwd, "/ws"); got != c.want {
			t.Errorf("ResolveCwd(%q) = %q; want %q", c.cwd, got, c.want)
		}
	}
}

// waitScreen waits until the terminal's screen contains want.
func waitScreen(t *testing.T, h *harness, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if term := h.srv.Terminals().Get(id); term != nil && strings.Contains(term.Snapshot(), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q", want)
}

func TestTerminalFolder(t *testing.T) {
	h := newHarness(t)
	ws := h.srv.Store().Snapshot().Cwd
	if err := os.Mkdir(filepath.Join(ws, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	add := func(cwd string) (*http.Response, store.Node) {
		resp, data := h.do("POST", "/api/nodes", store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "T-" + cwd, Agent: "shell", Shell: "/bin/sh", Cwd: cwd}}, nil)
		var n store.Node
		_ = json.Unmarshal(data, &n)
		if resp.StatusCode >= 400 && !strings.Contains(string(data), "folder not found") {
			t.Fatalf("error without the folder: %s", data)
		}
		return resp, n
	}
	if resp, _ := add("missing"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing folder: %d", resp.StatusCode)
	}
	resp, n := add("api")
	if resp.StatusCode != http.StatusCreated || n.Terminal.Cwd != "api" {
		t.Fatalf("relative folder: %d %+v", resp.StatusCode, n.Terminal)
	}
	_ = h.srv.Terminals().Get(n.ID).Input([]byte("pwd\r"))
	waitScreen(t, h, n.ID, filepath.Join(ws, "api"))

	// Editing: a bad folder is refused; a good one applies on restart.
	td := *n.Terminal
	td.Cwd = "nope"
	if resp, _ := h.do("PATCH", "/api/nodes/"+n.ID, map[string]any{"terminal": td}, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("patch to missing folder: %d", resp.StatusCode)
	}
	other := t.TempDir()
	td.Cwd = other
	if resp, data := h.do("PATCH", "/api/nodes/"+n.ID, map[string]any{"terminal": td}, nil); resp.StatusCode != 200 {
		t.Fatalf("patch folder: %d %s", resp.StatusCode, data)
	}
	if resp, data := h.do("POST", "/api/nodes/"+n.ID+"/restart", nil, nil); resp.StatusCode != 200 {
		t.Fatalf("restart: %d %s", resp.StatusCode, data)
	}
	_ = h.srv.Terminals().Get(n.ID).Input([]byte("pwd\r"))
	waitScreen(t, h, n.ID, other)
}

func TestDirSuggestions(t *testing.T) {
	ws := t.TempDir()
	for _, d := range []string{"api/src", "app", "Web", ".git"} {
		if err := os.MkdirAll(filepath.Join(ws, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(ws, "apx.txt"), nil, 0o644)
	_ = os.Symlink(filepath.Join(ws, "app"), filepath.Join(ws, "linked"))
	cases := []struct {
		typed string
		want  []string
	}{
		{"", []string{"api", "app", "linked", "Web"}},
		{"ap", []string{"api", "app"}},
		{"w", []string{"Web"}},
		{".", []string{".git"}},
		{"api/", []string{"api/src"}},
		{ws + "/a", []string{ws + "/api", ws + "/app"}},
		{"nope/", []string{}},
	}
	for _, c := range cases {
		if got := DirSuggestions(c.typed, ws); !reflect.DeepEqual(got, c.want) {
			t.Errorf("DirSuggestions(%q) = %q; want %q", c.typed, got, c.want)
		}
	}
}

func TestDirsAndShellsEndpoints(t *testing.T) {
	h := newHarness(t)
	ws := h.srv.Store().Snapshot().Cwd
	_ = os.Mkdir(filepath.Join(ws, "api"), 0o755)
	resp, data := h.do("GET", "/api/dirs?path="+url.QueryEscape("a"), nil, nil)
	var dirs struct{ Suggestions []string }
	if resp.StatusCode != 200 || json.Unmarshal(data, &dirs) != nil || !reflect.DeepEqual(dirs.Suggestions, []string{"api"}) {
		t.Fatalf("dirs: %d %s", resp.StatusCode, data)
	}
	resp, data = h.do("GET", "/api/shells", nil, nil)
	var shells struct{ Shells []terminal.ShellOption }
	if resp.StatusCode != 200 || json.Unmarshal(data, &shells) != nil || len(shells.Shells) == 0 || !shells.Shells[0].Default {
		t.Fatalf("shells: %d %s", resp.StatusCode, data)
	}
	for _, s := range shells.Shells[1:] {
		if s.Default {
			t.Fatalf("two defaults: %s", data)
		}
	}
}

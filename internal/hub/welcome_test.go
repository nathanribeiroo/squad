//go:build !windows

package hub

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// First open: a new installation is told apart, and step 2 reads a folder.
func TestWelcomeAndDirInfo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	h := newHarness(t, root)
	welcome := func(h *harness) bool {
		code, data := h.do("GET", "/api/welcome", nil, nil)
		var v struct{ FirstRun bool }
		if code != 200 || json.Unmarshal(data, &v) != nil {
			t.Fatalf("welcome: %d %s", code, data)
		}
		return v.FirstRun
	}
	if !welcome(h) {
		t.Fatal("a new installation is not a first run")
	}

	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}, {"commit", "-q", "--allow-empty", "-m", "first"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	_ = os.WriteFile(filepath.Join(repo, ".env"), []byte("A=1"), 0o600)
	_ = os.WriteFile(filepath.Join(repo, ".env.local"), []byte("B=2"), 0o600)
	code, data := h.do("GET", "/api/dirs/info?path="+url.QueryEscape(repo), nil, nil)
	var info DirInfo
	if code != 200 || json.Unmarshal(data, &info) != nil {
		t.Fatalf("info: %d %s", code, data)
	}
	if !info.Exists || !info.Git || info.Branch != "main" || !info.Commits || info.Changes != 2 || !reflect.DeepEqual(info.EnvFiles, []string{".env", ".env.local"}) {
		t.Fatalf("info: %+v", info)
	}
	plain := t.TempDir()
	_, data = h.do("GET", "/api/dirs/info?path="+url.QueryEscape(plain), nil, nil)
	_ = json.Unmarshal(data, &info)
	if !info.Exists || info.Git || info.Commits {
		t.Fatalf("plain folder: %+v", info)
	}
	_, data = h.do("GET", "/api/dirs/info?path="+url.QueryEscape(filepath.Join(plain, "nope")), nil, nil)
	_ = json.Unmarshal(data, &info)
	if info.Exists {
		t.Fatalf("missing folder: %+v", info)
	}

	// With a workspace saved, the next start isn't a first run.
	h.create("A")
	h.hub.Close()
	if welcome(newHarness(t, root)) {
		t.Fatal("a second start is a first run")
	}
}

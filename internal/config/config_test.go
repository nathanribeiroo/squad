package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHomeFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TORRE_HOME", dir)
	got, err := Home()
	if err != nil || got != dir {
		t.Fatalf("Home() = %q, %v; want %q", got, err, dir)
	}
	sub, err := Dir("workspaces", "w1")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(sub); err != nil || !fi.IsDir() {
		t.Fatalf("Dir did not create %s: %v", sub, err)
	}
}

// The names from before Squad became Torre keep working.
func TestLegacyEnv(t *testing.T) {
	old, cur := t.TempDir(), t.TempDir()
	for _, n := range []string{"SHELL", "BROWSER", "DEBUG"} {
		t.Setenv("TORRE_"+n, "")
		t.Setenv("SQUAD_"+n, "")
	}
	t.Setenv("TORRE_HOME", "")
	t.Setenv("SQUAD_HOME", old)
	t.Setenv("TORRE_PORT", "")
	t.Setenv("SQUAD_PORT", "7500")
	if got, _ := Home(); got != old {
		t.Fatalf("Home() = %q; want SQUAD_HOME %q", got, old)
	}
	if Port() != 7500 {
		t.Fatalf("Port() = %d; want SQUAD_PORT", Port())
	}
	if got := LegacyEnv(); len(got) != 2 || got[0] != "SQUAD_HOME" || got[1] != "SQUAD_PORT" {
		t.Fatalf("LegacyEnv() = %v", got)
	}
	t.Setenv("TORRE_HOME", cur)
	if got, _ := Home(); got != cur {
		t.Fatalf("Home() = %q; TORRE_HOME %q comes first", got, cur)
	}
	if got := LegacyEnv(); len(got) != 1 || got[0] != "SQUAD_PORT" {
		t.Fatalf("LegacyEnv() = %v", got)
	}
}

// fakeUserHome points os.UserHomeDir at a temporary folder, with neither
// TORRE_HOME nor SQUAD_HOME set.
func fakeUserHome(t *testing.T) string {
	u := t.TempDir()
	t.Setenv("HOME", u)
	t.Setenv("USERPROFILE", u)
	t.Setenv("TORRE_HOME", "")
	t.Setenv("SQUAD_HOME", "")
	return u
}

func TestMigrateHome(t *testing.T) {
	u := fakeUserHome(t)
	never := func(int) bool { return false }
	// Nothing from before: ~/.torre, and nothing to move.
	if got, _ := Home(); got != filepath.Join(u, ".torre") {
		t.Fatalf("Home() = %q", got)
	}
	if from, _, err := MigrateHome(never); from != "" || err != nil {
		t.Fatalf("MigrateHome with no ~/.squad: %q, %v", from, err)
	}

	// ~/.squad with a server that still runs: used as is, not moved.
	old := filepath.Join(u, ".squad")
	if err := os.MkdirAll(filepath.Join(old, "run"), 0o700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(old, "session-token"), []byte("tok\n"), 0o600)
	os.WriteFile(filepath.Join(old, "run", "server.json"), []byte(`{"pid": 4242}`), 0o600)
	if got, _ := Home(); got != old {
		t.Fatalf("Home() = %q; want the old folder until it moves", got)
	}
	if _, _, err := MigrateHome(func(pid int) bool { return pid == 4242 }); !errors.Is(err, ErrOldServer) {
		t.Fatalf("MigrateHome with the old server running: %v", err)
	}
	if !isDir(old) {
		t.Fatal("moved while the old server runs")
	}

	// The old server stopped: the folder moves, with what it had.
	from, to, err := MigrateHome(never)
	if err != nil || from != old || to != filepath.Join(u, ".torre") {
		t.Fatalf("MigrateHome = %q, %q, %v", from, to, err)
	}
	if b, err := os.ReadFile(filepath.Join(to, "session-token")); err != nil || string(b) != "tok\n" {
		t.Fatalf("token after the move: %q, %v", b, err)
	}
	if got, _ := Home(); got != to || isDir(old) {
		t.Fatalf("Home() = %q after the move (old still there: %v)", got, isDir(old))
	}
	// Once only: a new ~/.squad (an old binary started again) stays put.
	os.MkdirAll(old, 0o700)
	if from, _, err := MigrateHome(never); from != "" || err != nil || !isDir(old) {
		t.Fatalf("second MigrateHome: %q, %v", from, err)
	}
	if got, _ := Home(); got != to {
		t.Fatalf("Home() = %q; ~/.torre wins once it exists", got)
	}

	// TORRE_HOME (or SQUAD_HOME) set: never moves anything.
	u2 := fakeUserHome(t)
	os.MkdirAll(filepath.Join(u2, ".squad"), 0o700)
	t.Setenv("SQUAD_HOME", filepath.Join(u2, ".squad"))
	if from, _, err := MigrateHome(never); from != "" || err != nil {
		t.Fatalf("MigrateHome with SQUAD_HOME set: %q, %v", from, err)
	}
}

func TestServerInfoRoundTrip(t *testing.T) {
	t.Setenv("TORRE_HOME", t.TempDir())
	if _, err := ReadServerInfo(); err == nil {
		t.Fatal("expected error when no server is running")
	}
	in := ServerInfo{URL: "http://127.0.0.1:7420", Port: 7420, Token: "abc", PID: 42}
	if err := WriteServerInfo(in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadServerInfo()
	if err != nil || out != in {
		t.Fatalf("ReadServerInfo() = %+v, %v; want %+v", out, err, in)
	}
	RemoveServerInfo(41) // other pid: must keep the file
	if _, err := ReadServerInfo(); err != nil {
		t.Fatal("file removed for a different pid")
	}
	RemoveServerInfo(42)
	if _, err := ReadServerInfo(); err == nil {
		t.Fatal("file not removed for the owning pid")
	}
}

func TestWriteFileAtomicReplaces(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	if err := WriteFileAtomic(p, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "two" {
		t.Fatalf("content = %q", b)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestPortFromEnv(t *testing.T) {
	t.Setenv("SQUAD_PORT", "9999")
	if Port() != 9999 {
		t.Fatal("SQUAD_PORT ignored")
	}
	t.Setenv("SQUAD_PORT", "nope")
	if Port() != DefaultPort {
		t.Fatal("invalid SQUAD_PORT not ignored")
	}
}

func TestSessionTokenSurvivesRestarts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SQUAD_HOME", home)
	n := 0
	gen := func() string { n++; return strings.Repeat("ab", 24) }
	a, err := SessionToken(gen)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SessionToken(gen)
	if err != nil || a != b || n != 1 {
		t.Fatalf("second start: %q %v (generated %d times); want %q once", b, err, n, a)
	}
	p := filepath.Join(home, "session-token")
	if fi, err := os.Stat(p); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("token file: %v %v", fi, err)
	}
	// A damaged file is replaced; a deleted one logs everyone out.
	_ = os.WriteFile(p, []byte("nope"), 0o600)
	if c, _ := SessionToken(func() string { return strings.Repeat("cd", 24) }); c != strings.Repeat("cd", 24) {
		t.Fatalf("damaged file kept: %q", c)
	}
}

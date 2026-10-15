package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SQUAD_HOME", dir)
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

func TestServerInfoRoundTrip(t *testing.T) {
	t.Setenv("SQUAD_HOME", t.TempDir())
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

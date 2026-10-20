//go:build !windows

package pty

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syncBuffer is a bytes.Buffer safe for one writer and concurrent readers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func readAll(t *testing.T, p *Process, until string, timeout time.Duration) string {
	t.Helper()
	var buf syncBuffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&buf, p)
	}()
	deadline := time.After(timeout)
	for {
		if strings.Contains(buf.String(), until) {
			return buf.String()
		}
		select {
		case <-done:
			return buf.String()
		case <-deadline:
			t.Fatalf("timeout waiting for %q; got %q", until, buf.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestStartOutputAndExitCode(t *testing.T) {
	p, err := Start(Options{Argv: []string{"/bin/sh", "-c", "echo hello-pty; exit 3"}, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if out := readAll(t, p, "hello-pty", 5*time.Second); !strings.Contains(out, "hello-pty") {
		t.Fatalf("output %q", out)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	if p.ExitCode() != 3 {
		t.Fatalf("exit code %d", p.ExitCode())
	}
}

func TestResizeAndControllingTerminal(t *testing.T) {
	p, err := Start(Options{Argv: []string{"/bin/sh", "-c", "sleep 0.3; stty size; tty"}, Env: os.Environ(), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Resize(132, 43); err != nil {
		t.Fatal(err)
	}
	out := readAll(t, p, "/dev/", 5*time.Second)
	if !strings.Contains(out, "43 132") {
		t.Fatalf("stty size after resize: %q", out)
	}
	if strings.Contains(out, "not a tty") {
		t.Fatalf("no controlling tty: %q", out)
	}
}

func TestStopKillsProcessGroup(t *testing.T) {
	// The shell starts a background child in the same process group.
	p, err := Start(Options{Argv: []string{"/bin/sh", "-c", "sleep 60 & sleep 60"}, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, p) }()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	p.Stop(2 * time.Second)
	if time.Since(start) > 3*time.Second {
		t.Fatal("stop took too long")
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("not done after Stop")
	}
	if code := p.ExitCode(); code < 128 {
		t.Fatalf("expected signal exit, got %d", code)
	}
	_ = p.Close()
}

// A job of an interactive shell (an agent the shell started) has a process
// group of its own, the terminal's foreground one: stopping the terminal
// ends it too, even when it ignores the polite signals.
func TestStopKillsForegroundJob(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no bash")
	}
	pidFile := t.TempDir() + "/job.pid"
	p, err := Start(Options{Argv: []string{"/bin/bash", "--norc", "--noprofile", "-i"}, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, p) }()
	time.Sleep(300 * time.Millisecond)
	// The job ignores SIGHUP and SIGTERM, and writes its pid.
	if _, err := p.Write([]byte("sh -c 'trap \"\" HUP TERM; echo $$ > " + pidFile + "; exec sleep 60'\r")); err != nil {
		t.Fatal(err)
	}
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("the job didn't start")
	}
	p.Stop(500 * time.Millisecond)
	_ = p.Close()
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !running(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("the job (pid %d) survived the terminal", pid)
}

// running reports whether a process is alive (a zombie counts as gone).
func running(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return syscall.Kill(pid, 0) == nil // no /proc (macOS)
	}
	f := strings.Fields(string(b))
	return len(f) > 2 && f[2] != "Z"
}

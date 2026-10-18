//go:build !windows

package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var fakeAgent string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "torre-fakeagent")
	if err != nil {
		panic(err)
	}
	fakeAgent = filepath.Join(dir, "fakeagent")
	cmd := exec.Command("go", "build", "-o", fakeAgent, "github.com/nathanribeiroo/squad/internal/testutil/fakeagent")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build fakeagent: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type eventLog struct {
	mu     sync.Mutex
	events []Event
}

func (l *eventLog) add(e Event) { l.mu.Lock(); l.events = append(l.events, e); l.mu.Unlock() }

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func newTerm(t *testing.T, spec Spec) (*Manager, *Terminal) {
	t.Helper()
	m := NewManager("", nil)
	t.Cleanup(m.Shutdown)
	if spec.Shell == "" {
		spec.Shell = "/bin/sh"
	}
	if spec.Cwd == "" {
		spec.Cwd = t.TempDir()
	}
	term, err := m.Create(spec)
	if err != nil {
		t.Fatal(err)
	}
	return m, term
}

func TestShellEchoAndSnapshot(t *testing.T) {
	_, term := newTerm(t, Spec{Name: "sh", Agent: "shell"})
	if err := term.Input([]byte("echo torre-$((40+2))\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "echo output on screen", 5*time.Second, func() bool {
		return strings.Contains(term.Snapshot(), "torre-42")
	})
	if st, _ := term.State(); st != StatusRunning {
		t.Fatalf("status %s", st)
	}
}

func startFake(t *testing.T, env map[string]string) (*Terminal, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "agent.log")
	if env == nil {
		env = map[string]string{}
	}
	env["FAKEAGENT_LOG"] = logPath
	env["FAKEAGENT_WORK"] = "0.4"
	_, term := newTerm(t, Spec{Name: "fake", Agent: "custom", Command: fakeAgent, Env: env})
	waitFor(t, "fakeagent ready", 10*time.Second, func() bool {
		return strings.Contains(term.Snapshot(), "fakeagent ready")
	})
	return term, logPath
}

func readLog(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func TestPasteBracketedSubmitsOnce(t *testing.T) {
	term, logPath := startFake(t, nil)
	waitFor(t, "bracketed paste enabled", 5*time.Second, func() bool { return term.Screen().BracketedPaste() })
	if err := term.Paste("hello\nworld \x1b[201~sneaky", true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "turn end", 5*time.Second, func() bool { return strings.Contains(readLog(logPath), "TURN-END") })
	log := readLog(logPath)
	// ESC[200~ … ESC[201~ in one chunk; the embedded end marker was stripped.
	if !strings.Contains(log, "IN 1b5b3230307e68656c6c6f0a776f726c6420736e65616b791b5b3230317e") {
		t.Fatalf("paste not delimited as expected:\n%s", log)
	}
	if !strings.Contains(log, `SUBMIT "hello\nworld sneaky"`) {
		t.Fatalf("message not submitted:\n%s", log)
	}
	// The agent reacted after the first Enter, so no reinforcement Enter.
	if strings.Contains(log, "EMPTY-CR") {
		t.Fatalf("unexpected reinforcement CR:\n%s", log)
	}
	if !strings.Contains(term.Snapshot(), "answer: HELLO") {
		t.Fatalf("screen:\n%s", term.Snapshot())
	}
}

func TestPasteRawWhenNoBracketedMode(t *testing.T) {
	term, logPath := startFake(t, map[string]string{"FAKEAGENT_NOPASTE": "1"})
	time.Sleep(200 * time.Millisecond)
	if term.Screen().BracketedPaste() {
		t.Fatal("mode 2004 should be off")
	}
	if err := term.Paste("plain text", true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "turn end", 5*time.Second, func() bool { return strings.Contains(readLog(logPath), "TURN-END") })
	log := readLog(logPath)
	if strings.Contains(log, "1b5b3230307e") {
		t.Fatalf("raw paste must not use markers:\n%s", log)
	}
	if !strings.Contains(log, `SUBMIT "plain text"`) {
		t.Fatalf("not submitted:\n%s", log)
	}
}

func TestReinforcementEnterWhenSilent(t *testing.T) {
	// A program that ignores the first Enter and prints nothing: the second
	// CR must be sent.
	_, term := newTerm(t, Spec{Name: "cat", Agent: "custom", Command: "stty raw -echo; od -c"})
	time.Sleep(500 * time.Millisecond)
	before, _ := term.Output()
	if err := term.Paste("x", true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "two CRs reach od", 5*time.Second, func() bool {
		// od prints in 16-byte lines; force a flush by sending more bytes.
		_ = term.Raw([]byte("0123456789abcdef"))
		return strings.Count(term.Snapshot(), `\r`) >= 2
	})
	after, _ := term.Output()
	if after <= before {
		t.Fatal("no output observed")
	}
}

func TestSubscribeGetsRenderAndStream(t *testing.T) {
	_, term := newTerm(t, Spec{Name: "sh", Agent: "shell"})
	_ = term.Input([]byte("echo first-line\r"))
	waitFor(t, "first line", 5*time.Second, func() bool { return strings.Contains(term.Snapshot(), "first-line") })
	initial, ch, cancel := term.Subscribe()
	defer cancel()
	if !strings.Contains(initial, "first-line") {
		t.Fatalf("initial render lacks existing output: %q", initial)
	}
	_ = term.Input([]byte("echo second-line\r"))
	var got strings.Builder
	deadline := time.After(5 * time.Second)
	for !strings.Contains(got.String(), "second-line") {
		select {
		case chunk, ok := <-ch:
			if !ok {
				t.Fatal("subscription closed")
			}
			got.Write(chunk)
		case <-deadline:
			t.Fatalf("stream lacks second-line: %q", got.String())
		}
	}
}

func TestStopRestartAndEvents(t *testing.T) {
	var log eventLog
	m := NewManager("", log.add)
	defer m.Shutdown()
	term, err := m.Create(Spec{Name: "sh", Shell: "/bin/sh", Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	term.Stop()
	if st, _ := term.State(); st != StatusStopped {
		t.Fatalf("status after stop: %s", st)
	}
	if err := m.Restart(term.ID()); err != nil {
		t.Fatal(err)
	}
	_ = term.Input([]byte("echo back-again\r"))
	waitFor(t, "output after restart", 5*time.Second, func() bool { return strings.Contains(term.Snapshot(), "back-again") })
	_ = term.Input([]byte("exit 7\r"))
	waitFor(t, "exited status", 5*time.Second, func() bool {
		st, code := term.State()
		return st == StatusExited && code == 7
	})
	log.mu.Lock()
	defer log.mu.Unlock()
	var kinds []string
	for _, e := range log.events {
		if e.Kind == EventStatus {
			kinds = append(kinds, string(e.Status))
		}
	}
	joined := strings.Join(kinds, ",")
	if !strings.Contains(joined, "stopped") || !strings.HasSuffix(joined, "exited") {
		t.Fatalf("status events: %s", joined)
	}
}

// Writing to a terminal that was removed fails instead of waiting forever.
func TestRawAfterRemove(t *testing.T) {
	m, term := newTerm(t, Spec{Name: "sh"})
	m.Remove(term.ID())
	done := make(chan error, 1)
	go func() { done <- term.Raw([]byte("/exit\r")) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("no error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Raw hangs")
	}
}

func TestBuildEnvStripsMarkersAndSetsTerm(t *testing.T) {
	env := BuildEnv([]string{"PATH=/usr/bin", "CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=x", "CLAUDE_CODE_USE_BEDROCK=1", "TERM=dumb"},
		map[string]string{"SQUAD_TERMINAL_ID": "abc"}, "/opt/torre")
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"CLAUDECODE=", "CLAUDE_CODE_SESSION_ID=", "TERM=dumb"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("env still has %s:\n%s", bad, joined)
		}
	}
	for _, good := range []string{"PATH=/opt/torre:/usr/bin", "CLAUDE_CODE_USE_BEDROCK=1", "TERM=xterm-256color", "SQUAD_TERMINAL_ID=abc", "TERM_PROGRAM=Torre"} {
		if !strings.Contains(joined, good) {
			t.Fatalf("env lacks %s:\n%s", good, joined)
		}
	}
}

func TestQuoteAndLaunchLine(t *testing.T) {
	cases := []struct {
		f    ShellFamily
		in   []string
		want string
	}{
		{ShellPOSIX, []string{"claude", "--settings", "/tmp/a b.json"}, `claude --settings '/tmp/a b.json'`},
		{ShellPOSIX, []string{"echo", "it's"}, `echo 'it'\''s'`},
		{ShellFish, []string{"echo", "it's"}, `echo 'it\'s'`},
		{ShellPowerShell, []string{`C:\Program Files\x.exe`, "a'b"}, `& 'C:\Program Files\x.exe' 'a''b'`},
		{ShellCmd, []string{"x", `say "hi"`}, `x "say ""hi"""`},
	}
	for _, c := range cases {
		if got := LaunchLine(c.f, c.in); got != c.want {
			t.Errorf("LaunchLine(%v, %q) = %s; want %s", c.f, c.in, got, c.want)
		}
	}
	for f, want := range map[ShellFamily]string{
		ShellPOSIX:      "a --resume x || a --session-id x",
		ShellCmd:        "a --resume x || a --session-id x",
		ShellPowerShell: "a --resume x; if ($LASTEXITCODE -ne 0) { a --session-id x }",
	} {
		if got := OrElse(f, "a --resume x", "a --session-id x"); got != want {
			t.Errorf("OrElse(%v) = %s; want %s", f, got, want)
		}
	}
}

func TestFallsBackWhenShellCannotStart(t *testing.T) {
	// Exists and is executable but can't be exec'd: like a shell blocked by
	// group policy on Windows. The terminal must start the next candidate.
	bad := filepath.Join(t.TempDir(), "blocked-shell")
	if err := os.WriteFile(bad, []byte{0, 1, 2, 3}, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TORRE_SHELL", "/bin/sh")
	_, term := newTerm(t, Spec{Name: "sh", Agent: "shell", Shell: bad})
	if err := term.Input([]byte("echo fallback-$((1+1))\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "echo output from the fallback shell", 5*time.Second, func() bool {
		return strings.Contains(term.Snapshot(), "fallback-2")
	})
}

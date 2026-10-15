// Package terminal runs Squad terminals: a shell (and optionally an agent
// launched inside it) on a PTY, mirrored by a headless screen, with a
// serialized input queue and fan-out of output to attached clients.
package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/pty"
	"github.com/nathanribeiroo/squad/internal/screen"
)

// Spec describes a terminal.
type Spec struct {
	ID      string
	Name    string
	Agent   string // claude | codex | gemini | shell | custom
	Command string // line typed into the shell after start; empty = plain shell
	Shell   string // preferred shell executable; empty = platform default
	Cwd     string
	Env     map[string]string // extra variables (Squad identity, tokens)
	Cols    int
	Rows    int
}

// Status is the process lifecycle state (UI states like "working" come from
// the activity monitor, not from here).
type Status string

const (
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusExited   Status = "exited"
	StatusStopped  Status = "stopped"
	StatusFailed   Status = "failed"
)

// EventKind tells what changed in a terminal.
type EventKind string

const (
	EventStatus EventKind = "status" // Status / ExitCode changed
	EventTitle  EventKind = "title"  // window title (OSC 0/2) changed
)

// Event is emitted to the Manager's listener.
type Event struct {
	Kind     EventKind
	ID       string
	Status   Status
	ExitCode int
	Title    string
	Err      string
}

// Terminal is one running (or stopped) terminal.
type Terminal struct {
	spec   Spec
	family ShellFamily
	emit   func(Event)

	mu        sync.Mutex
	proc      *pty.Process
	scr       screen.Screen
	status    Status
	exitCode  int
	lastErr   string
	startedAt time.Time

	// subMu serializes "screen.Write + broadcast" against Subscribe so a new
	// client's initial render and the live stream never overlap or gap.
	subMu  sync.Mutex
	subs   map[int]*subscriber
	nextID int

	writeQ chan writeJob
	quit   chan struct{}

	bytesOut   atomic.Int64 // total output bytes
	lastOutput atomic.Int64 // unix nanos of the last output
}

type subscriber struct {
	ch     chan []byte
	closed bool
}

type writeJob struct {
	fn   func(*pty.Process) error
	done chan error
}

// Spec returns the terminal's spec.
func (t *Terminal) Spec() Spec { return t.spec }

// ID returns the terminal id.
func (t *Terminal) ID() string { return t.spec.ID }

// Name returns the display name.
func (t *Terminal) Name() string { return t.spec.Name }

// State returns status and exit code.
func (t *Terminal) State() (Status, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status, t.exitCode
}

// Screen returns the current screen mirror.
func (t *Terminal) Screen() screen.Screen {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scr
}

// Snapshot returns the visible screen text.
func (t *Terminal) Snapshot() string { return t.Screen().Snapshot() }

// Output returns the total output byte count and the time of the last output
// (zero if none); the activity monitor samples these.
func (t *Terminal) Output() (int64, time.Time) {
	n := t.lastOutput.Load()
	var last time.Time
	if n > 0 {
		last = time.Unix(0, n)
	}
	return t.bytesOut.Load(), last
}

func (t *Terminal) setStatus(s Status, code int, err string) {
	t.mu.Lock()
	t.status, t.exitCode, t.lastErr = s, code, err
	t.mu.Unlock()
	t.emit(Event{Kind: EventStatus, ID: t.spec.ID, Status: s, ExitCode: code, Err: err})
}

// start launches the shell and, if configured, types the agent launch line.
func (t *Terminal) start(cliDir string) error {
	argv, fam := ResolveShell(t.spec.Shell)
	t.family = fam
	cols, rows := t.spec.Cols, t.spec.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	scr := screen.New(cols, rows, screen.Events{Title: func(title string) {
		t.emit(Event{Kind: EventTitle, ID: t.spec.ID, Title: title})
	}})
	cwd := t.spec.Cwd
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	t.mu.Lock()
	t.scr = scr
	t.status = StatusStarting
	t.mu.Unlock()

	proc, err := pty.Start(pty.Options{
		Argv: argv, Dir: cwd, Cols: cols, Rows: rows,
		Env: BuildEnv(os.Environ(), t.spec.Env, cliDir),
	})
	if err != nil {
		_ = scr.Close()
		t.setStatus(StatusFailed, -1, err.Error())
		return fmt.Errorf("start %s: %w", argv[0], err)
	}
	t.mu.Lock()
	t.proc = proc
	t.startedAt = time.Now()
	t.mu.Unlock()
	t.setStatus(StatusRunning, 0, "")

	go t.readLoop(proc, scr)
	go func() { _, _ = io.Copy(proc, scr.Replies()) }()
	go t.waitLoop(proc)

	if t.spec.Command != "" {
		// Type-ahead: the line waits in the TTY buffer until the shell reads it.
		_ = t.enqueue(func(p *pty.Process) error {
			_, err := p.Write([]byte(t.spec.Command + "\r"))
			return err
		}, false)
	}
	return nil
}

func (t *Terminal) readLoop(proc *pty.Process, scr screen.Screen) {
	buf := make([]byte, 32*1024)
	for {
		n, err := proc.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			t.bytesOut.Add(int64(n))
			t.lastOutput.Store(time.Now().UnixNano())
			t.subMu.Lock()
			scr.Write(chunk)
			t.broadcastLocked(chunk)
			t.subMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (t *Terminal) waitLoop(proc *pty.Process) {
	<-proc.Done()
	code := proc.ExitCode()
	t.mu.Lock()
	stopped := t.status == StatusStopped
	current := t.proc == proc
	t.mu.Unlock()
	if !current {
		return // replaced by a restart
	}
	if !stopped {
		t.setStatus(StatusExited, code, "")
	}
}

func (t *Terminal) writeLoop() {
	for {
		select {
		case <-t.quit:
			return
		case job := <-t.writeQ:
			t.mu.Lock()
			proc := t.proc
			t.mu.Unlock()
			var err error
			if proc == nil {
				err = errors.New("terminal not running")
			} else {
				err = job.fn(proc)
			}
			if job.done != nil {
				job.done <- err
			}
		}
	}
}

// enqueue adds a job to the input queue; with wait it blocks until done.
func (t *Terminal) enqueue(fn func(*pty.Process) error, wait bool) error {
	job := writeJob{fn: fn}
	if wait {
		job.done = make(chan error, 1)
	}
	select {
	case t.writeQ <- job:
	case <-t.quit:
		return errors.New("terminal closed")
	}
	if !wait {
		return nil
	}
	return <-job.done
}

// Input sends keyboard input (from an attached client) in queue order.
func (t *Terminal) Input(p []byte) error {
	data := append([]byte(nil), p...)
	return t.enqueue(func(proc *pty.Process) error {
		_, err := proc.Write(data)
		return err
	}, false)
}

// Raw writes bytes as one write and waits for it to be sent.
func (t *Terminal) Raw(p []byte) error {
	data := append([]byte(nil), p...)
	return t.enqueue(func(proc *pty.Process) error {
		_, err := proc.Write(data)
		return err
	}, true)
}

// PasteTimings are the delays used by Paste (see docs/03 §7).
type PasteTimings struct {
	BeforeEnter time.Duration // after the text, before the first CR
	BeforeExtra time.Duration // before the reinforcement CR
}

// TimingsFor returns the delays for a text of n runes.
func TimingsFor(bracketed bool, n int) PasteTimings {
	if bracketed {
		switch {
		case n > 2000:
			return PasteTimings{900 * time.Millisecond, 300 * time.Millisecond}
		case n > 500:
			return PasteTimings{350 * time.Millisecond, 250 * time.Millisecond}
		default:
			return PasteTimings{200 * time.Millisecond, 250 * time.Millisecond}
		}
	}
	switch {
	case n > 2000:
		return PasteTimings{1500 * time.Millisecond, 1000 * time.Millisecond}
	case n > 500:
		return PasteTimings{500 * time.Millisecond, 500 * time.Millisecond}
	default:
		return PasteTimings{50 * time.Millisecond, 100 * time.Millisecond}
	}
}

var pasteMarkers = strings.NewReplacer("\x1b[200~", "", "\x1b[201~", "")

// Paste types text the way a user pastes it: as one bracketed paste when the
// program enabled mode 2004, raw otherwise. With submit it then presses
// Enter, and presses it once more only if the program produced no output in
// response (a safety net for TUIs that swallow the first Enter after a paste,
// without double-submitting on programs that reacted).
func (t *Terminal) Paste(text string, submit bool) error {
	text = pasteMarkers.Replace(text)
	return t.enqueue(func(proc *pty.Process) error {
		bracketed := t.Screen().BracketedPaste()
		tm := TimingsFor(bracketed, utf8.RuneCountInString(text))
		payload := text
		if bracketed {
			payload = "\x1b[200~" + text + "\x1b[201~"
		}
		if _, err := proc.Write([]byte(payload)); err != nil {
			return err
		}
		if !submit {
			return nil
		}
		time.Sleep(tm.BeforeEnter)
		before := t.bytesOut.Load()
		if _, err := proc.Write([]byte("\r")); err != nil {
			return err
		}
		time.Sleep(tm.BeforeExtra)
		if t.bytesOut.Load() == before {
			_, err := proc.Write([]byte("\r"))
			return err
		}
		return nil
	}, true)
}

// Resize changes the PTY and screen size.
func (t *Terminal) Resize(cols, rows int) error {
	t.mu.Lock()
	proc, scr := t.proc, t.scr
	t.spec.Cols, t.spec.Rows = cols, rows
	t.mu.Unlock()
	if scr != nil {
		scr.Resize(cols, rows)
	}
	if proc != nil {
		return proc.Resize(cols, rows)
	}
	return nil
}

// Subscribe attaches a client: it gets a render of the current screen (to
// paint first) and then every output chunk. When the client falls too far
// behind, its channel is closed and it should resubscribe.
func (t *Terminal) Subscribe() (initial string, ch <-chan []byte, cancel func()) {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	initial = t.Screen().Render()
	sub := &subscriber{ch: make(chan []byte, 256)}
	id := t.nextID
	t.nextID++
	t.subs[id] = sub
	return initial, sub.ch, func() {
		t.subMu.Lock()
		defer t.subMu.Unlock()
		if s, ok := t.subs[id]; ok {
			delete(t.subs, id)
			if !s.closed {
				s.closed = true
				close(s.ch)
			}
		}
	}
}

func (t *Terminal) broadcastLocked(chunk []byte) {
	for id, s := range t.subs {
		select {
		case s.ch <- chunk:
		default:
			s.closed = true
			close(s.ch)
			delete(t.subs, id)
		}
	}
}

// Stop ends the process tree; the terminal can be restarted.
func (t *Terminal) Stop() {
	t.mu.Lock()
	proc := t.proc
	if proc != nil {
		t.status = StatusStopped
	}
	t.mu.Unlock()
	if proc == nil {
		return
	}
	_ = proc.Close()
	t.setStatus(StatusStopped, proc.ExitCode(), "")
}

func (t *Terminal) close() {
	t.Stop()
	close(t.quit)
	t.subMu.Lock()
	for id, s := range t.subs {
		if !s.closed {
			s.closed = true
			close(s.ch)
		}
		delete(t.subs, id)
	}
	t.subMu.Unlock()
	if scr := t.Screen(); scr != nil {
		_ = scr.Close()
	}
}

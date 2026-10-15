// Package pty starts a process attached to a pseudo-terminal and manages its
// lifetime: a Unix PTY (creack/pty, new session + controlling terminal) or a
// Windows ConPTY (charmbracelet/x/xpty) inside a kill-on-close Job Object.
package pty

import (
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Options describe the process to start.
type Options struct {
	Argv       []string // program and arguments; Argv[0] is resolved via PATH
	Dir        string
	Env        []string // full environment (not merged with the parent's)
	Cols, Rows int
}

// Process is a running program attached to a PTY. Read returns its output;
// Write sends input as if typed.
type Process struct {
	cmd      *exec.Cmd
	rw       io.ReadWriter
	resize   func(cols, rows int) error
	closePTY func() error
	plat     platformState

	done     chan struct{}
	exitCode int
	waitErr  error

	closeOnce sync.Once
	writeMu   sync.Mutex
}

// Start launches the process.
func Start(o Options) (*Process, error) {
	if len(o.Argv) == 0 {
		return nil, errors.New("pty: empty argv")
	}
	if o.Cols <= 0 {
		o.Cols = 80
	}
	if o.Rows <= 0 {
		o.Rows = 24
	}
	return startPlatform(o)
}

func (p *Process) Read(b []byte) (int, error) { return p.rw.Read(b) }

// Write is safe for concurrent use; each call is written atomically.
func (p *Process) Write(b []byte) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.rw.Write(b)
}

// Resize changes the terminal size (the program receives SIGWINCH on Unix).
func (p *Process) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return p.resize(cols, rows)
}

// Pid returns the process id.
func (p *Process) Pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Done is closed when the process has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitCode is valid after Done: the exit status, 128+signal when killed by a
// signal, or -1 when unknown.
func (p *Process) ExitCode() int {
	<-p.done
	return p.exitCode
}

// Stop ends the process tree: politely first, forcibly after grace.
func (p *Process) Stop(grace time.Duration) {
	select {
	case <-p.done:
		return
	default:
	}
	stopTree(p, grace)
}

// Close stops the process (if still running) and releases the PTY.
func (p *Process) Close() error {
	p.Stop(2 * time.Second)
	var err error
	p.closeOnce.Do(func() {
		err = p.closePTY()
		releasePlatform(p)
	})
	return err
}

func (p *Process) finish(err error) {
	p.waitErr = err
	p.exitCode = exitCodeOf(p.cmd)
	close(p.done)
}

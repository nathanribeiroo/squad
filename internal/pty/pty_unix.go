//go:build !windows

package pty

import (
	"os/exec"
	"syscall"
	"time"

	creack "github.com/creack/pty"
)

type platformState struct{}

func startPlatform(o Options) (*Process, error) {
	cmd := exec.Command(o.Argv[0], o.Argv[1:]...)
	cmd.Dir = o.Dir
	cmd.Env = o.Env
	// StartWithSize puts the child in a new session with the PTY as its
	// controlling terminal (job control, Ctrl-C) and closes the parent's copy
	// of the slave, so reads end with EIO once the program exits.
	f, err := creack.StartWithSize(cmd, &creack.Winsize{Cols: uint16(o.Cols), Rows: uint16(o.Rows)}) //nolint:gosec
	if err != nil {
		return nil, err
	}
	p := &Process{
		cmd: cmd,
		rw:  f,
		resize: func(cols, rows int) error {
			return creack.Setsize(f, &creack.Winsize{Cols: uint16(cols), Rows: uint16(rows)}) //nolint:gosec
		},
		closePTY: f.Close,
		done:     make(chan struct{}),
	}
	go func() { p.finish(cmd.Wait()) }()
	return p, nil
}

// stopTree signals the whole process group (the session leader's pgid equals
// its pid): SIGHUP + SIGTERM, then SIGKILL after grace.
func stopTree(p *Process, grace time.Duration) {
	pgid := p.Pid()
	if pgid <= 0 {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGHUP)
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(grace):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-p.done
	}
}

func releasePlatform(*Process) {}

func exitCodeOf(cmd *exec.Cmd) int {
	ps := cmd.ProcessState
	if ps == nil {
		return -1
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

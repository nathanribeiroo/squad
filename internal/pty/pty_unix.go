//go:build !windows

package pty

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	creack "github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type platformState struct {
	master *os.File // the PTY, to learn its foreground process group
}

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
		plat:     platformState{master: f},
		done:     make(chan struct{}),
	}
	go func() { p.finish(cmd.Wait()) }()
	return p, nil
}

// stopTree signals the shell's process group (the session leader's pgid
// equals its pid) and the terminal's foreground one: a job the shell started,
// such as the agent, which job control puts in a group of its own. Left to
// the shell, such a job could outlive it, orphaned (an agent catching SIGHUP
// was seen stopped for good). SIGHUP, SIGCONT and SIGTERM first, then SIGKILL
// to what remains after grace.
func stopTree(p *Process, grace time.Duration) {
	pgid := p.Pid()
	if pgid <= 0 {
		return
	}
	groups := []int{pgid}
	if fg := foreground(p.plat.master); fg > 0 && fg != pgid {
		groups = append(groups, fg)
	}
	for _, g := range groups {
		_ = syscall.Kill(-g, syscall.SIGHUP)
		_ = syscall.Kill(-g, syscall.SIGCONT) // a stopped job gets them too
		_ = syscall.Kill(-g, syscall.SIGTERM)
	}
	deadline := time.Now().Add(grace)
	for {
		select {
		case <-p.done:
			if !anyAlive(groups[1:]) {
				return
			}
		default:
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, g := range groups {
		_ = syscall.Kill(-g, syscall.SIGKILL)
	}
	<-p.done
}

// foreground returns the PTY's foreground process group (0 if unknown).
func foreground(master *os.File) int {
	if master == nil {
		return 0
	}
	rc, err := master.SyscallConn()
	if err != nil {
		return 0
	}
	pg := 0
	_ = rc.Control(func(fd uintptr) {
		if v, err := unix.IoctlGetInt(int(fd), unix.TIOCGPGRP); err == nil {
			pg = v
		}
	})
	return pg
}

// anyAlive reports whether a process group still has a member.
func anyAlive(groups []int) bool {
	for _, g := range groups {
		if syscall.Kill(-g, 0) == nil {
			return true
		}
	}
	return false
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

//go:build windows

package pty

import (
	"context"
	"os/exec"
	"time"
	"unsafe"

	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/windows"
)

// platformState holds the Job Object that owns the process tree; closing it
// (or terminating it) kills every process assigned to it.
type platformState struct {
	job windows.Handle
}

func startPlatform(o Options) (*Process, error) {
	cp, err := xpty.NewConPty(o.Cols, o.Rows)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(o.Argv[0], o.Argv[1:]...)
	cmd.Dir = o.Dir
	cmd.Env = o.Env
	if err := cp.Start(cmd); err != nil {
		_ = cp.Close()
		return nil, err
	}
	p := &Process{
		cmd:      cmd,
		rw:       cp,
		resize:   cp.Resize,
		closePTY: cp.Close,
		done:     make(chan struct{}),
	}
	p.plat.job = newKillOnCloseJob(cmd.Process.Pid)
	go func() {
		err := xpty.WaitProcess(context.Background(), cmd)
		p.finish(err)
		// ConPTY keeps the output pipe open after the program exits; close the
		// pseudo console (after a moment to flush) so readers see EOF.
		time.Sleep(200 * time.Millisecond)
		p.closeOnce.Do(func() {
			_ = p.closePTY()
			releasePlatform(p)
		})
	}()
	return p, nil
}

// newKillOnCloseJob creates a Job Object with KILL_ON_JOB_CLOSE and assigns
// pid to it; children spawned afterwards inherit the job. Errors are ignored:
// without a job, Stop falls back to killing the main process only.
func newKillOnCloseJob(pid int) windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return 0
	}
	return job
}

// stopTree terminates the whole job (Windows has no SIGTERM for console
// programs launched this way); grace is unused but kept for parity.
func stopTree(p *Process, _ time.Duration) {
	if p.plat.job != 0 {
		_ = windows.TerminateJobObject(p.plat.job, 1)
	} else if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
}

func releasePlatform(p *Process) {
	if p.plat.job != 0 {
		_ = windows.CloseHandle(p.plat.job)
		p.plat.job = 0
	}
}

func exitCodeOf(cmd *exec.Cmd) int {
	if cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

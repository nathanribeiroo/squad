package floors

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareTree(*exec.Cmd) {}

// procTree is a Job Object holding the setup command and what it starts;
// closing it kills whatever is left.
type procTree windows.Handle

func watchTree(cmd *exec.Cmd) procTree {
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
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)) //nolint:gosec
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return 0
	}
	return procTree(job)
}

func killTree(cmd *exec.Cmd, t procTree) {
	if t != 0 {
		_ = windows.TerminateJobObject(windows.Handle(t), 1)
	} else if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (t procTree) release() {
	if t != 0 {
		_ = windows.CloseHandle(windows.Handle(t))
	}
}

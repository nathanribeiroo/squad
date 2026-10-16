//go:build !windows

package floors

import (
	"os/exec"
	"syscall"
)

// The setup command runs in its own process group, so stopping it stops
// what it started (npm and its children).
func prepareTree(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

type procTree struct{}

func watchTree(*exec.Cmd) procTree { return procTree{} }

func killTree(cmd *exec.Cmd, _ procTree) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

func (procTree) release() {}

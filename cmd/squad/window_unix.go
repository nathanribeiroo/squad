//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach keeps the window open when Ctrl-C stops the server, as on Windows.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

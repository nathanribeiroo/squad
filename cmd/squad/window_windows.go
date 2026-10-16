package main

import "os/exec"

// detach: a GUI browser started from the console doesn't get its Ctrl-C.
func detach(*exec.Cmd) {}

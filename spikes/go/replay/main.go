// Command replay feeds the first <offset> bytes of a raw PTY capture into
// charmbracelet/x/vt and prints the screen, to compare emulator versions.
//
// Usage: replay <raw> <cols> <rows> <offset>
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/charmbracelet/x/vt"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: replay <raw> <cols> <rows> <offset>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cols, _ := strconv.Atoi(os.Args[2])
	rows, _ := strconv.Atoi(os.Args[3])
	off, _ := strconv.Atoi(os.Args[4])
	if off > len(data) {
		off = len(data)
	}
	emu := vt.NewEmulator(cols, rows)
	// Replies to terminal queries go to a synchronous pipe; drain them or Write blocks.
	go func() { _, _ = io.Copy(io.Discard, emu) }()
	// Feed in 1 KiB chunks to mimic PTY reads splitting sequences.
	for i := 0; i < off; i += 1024 {
		end := min(i+1024, off)
		_, _ = emu.Write(data[i:end])
	}
	fmt.Print(emu.String())
}

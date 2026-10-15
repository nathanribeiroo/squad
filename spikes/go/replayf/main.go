// Command replayf is replay with the OSC pre-filter in front of the emulator.
// Usage: replayf <raw> <cols> <rows> <offset>
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/charmbracelet/x/vt"

	"spike/oscfilter"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cols, _ := strconv.Atoi(os.Args[2])
	rows, _ := strconv.Atoi(os.Args[3])
	off, _ := strconv.Atoi(os.Args[4])
	off = min(off, len(data))
	emu := vt.NewEmulator(cols, rows)
	go func() { _, _ = io.Copy(io.Discard, emu) }()
	titles := 0
	var last string
	f := &oscfilter.Filter{OnOSC: func(p []byte) {
		if len(p) > 2 && (p[0] == '0' || p[0] == '2') && p[1] == ';' {
			titles++
			last = string(p[2:])
		}
	}}
	for i := 0; i < off; i += 1024 {
		_, _ = emu.Write(f.Feed(data[i:min(i+1024, off)]))
	}
	fmt.Print(emu.String())
	fmt.Fprintf(os.Stderr, "titles=%d last=%q\n", titles, last)
}

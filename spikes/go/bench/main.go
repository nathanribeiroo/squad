// Command bench measures memory per emulator and parse throughput for vt.
// Usage: bench <raw> <terminals>
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/charmbracelet/x/vt"

	"spike/oscfilter"
)

func main() {
	data, _ := os.ReadFile(os.Args[1])
	n, _ := strconv.Atoi(os.Args[2])
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	emus := make([]*vt.Emulator, n)
	t0 := time.Now()
	for i := range emus {
		e := vt.NewEmulator(120, 36)
		sb, _ := strconv.Atoi(os.Getenv("SB")); e.SetScrollbackSize(sb)
		go func() { _, _ = io.Copy(io.Discard, e) }()
		f := &oscfilter.Filter{}
		for k := 0; k < 4; k++ { // ~4x the capture per terminal
			_, _ = e.Write(f.Feed(data))
		}
		emus[i] = e
	}
	el := time.Since(t0)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	total := float64(len(data)*4*n) / 1e6
	fmt.Printf("scrollback_lines(last)=%d ", emus[n-1].ScrollbackLen())
	fmt.Printf("vt: %d terminals, fed %.1f MB in %v (%.1f MB/s), heap per terminal %.2f MB, process sys %.1f MB\n",
		n, total, el.Round(time.Millisecond), total/el.Seconds(),
		float64(after.HeapAlloc-before.HeapAlloc)/1e6/float64(n), float64(after.Sys)/1e6)
	runtime.KeepAlive(emus)
}

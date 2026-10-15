// Command shot takes a PNG screenshot of a URL with a headless Chrome/Edge
// over CDP (chromedp). Used to compare the web UI with the design reference.
//
//	go run . -url http://127.0.0.1:8000/Squad.dc.html -w 1760 -h 1040 -out ref.png
//	go run . -url … -wait 3s -full -dark=false
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func main() {
	url := flag.String("url", "", "page to capture")
	w := flag.Int("w", 1440, "viewport width")
	h := flag.Int("h", 900, "viewport height")
	out := flag.String("out", "shot.png", "output PNG")
	wait := flag.Duration("wait", 2*time.Second, "wait after load")
	full := flag.Bool("full", false, "capture the full page height")
	dark := flag.Bool("dark", true, "prefers-color-scheme: dark")
	scale := flag.Float64("scale", 1, "device scale factor")
	js := flag.String("eval", "", "JavaScript to run before capturing")
	settle := flag.Duration("settle", 500*time.Millisecond, "wait after -eval")
	flag.Parse()
	if *url == "" {
		fmt.Fprintln(os.Stderr, "missing -url")
		os.Exit(2)
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.WindowSize(*w, *h))
	actx, cancelA := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelA()
	ctx, cancel := chromedp.NewContext(actx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 90*time.Second)
	defer cancelT()

	// Allocate the tab first so console output and uncaught exceptions of the
	// page can be printed to stderr while it loads.
	if err := chromedp.Do(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		os.Exit(1)
	}
	tgt := chromedp.FromContext(ctx).Target
	go func() {
		for ev, err := range cdp.Events(ctx, tgt, runtime.ConsoleAPICalled) {
			if err != nil {
				return
			}
			var parts []string
			for _, a := range ev.Args {
				if a.Description != "" {
					parts = append(parts, a.Description)
				} else {
					parts = append(parts, string(a.Value))
				}
			}
			fmt.Fprintf(os.Stderr, "console.%s: %s\n", ev.Type, strings.Join(parts, " "))
		}
	}()
	go func() {
		for ev, err := range cdp.Events(ctx, tgt, runtime.ExceptionThrown) {
			if err != nil {
				return
			}
			d := ev.ExceptionDetails
			msg := d.Text
			if d.Exception != nil && d.Exception.Description != "" {
				msg = d.Exception.Description
			}
			fmt.Fprintf(os.Stderr, "exception: %s (%s:%d)\n", msg, d.URL, d.LineNumber+1)
		}
	}()

	scheme := "light"
	if *dark {
		scheme = "dark"
	}
	err := chromedp.Do(ctx,
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			if _, err := cdp.Call(ctx, t, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
				Width: int64(*w), Height: int64(*h), DeviceScaleFactor: *scale,
			}); err != nil {
				return err
			}
			_, err := cdp.Call(ctx, t, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{
				Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}},
			})
			return err
		}),
		chromedp.Navigate(*url),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "navigate:", err)
		os.Exit(1)
	}
	time.Sleep(*wait)
	if *js != "" {
		if _, err := chromedp.Run(ctx, chromedp.Evaluate[any](*js)); err != nil {
			fmt.Fprintln(os.Stderr, "eval:", err)
		}
		time.Sleep(*settle)
	}
	var png []byte
	err = chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		params := page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatPng}
		if *full {
			beyond := true
			params.CaptureBeyondViewport = &beyond
			lm, err := cdp.Call(ctx, t, page.GetLayoutMetrics, struct{}{})
			if err != nil {
				return err
			}
			params.Clip = &page.Viewport{X: 0, Y: 0, Width: lm.CSSContentSize.Width, Height: lm.CSSContentSize.Height, Scale: 1}
		}
		res, err := cdp.Call(ctx, t, page.CaptureScreenshot, params)
		png = res.Data
		return err
	}))
	if err != nil {
		fmt.Fprintln(os.Stderr, "capture:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, png, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out, len(png), "bytes")
}

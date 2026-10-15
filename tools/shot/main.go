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
	"github.com/chromedp/cdproto/input"
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
	steps := flag.String("steps", "", "real input before capturing, ';'-separated: click X Y | dblclick X Y | drag X1 Y1 X2 Y2 | type TEXT | key NAME [alt|ctrl|shift|meta] | wheel X Y DY [ctrl] | sleep DUR | eval JS | shot FILE")
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
	if *steps != "" {
		for _, st := range strings.Split(*steps, ";") {
			if st = strings.TrimSpace(st); st == "" {
				continue
			}
			if err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error { return runStep(ctx, t, st) })); err != nil {
				fmt.Fprintf(os.Stderr, "step %q: %v\n", st, err)
			}
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

var keys = map[string]struct {
	key  string
	code int64
	text string
}{
	"Enter": {"Enter", 13, "\r"}, "Escape": {"Escape", 27, ""}, "Backspace": {"Backspace", 8, ""},
	"Delete": {"Delete", 46, ""}, "Tab": {"Tab", 9, ""},
}

// runStep sends one real input event over CDP (see -steps).
func runStep(ctx context.Context, t *chromedp.Target, step string) error {
	f := strings.Fields(step)
	num := func(i int) float64 {
		var v float64
		if i < len(f) {
			fmt.Sscan(f[i], &v)
		}
		return v
	}
	mouse := func(typ input.DispatchMouseEventType, x, y float64, buttons int64, mods int64) error {
		p := input.DispatchMouseEventParams{Type: typ, X: x, Y: y, Modifiers: mods}
		if typ == input.DispatchMouseEventTypeMousePressed || typ == input.DispatchMouseEventTypeMouseReleased {
			p.Button, p.ClickCount = input.MouseButtonLeft, 1
		}
		p.Buttons = buttons
		_, err := cdp.Call(ctx, t, input.DispatchMouseEvent, p)
		return err
	}
	mods := func(words []string) int64 {
		var m int64
		for _, w := range words {
			switch w {
			case "alt":
				m |= 1
			case "ctrl":
				m |= 2
			case "meta":
				m |= 4
			case "shift":
				m |= 8
			}
		}
		return m
	}
	switch f[0] {
	case "click":
		x, y := num(1), num(2)
		if err := mouse(input.DispatchMouseEventTypeMouseMoved, x, y, 0, 0); err != nil {
			return err
		}
		if err := mouse(input.DispatchMouseEventTypeMousePressed, x, y, 1, 0); err != nil {
			return err
		}
		return mouse(input.DispatchMouseEventTypeMouseReleased, x, y, 0, 0)
	case "dblclick":
		x, y := num(1), num(2)
		for _, n := range []int64{1, 2} {
			for _, typ := range []input.DispatchMouseEventType{input.DispatchMouseEventTypeMousePressed, input.DispatchMouseEventTypeMouseReleased} {
				p := input.DispatchMouseEventParams{Type: typ, X: x, Y: y, Button: input.MouseButtonLeft, ClickCount: n}
				if typ == input.DispatchMouseEventTypeMousePressed {
					p.Buttons = 1
				}
				if _, err := cdp.Call(ctx, t, input.DispatchMouseEvent, p); err != nil {
					return err
				}
			}
		}
		return nil
	case "drag":
		x1, y1, x2, y2 := num(1), num(2), num(3), num(4)
		if err := mouse(input.DispatchMouseEventTypeMouseMoved, x1, y1, 0, 0); err != nil {
			return err
		}
		if err := mouse(input.DispatchMouseEventTypeMousePressed, x1, y1, 1, 0); err != nil {
			return err
		}
		for i := 1; i <= 12; i++ {
			k := float64(i) / 12
			if err := mouse(input.DispatchMouseEventTypeMouseMoved, x1+(x2-x1)*k, y1+(y2-y1)*k, 1, 0); err != nil {
				return err
			}
			time.Sleep(16 * time.Millisecond)
		}
		return mouse(input.DispatchMouseEventTypeMouseReleased, x2, y2, 0, 0)
	case "type":
		_, err := cdp.Call(ctx, t, input.InsertText, input.InsertTextParams{Text: strings.TrimSpace(strings.TrimPrefix(step, "type"))})
		return err
	case "key":
		name := f[1]
		m := mods(f[2:])
		k, ok := keys[name]
		code := name
		if !ok && strings.HasPrefix(name, "Key") && len(name) == 4 {
			c := strings.ToLower(name[3:])
			k = struct {
				key  string
				code int64
				text string
			}{c, int64(name[3]), c}
		} else {
			code = k.key
		}
		down := input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyDown, Key: k.key, Code: code, WindowsVirtualKeyCode: k.code, Modifiers: m}
		if m == 0 || m == 8 {
			down.Text = k.text
		}
		if down.Text == "" {
			down.Type = input.DispatchKeyEventTypeRawKeyDown
		}
		if _, err := cdp.Call(ctx, t, input.DispatchKeyEvent, down); err != nil {
			return err
		}
		_, err := cdp.Call(ctx, t, input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyUp, Key: k.key, Code: code, WindowsVirtualKeyCode: k.code, Modifiers: m})
		return err
	case "wheel":
		// CDP requires deltaX, and the generated params omit zero values.
		p := input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseWheel, X: num(1), Y: num(2), DeltaX: 1e-9, DeltaY: num(3), Modifiers: mods(f[4:])}
		_, err := cdp.Call(ctx, t, input.DispatchMouseEvent, p)
		return err
	case "sleep":
		d, err := time.ParseDuration(f[1])
		if err != nil {
			return err
		}
		time.Sleep(d)
		return nil
	case "shot":
		// An intermediate capture of the viewport: shot FILE
		res, err := cdp.Call(ctx, t, page.CaptureScreenshot, page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatPng})
		if err != nil {
			return err
		}
		return os.WriteFile(f[1], res.Data, 0o644)
	case "eval":
		await := true
		_, err := cdp.Call(ctx, t, runtime.Evaluate, runtime.EvaluateParams{Expression: strings.TrimSpace(strings.TrimPrefix(step, "eval")), AwaitPromise: &await})
		return err
	}
	return fmt.Errorf("unknown step %q", f[0])
}

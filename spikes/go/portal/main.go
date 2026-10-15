// Command portal drives a Chromium-based browser (Chrome or Edge) over CDP with
// chromedp, without Playwright: accessibility snapshot with @eN refs and boxes,
// fill and click by ref with native input events, and one screencast frame.
//
// Usage: portal [-browser /path/to/chrome-or-msedge] [-out frame.jpg]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const testPage = `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><title>Pagina de Teste REA</title></head>
<body><h1>Teste de portal</h1>
<label for="nome">Nome</label><input id="nome" placeholder="Seu nome">
<button id="ok" onclick="document.getElementById('saida').textContent='clicado: '+document.getElementById('nome').value">Enviar</button>
<a href="#sobre">Sobre</a><div role="status" id="saida">aguardando</div>
<nav aria-label="principal"><ul><li>Um</li><li>Dois</li></ul></nav></body></html>`

// Roles kept in the snapshot: interactive controls plus landmarks/headings.
var keepRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true, "checkbox": true,
	"radio": true, "combobox": true, "listbox": true, "option": true, "menuitem": true,
	"tab": true, "switch": true, "slider": true, "spinbutton": true, "status": true,
	"navigation": true, "main": true, "heading": true, "LabelText": true, "dialog": true,
}

type ref struct {
	backend    cdp.BackendNodeID
	role, name string
	x, y, w, h float64
}

func str(v *accessibility.Value) string {
	if v == nil || len(v.Value) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(v.Value, &s) == nil {
		return s
	}
	return string(v.Value)
}

func main() {
	browser := flag.String("browser", "", "browser executable (empty = auto-detect Chrome/Chromium)")
	out := flag.String("out", "frame.jpg", "screencast frame output")
	flag.Parse()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testPage))
	}))
	defer srv.Close()

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.WindowSize(800, 600))
	if *browser != "" {
		opts = append(opts, chromedp.ExecPath(*browser))
	}
	actx, cancelA := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelA()
	ctx, cancel := chromedp.NewContext(actx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, 60*time.Second)
	defer cancelT()

	t0 := time.Now()
	must(chromedp.Do(ctx, chromedp.Navigate(srv.URL)))
	fmt.Printf("launch+navigate: %v\n", time.Since(t0).Round(time.Millisecond))

	// 1. Accessibility snapshot with refs.
	var refs []ref
	must(chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		res, err := cdp.Call(ctx, t, accessibility.GetFullAXTree, accessibility.GetFullAXTreeParams{})
		if err != nil {
			return err
		}
		for _, n := range res.Nodes {
			role := str(n.Role)
			if n.Ignored || !keepRoles[role] || n.BackendDOMNodeID == 0 {
				continue
			}
			box, err := cdp.Call(ctx, t, dom.GetBoxModel, dom.GetBoxModelParams{BackendNodeID: n.BackendDOMNodeID})
			if err != nil || box.Model == nil {
				continue
			}
			q := box.Model.Border
			refs = append(refs, ref{n.BackendDOMNodeID, role, str(n.Name), q[0], q[1], q[2] - q[0], q[5] - q[1]})
		}
		return nil
	})))
	title, err := chromedp.Run(ctx, chromedp.Title())
	must(err)
	url, err := chromedp.Run(ctx, chromedp.Location())
	must(err)
	fmt.Printf("viewport: 800x600  url: %s  title: %s\n", url, title)
	for i, r := range refs {
		fmt.Printf("@e%d %s %q [%.0f,%.0f %.0fx%.0f]\n", i+1, r.role, r.name, r.x, r.y, r.w, r.h)
	}

	find := func(role, name string) ref {
		for _, r := range refs {
			if r.role == role && r.name == name {
				return r
			}
		}
		fmt.Fprintf(os.Stderr, "ref %s %q not found\n", role, name)
		os.Exit(1)
		return ref{}
	}

	// 2. Arm listeners to check event trust, then fill + click by ref.
	_, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => { window.__ev = []; const b = document.getElementById('ok');
		['pointerdown','mousedown','mouseup','click'].forEach(t => b.addEventListener(t, e => window.__ev.push(t + ':' + e.isTrusted)));
		const i = document.getElementById('nome'); ['keydown','beforeinput','input'].forEach(t => i.addEventListener(t, e => window.__ev.push(t + ':' + e.isTrusted)));
		return true })()`))
	must(err)

	box := find("textbox", "Nome")
	btn := find("button", "Enviar")
	must(chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		if _, err := cdp.Call(ctx, t, dom.Focus, dom.FocusParams{BackendNodeID: box.backend}); err != nil {
			return err
		}
		if _, err := cdp.Call(ctx, t, input.InsertText, input.InsertTextParams{Text: "Nathan"}); err != nil {
			return err
		}
		if _, err := cdp.Call(ctx, t, dom.ScrollIntoViewIfNeeded, dom.ScrollIntoViewIfNeededParams{BackendNodeID: btn.backend}); err != nil {
			return err
		}
		cx, cy := btn.x+btn.w/2, btn.y+btn.h/2
		for _, typ := range []input.DispatchMouseEventType{input.DispatchMouseEventTypeMousePressed, input.DispatchMouseEventTypeMouseReleased} {
			p := input.DispatchMouseEventParams{Type: typ, X: cx, Y: cy, Button: input.MouseButtonLeft, ClickCount: 1}
			if _, err := cdp.Call(ctx, t, input.DispatchMouseEvent, p); err != nil {
				return err
			}
		}
		return nil
	})))
	result, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.getElementById('saida').textContent`))
	must(err)
	events, err := chromedp.Run(ctx, chromedp.Evaluate[string](`window.__ev.join(' ')`))
	must(err)
	fmt.Printf("after fill+click: %q\nevents: %s\n", result, events)

	// 3. One screencast frame (what the canvas would stream to the browser UI).
	frames := chromedp.Events(ctx, page.ScreencastFrame)
	q := int64(70)
	must(chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		_, err := cdp.Call(ctx, t, page.StartScreencast, page.StartScreencastParams{Format: page.StartScreencastFormatJpeg, Quality: &q, MaxWidth: 800, MaxHeight: 600})
		return err
	})))
	fctx, fcancel := context.WithTimeout(ctx, 5*time.Second)
	defer fcancel()
	_ = fctx
	for f, err := range frames {
		must(err)
		must(os.WriteFile(*out, f.Data, 0o644))
		fmt.Printf("screencast frame: %d bytes -> %s\n", len(f.Data), *out)
		_ = chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := cdp.Call(ctx, t, page.ScreencastFrameAck, page.ScreencastFrameAckParams{SessionID: f.SessionID})
			return err
		}))
		break
	}
	fmt.Println(strings.Repeat("-", 20), "total", time.Since(t0).Round(time.Millisecond))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

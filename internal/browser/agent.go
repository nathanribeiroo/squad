package browser

// What agents do with a page (squad portal …): read it as a list of elements
// with refs (@e1, @e2…), act on refs or coordinates, and wait for the page to
// settle before answering.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// keepRoles are listed in a snapshot: controls, plus headings and landmarks
// for context.
var keepRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true, "checkbox": true,
	"radio": true, "combobox": true, "listbox": true, "option": true, "menuitem": true,
	"menuitemcheckbox": true, "menuitemradio": true, "tab": true, "switch": true, "slider": true,
	"spinbutton": true, "status": true, "alert": true, "dialog": true, "alertdialog": true,
	"heading": true, "navigation": true, "main": true, "img": true,
}

// maxRefs caps a snapshot; pages with more ask the agent to scroll.
const maxRefs = 300

// Element is one entry of a snapshot.
type Element struct {
	Ref     string // "@e3"
	Role    string
	Name    string
	Value   string
	Checked string // "true", "false", "mixed" or ""
	X, Y    float64
	W, H    float64
	backend cdp.BackendNodeID
}

// Label is how the element is printed and shown: `button "Enviar"`.
func (e Element) Label() string { return fmt.Sprintf("%s %q", e.Role, e.Name) }

func axString(v *accessibility.Value) string {
	if v == nil || len(v.Value) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(v.Value, &s) == nil {
		return s
	}
	return strings.Trim(string(v.Value), `"`)
}

// Snapshot lists the page's elements with fresh refs (the previous refs stop
// working). Boxes are viewport CSS px; offscreen elements are marked.
func (p *Page) Snapshot() (string, error) {
	c, done := p.call()
	defer done()
	tree, err := chromedp.Call(c, accessibility.GetFullAXTree, accessibility.GetFullAXTreeParams{})
	if err != nil {
		return "", err
	}
	var els []Element
	for _, n := range tree.Nodes {
		role := axString(n.Role)
		if n.Ignored || !keepRoles[role] || n.BackendDOMNodeID == 0 {
			continue
		}
		box, err := chromedp.Call(c, dom.GetBoxModel, dom.GetBoxModelParams{BackendNodeID: n.BackendDOMNodeID})
		if err != nil || box.Model == nil || len(box.Model.Border) < 8 {
			continue // not rendered
		}
		q := box.Model.Border
		e := Element{Role: role, Name: oneLine(axString(n.Name)), Value: oneLine(axString(n.Value)), backend: n.BackendDOMNodeID,
			X: q[0], Y: q[1], W: q[2] - q[0], H: q[5] - q[1]}
		if e.W <= 0 || e.H <= 0 {
			continue
		}
		if e.Name == "" {
			e.Name = oneLine(axString(n.Description))
		}
		if e.Name == "" && e.Value == "" {
			e.Name = p.text(c, e.backend) // e.g. a status message
		}
		for _, prop := range n.Properties {
			if prop.Name == accessibility.PropertyNameChecked {
				e.Checked = axString(prop.Value)
			}
		}
		els = append(els, e)
	}
	// Reading order: top to bottom, then left to right.
	sort.SliceStable(els, func(i, j int) bool {
		if els[i].Y != els[j].Y {
			return els[i].Y < els[j].Y
		}
		return els[i].X < els[j].X
	})
	more := 0
	if len(els) > maxRefs {
		more = len(els) - maxRefs
		els = els[:maxRefs]
	}
	for i := range els {
		els[i].Ref = "@e" + strconv.Itoa(i+1)
	}
	p.mu.Lock()
	p.refs = map[string]Element{}
	for _, e := range els {
		p.refs[e.Ref] = e
	}
	st := p.state
	p.mu.Unlock()

	var sc struct{ X, Y, W, H float64 }
	if out, err := p.Evaluate(`JSON.stringify({X: scrollX, Y: scrollY, W: document.documentElement.scrollWidth, H: document.documentElement.scrollHeight})`, 5*time.Second); err == nil {
		_ = json.Unmarshal([]byte(out), &sc)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Page: %s\nURL: %s\n", st.Title, st.URL)
	fmt.Fprintf(&sb, "Viewport: %dx%d, scrolled to %.0f,%.0f of %.0fx%.0f\n\n", st.Width, st.Height, sc.X, sc.Y, sc.W, sc.H)
	for _, e := range els {
		fmt.Fprintf(&sb, "%s %s [%.0f,%.0f %.0fx%.0f]", e.Ref, e.Label(), e.X, e.Y, e.W, e.H)
		if e.Value != "" && e.Value != e.Name {
			fmt.Fprintf(&sb, " value=%q", e.Value)
		}
		if e.Checked != "" {
			sb.WriteString(" checked=" + e.Checked)
		}
		if e.Y+e.H < 0 || e.Y > float64(st.Height) || e.X+e.W < 0 || e.X > float64(st.Width) {
			sb.WriteString(" offscreen")
		}
		sb.WriteString("\n")
	}
	if len(els) == 0 {
		sb.WriteString("(no interactive elements)\n")
	}
	if more > 0 {
		fmt.Fprintf(&sb, "… and %d more elements; scroll or use eval to look further.\n", more)
	}
	return sb.String(), nil
}

// oneLine collapses whitespace and caps the length of names and values.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120]) + "…"
	}
	return s
}

// text is the visible text of an element without an accessible name.
func (p *Page) text(c context.Context, backend cdp.BackendNodeID) string {
	obj, err := chromedp.Call(c, dom.ResolveNode, dom.ResolveNodeParams{BackendNodeID: backend})
	if err != nil || obj.Object == nil {
		return ""
	}
	yes := true
	res, err := chromedp.Call(c, cdpruntime.CallFunctionOn, cdpruntime.CallFunctionOnParams{
		ObjectID: obj.Object.ObjectID, FunctionDeclaration: `function() { return this.innerText || '' }`, ReturnByValue: &yes,
	})
	if err != nil || res.Result == nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(res.Result.Value, &s)
	return oneLine(s)
}

// Ref returns an element of the last snapshot.
func (p *Page) Ref(ref string) (Element, error) {
	if !strings.HasPrefix(ref, "@") {
		ref = "@" + ref
	}
	p.mu.Lock()
	e, ok := p.refs[ref]
	p.mu.Unlock()
	if !ok {
		return Element{}, fmt.Errorf("unknown ref %s: take a new snapshot (refs expire when the page changes)", ref)
	}
	return e, nil
}

// center scrolls the element into view and returns its center now.
func (p *Page) center(e Element) (float64, float64, error) {
	c, done := p.call()
	defer done()
	_, _ = chromedp.Call(c, dom.ScrollIntoViewIfNeeded, dom.ScrollIntoViewIfNeededParams{BackendNodeID: e.backend})
	box, err := chromedp.Call(c, dom.GetBoxModel, dom.GetBoxModelParams{BackendNodeID: e.backend})
	if err != nil || box.Model == nil || len(box.Model.Border) < 8 {
		return 0, 0, fmt.Errorf("%s %s is no longer on the page: take a new snapshot", e.Ref, e.Label())
	}
	q := box.Model.Border
	return (q[0] + q[2]) / 2, (q[1] + q[5]) / 2, nil
}

// Click clicks the center of a ref and returns where it clicked.
func (p *Page) Click(ref string) (Element, float64, float64, error) {
	e, err := p.Ref(ref)
	if err != nil {
		return e, 0, 0, err
	}
	x, y, err := p.center(e)
	if err != nil {
		return e, 0, 0, err
	}
	return e, x, y, p.ClickAt(x, y)
}

// ClickAt clicks viewport coordinates.
func (p *Page) ClickAt(x, y float64) error {
	for _, typ := range []string{"move", "down", "up"} {
		if err := p.Mouse(MouseEvent{Type: typ, X: x, Y: y, Button: "left", Clicks: 1}); err != nil {
			return err
		}
	}
	return nil
}

// Fill replaces the text of a field (input, textarea, contenteditable) with
// text, typed as real input.
func (p *Page) Fill(ref, text string) (Element, float64, float64, error) {
	e, err := p.Ref(ref)
	if err != nil {
		return e, 0, 0, err
	}
	x, y, err := p.center(e)
	if err != nil {
		return e, 0, 0, err
	}
	c, done := p.call()
	defer done()
	if _, err := chromedp.Call(c, dom.Focus, dom.FocusParams{BackendNodeID: e.backend}); err != nil {
		return e, x, y, fmt.Errorf("%s %s can't take text: %v", e.Ref, e.Label(), err)
	}
	if obj, err := chromedp.Call(c, dom.ResolveNode, dom.ResolveNodeParams{BackendNodeID: e.backend}); err == nil && obj.Object != nil {
		_, _ = chromedp.Call(c, cdpruntime.CallFunctionOn, cdpruntime.CallFunctionOnParams{
			ObjectID: obj.Object.ObjectID,
			FunctionDeclaration: `function() {
				if ('value' in this) { this.value = ''; this.dispatchEvent(new Event('input', {bubbles: true})); }
				else if (this.isContentEditable) { this.textContent = ''; }
			}`,
		})
	}
	return e, x, y, p.InsertText(text)
}

// keyInfo is what a named key sends.
type keyInfo struct {
	key, code string
	keyCode   int
}

var namedKeys = map[string]keyInfo{
	"enter": {"Enter", "Enter", 13}, "tab": {"Tab", "Tab", 9}, "escape": {"Escape", "Escape", 27},
	"esc": {"Escape", "Escape", 27}, "backspace": {"Backspace", "Backspace", 8}, "delete": {"Delete", "Delete", 46},
	"space": {" ", "Space", 32}, "arrowleft": {"ArrowLeft", "ArrowLeft", 37}, "arrowup": {"ArrowUp", "ArrowUp", 38},
	"arrowright": {"ArrowRight", "ArrowRight", 39}, "arrowdown": {"ArrowDown", "ArrowDown", 40},
	"left": {"ArrowLeft", "ArrowLeft", 37}, "up": {"ArrowUp", "ArrowUp", 38}, "right": {"ArrowRight", "ArrowRight", 39},
	"down": {"ArrowDown", "ArrowDown", 40}, "home": {"Home", "Home", 36}, "end": {"End", "End", 35},
	"pageup": {"PageUp", "PageUp", 33}, "pagedown": {"PageDown", "PageDown", 34},
}

var modNames = map[string]int{"ctrl": ModCtrl, "control": ModCtrl, "alt": ModAlt, "option": ModAlt,
	"shift": ModShift, "meta": ModMeta, "cmd": ModMeta, "command": ModMeta}

// Press presses a key or combination: "Enter", "Escape", "a", "Control+a",
// "Shift+Tab".
func (p *Page) Press(combo string) error {
	parts := strings.Split(combo, "+")
	if combo == "+" {
		parts = []string{"+"}
	}
	mods := 0
	for _, m := range parts[:len(parts)-1] {
		bit, ok := modNames[strings.ToLower(strings.TrimSpace(m))]
		if !ok {
			return fmt.Errorf("unknown modifier %q (use Control, Alt, Shift or Meta)", m)
		}
		mods |= bit
	}
	name := strings.TrimSpace(parts[len(parts)-1])
	k, ok := namedKeys[strings.ToLower(name)]
	if !ok {
		r := []rune(name)
		if len(r) != 1 {
			return fmt.Errorf("unknown key %q (use a character or Enter, Tab, Escape, Backspace, Delete, Space, ArrowUp…, Home, End, PageUp, PageDown)", name)
		}
		k = keyInfo{key: name}
		up := unicode.ToUpper(r[0])
		switch {
		case up >= 'A' && up <= 'Z':
			k.code, k.keyCode = "Key"+string(up), int(up)
		case r[0] >= '0' && r[0] <= '9':
			k.code, k.keyCode = "Digit"+name, int(r[0])
		}
	}
	for _, typ := range []string{"down", "up"} {
		if err := p.Key(KeyEvent{Type: typ, Key: k.key, Code: k.code, KeyCode: k.keyCode, Mods: mods}); err != nil {
			return err
		}
	}
	return nil
}

// Scroll scrolls the page by dy CSS px (negative: up) at the viewport center.
func (p *Page) Scroll(dy float64) error {
	st := p.State()
	return p.Mouse(MouseEvent{Type: "wheel", X: float64(max(st.Width, 2)) / 2, Y: float64(max(st.Height, 2)) / 2, DY: dy})
}

// Screenshot returns a PNG of the viewport.
func (p *Page) Screenshot() ([]byte, error) {
	c, done := p.call()
	defer done()
	res, err := chromedp.Call(c, page.CaptureScreenshot, page.CaptureScreenshotParams{Format: page.CaptureScreenshotFormatPng})
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

// Settle waits for what an action started: a short pause, then the end of a
// navigation (up to timeout).
func (p *Page) Settle(timeout time.Duration) {
	time.Sleep(250 * time.Millisecond)
	deadline := time.Now().Add(timeout)
	for p.State().Loading && time.Now().Before(deadline) && p.alive() {
		time.Sleep(50 * time.Millisecond)
	}
}

// ConsoleEntry is a console message or uncaught error of the page.
type ConsoleEntry struct {
	Time  time.Time
	Level string // log, warning, error, exception…
	Text  string
}

// maxConsole is how many console entries a page keeps.
const maxConsole = 200

// watchConsole keeps the page's last console messages and errors.
func (p *Page) watchConsole(ctx context.Context) {
	go func() {
		for ev, err := range chromedp.Events(ctx, cdpruntime.ConsoleAPICalled) {
			if err != nil {
				return
			}
			parts := make([]string, 0, len(ev.Args))
			for _, a := range ev.Args {
				switch {
				case len(a.Value) > 0:
					var s string
					if json.Unmarshal(a.Value, &s) == nil {
						parts = append(parts, s)
					} else {
						parts = append(parts, string(a.Value))
					}
				case a.Description != "":
					parts = append(parts, a.Description)
				default:
					parts = append(parts, string(a.Type))
				}
			}
			p.addConsole(string(ev.Type), strings.Join(parts, " "))
		}
	}()
	for ev, err := range chromedp.Events(ctx, cdpruntime.ExceptionThrown) {
		if err != nil {
			return
		}
		d := ev.ExceptionDetails
		msg := d.Text
		if d.Exception != nil && d.Exception.Description != "" {
			msg = d.Exception.Description
		}
		p.addConsole("exception", msg)
	}
}

func (p *Page) addConsole(level, text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.console = append(p.console, ConsoleEntry{Time: time.Now(), Level: level, Text: text})
	if len(p.console) > maxConsole {
		p.console = p.console[len(p.console)-maxConsole:]
	}
}

// Console returns the last n console entries, oldest first.
func (p *Page) Console(n int) []ConsoleEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n <= 0 || n > len(p.console) {
		n = len(p.console)
	}
	return append([]ConsoleEntry(nil), p.console[len(p.console)-n:]...)
}

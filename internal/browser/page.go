package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// callTimeout bounds every protocol call of a tab.
const callTimeout = 15 * time.Second

// State is what the portal header shows.
type State struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	Loading    bool   `json:"loading"`
	Error      string `json:"error,omitempty"` // last navigation error (net::ERR_…)
	CanBack    bool   `json:"canBack"`
	CanForward bool   `json:"canForward"`
	Width      int    `json:"width"`  // viewport in CSS px
	Height     int    `json:"height"` //
}

// Frame is one screencast image (JPEG).
type Frame struct {
	Data []byte
	Time time.Time // when the browser painted it
}

// Page is the tab of one portal.
type Page struct {
	id     string
	ctx    context.Context // chromedp context of the tab
	cancel context.CancelFunc
	log    *slog.Logger

	mu        sync.Mutex
	state     State
	mainFrame cdp.FrameID
	subs      map[int]chan Frame
	stateFns  map[int]func(State)
	nextID    int
	casting   bool
	last      Frame
	refs      map[string]Element // last snapshot; cleared when the page changes
	console   []ConsoleEntry
}

func newPage(id string, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) *Page {
	p := &Page{id: id, ctx: ctx, cancel: cancel, log: log, subs: map[int]chan Frame{}, stateFns: map[int]func(State){}}
	c, done := p.call()
	defer done()
	_, _ = chromedp.Call(c, page.Enable, page.EnableParams{})
	// Headless tabs are never focused; without this, focus and caret don't work.
	_, _ = chromedp.Call(c, emulation.SetFocusEmulationEnabled, emulation.SetFocusEmulationEnabledParams{Enabled: true})
	go p.watch()
	go p.watchConsole(p.ctx)
	return p
}

func (p *Page) alive() bool { return p.ctx.Err() == nil }

func (p *Page) close() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ch := range p.subs {
		close(ch)
		delete(p.subs, id)
	}
}

// call returns a context for one protocol call of this tab.
func (p *Page) call() (context.Context, context.CancelFunc) {
	return context.WithTimeout(p.ctx, callTimeout)
}

// watch follows navigation, loading and screencast events until the tab
// closes.
func (p *Page) watch() {
	go func() {
		for ev, err := range chromedp.Events(p.ctx, page.FrameNavigated) {
			if err != nil {
				return
			}
			if ev.Frame.ParentID != "" {
				continue
			}
			p.mu.Lock()
			p.mainFrame = ev.Frame.ID
			p.refs = nil // a new document: old refs point nowhere
			if !strings.HasPrefix(ev.Frame.URL, "chrome-error:") {
				p.state.Error = ""
				p.state.URL = ev.Frame.URL + ev.Frame.URLFragment
			}
			p.mu.Unlock()
			p.refresh()
		}
	}()
	go func() {
		for _, err := range chromedp.Events(p.ctx, page.NavigatedWithinDocument) {
			if err != nil {
				return
			}
			p.refresh()
		}
	}()
	go func() {
		for ev, err := range chromedp.Events(p.ctx, page.FrameStartedLoading) {
			if err != nil {
				return
			}
			p.setLoading(ev.FrameID, true)
		}
	}()
	go func() {
		for ev, err := range chromedp.Events(p.ctx, page.FrameStoppedLoading) {
			if err != nil {
				return
			}
			p.setLoading(ev.FrameID, false)
		}
	}()
	for ev, err := range chromedp.Events(p.ctx, page.ScreencastFrame) {
		if err != nil {
			return
		}
		c, done := p.call()
		_, _ = chromedp.Call(c, page.ScreencastFrameAck, page.ScreencastFrameAckParams{SessionID: ev.SessionID})
		done()
		f := Frame{Data: ev.Data, Time: time.Now()}
		if ev.Metadata != nil && ev.Metadata.Timestamp > 0 {
			sec, frac := math.Modf(float64(ev.Metadata.Timestamp))
			f.Time = time.Unix(int64(sec), int64(frac*1e9))
		}
		p.mu.Lock()
		p.last = f
		for _, ch := range p.subs {
			select {
			case ch <- f:
			default: // a slow viewer skips frames instead of lagging behind
				select {
				case <-ch:
				default:
				}
				ch <- f
			}
		}
		p.mu.Unlock()
	}
}

func (p *Page) setLoading(frame cdp.FrameID, loading bool) {
	p.mu.Lock()
	main := p.mainFrame == "" || frame == p.mainFrame
	p.mu.Unlock()
	if !main {
		return
	}
	p.update(func(s *State) { s.Loading = loading })
	if !loading {
		p.refresh()
	}
}

// refresh reads URL, title and history from the browser.
func (p *Page) refresh() {
	c, done := p.call()
	defer done()
	h, err := chromedp.Call(c, page.GetNavigationHistory, cdp.Empty{})
	if err != nil || len(h.Entries) == 0 {
		return
	}
	i := int(h.CurrentIndex)
	if i < 0 || i >= len(h.Entries) {
		return
	}
	e := h.Entries[i]
	p.update(func(s *State) {
		if !strings.HasPrefix(e.URL, "chrome-error:") {
			s.URL = e.URL
		}
		s.Title = e.Title
		s.CanBack = i > firstEntry(h.Entries)
		s.CanForward = i < len(h.Entries)-1
	})
}

// firstEntry skips the blank page every new tab starts on, so the first
// real page has nothing to go back to.
func firstEntry(entries []*page.NavigationEntry) int {
	for i, e := range entries {
		if e.URL != "about:blank" {
			return i
		}
	}
	return 0
}

// update changes the state and tells the listeners.
func (p *Page) update(fn func(*State)) {
	p.mu.Lock()
	old := p.state
	fn(&p.state)
	st := p.state
	fns := make([]func(State), 0, len(p.stateFns))
	for _, f := range p.stateFns {
		fns = append(fns, f)
	}
	p.mu.Unlock()
	if st != old {
		for _, f := range fns {
			f(st)
		}
	}
}

// State returns the current state.
func (p *Page) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// OnState calls fn after every state change until cancel.
func (p *Page) OnState(fn func(State)) (cancel func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.nextID
	p.nextID++
	p.stateFns[id] = fn
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.stateFns, id)
	}
}

// Subscribe streams frames (the last one first, if any). The screencast runs
// only while someone is subscribed.
func (p *Page) Subscribe() (<-chan Frame, func()) {
	ch := make(chan Frame, 2)
	p.mu.Lock()
	id := p.nextID
	p.nextID++
	p.subs[id] = ch
	if p.last.Data != nil {
		ch <- p.last
	}
	start := !p.casting
	p.casting = true
	p.mu.Unlock()
	if start {
		q := int64(70)
		c, done := p.call()
		if _, err := chromedp.Call(c, page.StartScreencast, page.StartScreencastParams{Format: "jpeg", Quality: &q}); err != nil {
			p.log.Warn("portal screencast", "err", err)
		}
		done()
	}
	return ch, func() {
		p.mu.Lock()
		if c, ok := p.subs[id]; ok {
			close(c)
			delete(p.subs, id)
		}
		stop := p.casting && len(p.subs) == 0
		if stop {
			p.casting = false
		}
		p.mu.Unlock()
		if stop && p.alive() {
			c, done := p.call()
			_, _ = chromedp.Call(c, page.StopScreencast, cdp.Empty{})
			done()
		}
	}
}

// Navigate loads url. A failed load (refused, DNS…) is reported in State.Error.
func (p *Page) Navigate(url string) error {
	c, done := p.call()
	defer done()
	p.update(func(s *State) { s.Loading, s.Error = true, "" })
	res, err := chromedp.Call(c, page.Navigate, page.NavigateParams{URL: url})
	if err != nil {
		p.update(func(s *State) { s.Loading, s.Error = false, err.Error() })
		return err
	}
	if res.ErrorText != "" {
		p.update(func(s *State) { s.Loading, s.Error, s.URL = false, res.ErrorText, url })
	}
	return nil
}

// Back and Forward move in the tab's history.
func (p *Page) Back() error    { return p.history(-1) }
func (p *Page) Forward() error { return p.history(+1) }

func (p *Page) history(step int) error {
	c, done := p.call()
	defer done()
	h, err := chromedp.Call(c, page.GetNavigationHistory, cdp.Empty{})
	if err != nil {
		return err
	}
	i := int(h.CurrentIndex) + step
	if i < firstEntry(h.Entries) || i >= len(h.Entries) {
		return nil
	}
	_, err = chromedp.Call(c, page.NavigateToHistoryEntry, page.NavigateToHistoryEntryParams{EntryID: h.Entries[i].ID})
	return err
}

// Reload reloads the page.
func (p *Page) Reload() error {
	c, done := p.call()
	defer done()
	_, err := chromedp.Call(c, page.Reload, page.ReloadParams{})
	return err
}

// Resize sets the viewport in CSS px.
func (p *Page) Resize(w, h int) error {
	if w < 50 || h < 50 {
		return fmt.Errorf("browser: viewport %dx%d too small", w, h)
	}
	c, done := p.call()
	defer done()
	_, err := chromedp.Call(c, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
		Width: int64(w), Height: int64(h), DeviceScaleFactor: 1,
	})
	if err == nil {
		p.update(func(s *State) { s.Width, s.Height = w, h })
	}
	return err
}

// Modifier bits of mouse and key events (DevTools protocol).
const (
	ModAlt   = 1
	ModCtrl  = 2
	ModMeta  = 4
	ModShift = 8
)

// MouseEvent is a pointer event in viewport CSS px.
type MouseEvent struct {
	Type   string  `json:"event"`  // down, up, move, wheel
	X      float64 `json:"x"`      //
	Y      float64 `json:"y"`      //
	Button string  `json:"button"` // left, middle, right, none
	Clicks int     `json:"clicks"` // 1, 2 (double click)…
	DX     float64 `json:"dx"`     // wheel
	DY     float64 `json:"dy"`     //
	Mods   int     `json:"mods"`   //
}

var mouseTypes = map[string]input.DispatchMouseEventType{
	"down": input.DispatchMouseEventTypeMousePressed, "up": input.DispatchMouseEventTypeMouseReleased,
	"move": input.DispatchMouseEventTypeMouseMoved, "wheel": input.DispatchMouseEventTypeMouseWheel,
}

var buttonBits = map[string]int64{"left": 1, "right": 2, "middle": 4}

// Mouse dispatches a trusted pointer event.
func (p *Page) Mouse(ev MouseEvent) error {
	typ, ok := mouseTypes[ev.Type]
	if !ok {
		return fmt.Errorf("browser: mouse event %q", ev.Type)
	}
	button := input.MouseButton(ev.Button)
	if _, ok := buttonBits[ev.Button]; !ok {
		button = input.MouseButtonNone
	}
	params := input.DispatchMouseEventParams{
		Type: typ, X: ev.X, Y: ev.Y, Modifiers: int64(ev.Mods), Button: button, DeltaX: ev.DX, DeltaY: ev.DY,
	}
	if typ == input.DispatchMouseEventTypeMousePressed || typ == input.DispatchMouseEventTypeMouseReleased {
		params.ClickCount = int64(max(ev.Clicks, 1))
	}
	if typ == input.DispatchMouseEventTypeMousePressed {
		params.Buttons = buttonBits[ev.Button]
	}
	if typ == input.DispatchMouseEventTypeMouseWheel {
		// The protocol requires both deltas, and zero values are left out of
		// the request.
		if params.DeltaX == 0 {
			params.DeltaX = 1e-9
		}
		if params.DeltaY == 0 {
			params.DeltaY = 1e-9
		}
	}
	c, done := p.call()
	defer done()
	_, err := chromedp.Call(c, input.DispatchMouseEvent, params)
	return err
}

// KeyEvent is a key press or release, as the browser's KeyboardEvent saw it.
type KeyEvent struct {
	Type    string `json:"event"`   // down, up
	Key     string `json:"key"`     // "a", "Enter", "ArrowLeft"…
	Code    string `json:"code"`    // "KeyA"…
	KeyCode int    `json:"keyCode"` // Windows virtual key code
	Mods    int    `json:"mods"`
}

// Key dispatches a trusted key event; printable keys also type their text.
func (p *Page) Key(ev KeyEvent) error {
	params := input.DispatchKeyEventParams{
		Key: ev.Key, Code: ev.Code, WindowsVirtualKeyCode: int64(ev.KeyCode), Modifiers: int64(ev.Mods),
	}
	switch ev.Type {
	case "down":
		params.Type = input.DispatchKeyEventTypeRawKeyDown
		if text := keyText(ev); text != "" {
			params.Type, params.Text, params.UnmodifiedText = input.DispatchKeyEventTypeKeyDown, text, text
		}
	case "up":
		params.Type = input.DispatchKeyEventTypeKeyUp
	default:
		return fmt.Errorf("browser: key event %q", ev.Type)
	}
	c, done := p.call()
	defer done()
	_, err := chromedp.Call(c, input.DispatchKeyEvent, params)
	return err
}

// keyText is the text a key types: one character without Ctrl/Meta, or
// Enter and Tab.
func keyText(ev KeyEvent) string {
	if ev.Mods&(ModCtrl|ModMeta) != 0 {
		return ""
	}
	switch ev.Key {
	case "Enter":
		return "\r"
	case "Tab":
		return "\t"
	}
	if len([]rune(ev.Key)) == 1 {
		return ev.Key
	}
	return ""
}

// InsertText types text at the caret (paste).
func (p *Page) InsertText(text string) error {
	c, done := p.call()
	defer done()
	_, err := chromedp.Call(c, input.InsertText, input.InsertTextParams{Text: text})
	return err
}

// Evaluate runs JavaScript in the page and returns its result as JSON (or
// the string itself).
func (p *Page) Evaluate(js string, timeout time.Duration) (string, error) {
	c, done := context.WithTimeout(p.ctx, timeout+2*time.Second)
	defer done()
	yes := true
	res, err := chromedp.Call(c, cdpruntime.Evaluate, cdpruntime.EvaluateParams{
		Expression: js, ReturnByValue: &yes, AwaitPromise: &yes, Timeout: cdpruntime.TimeDelta(timeout.Milliseconds()),
	})
	if err != nil {
		return "", err
	}
	if d := res.ExceptionDetails; d != nil {
		msg := d.Text
		if d.Exception != nil && d.Exception.Description != "" {
			msg = d.Exception.Description
		}
		return "", errors.New(msg)
	}
	if res.Result == nil || len(res.Result.Value) == 0 {
		if res.Result != nil && res.Result.Description != "" {
			return res.Result.Description, nil
		}
		return "", nil
	}
	var s string
	if json.Unmarshal(res.Result.Value, &s) == nil {
		return s, nil
	}
	return string(res.Result.Value), nil
}

package browser

import (
	"bytes"
	"encoding/json"
	"image/jpeg"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const page1 = `<!doctype html><html><head><title>Teste</title></head><body style="margin:0">
<input id="nome" style="position:absolute;left:20px;top:20px;width:200px;height:30px">
<button id="ok" style="position:absolute;left:20px;top:80px;width:100px;height:30px"
  onclick="document.getElementById('saida').textContent='clicado: '+document.getElementById('nome').value">Enviar</button>
<div id="saida" style="position:absolute;left:20px;top:140px">aguardando</div>
</body></html>`

const page2 = `<!doctype html><html><head><title>Dois</title></head><body>segunda</body></html>`

// testManager starts a Manager on the installed browser, or skips.
func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	path := Find()
	if path == "" {
		t.Skip("no Chromium-based browser (set SQUAD_BROWSER)")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/2" {
			_, _ = w.Write([]byte(page2))
			return
		}
		_, _ = w.Write([]byte(page1))
	}))
	t.Cleanup(srv.Close)
	m := New(Options{ExecPath: path, Profile: t.TempDir(), IdleClose: 200 * time.Millisecond})
	t.Cleanup(m.Shutdown)
	return m, srv.URL
}

func waitState(t *testing.T, p *Page, what string, cond func(State) bool) State {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := p.State(); cond(s) {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s; state %+v", what, p.State())
	return State{}
}

func eval(t *testing.T, p *Page, js string) string {
	t.Helper()
	out, err := p.Evaluate(js, 5*time.Second)
	if err != nil {
		t.Fatalf("eval %s: %v", js, err)
	}
	return out
}

func click(t *testing.T, p *Page, selector string) {
	t.Helper()
	var r struct{ X, Y, Width, Height float64 }
	_ = json.Unmarshal([]byte(eval(t, p, `JSON.stringify(document.querySelector('`+selector+`').getBoundingClientRect())`)), &r)
	x, y := r.X+r.Width/2, r.Y+r.Height/2
	for _, typ := range []string{"move", "down", "up"} {
		if err := p.Mouse(MouseEvent{Type: typ, X: x, Y: y, Button: "left", Clicks: 1}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPageNavigateFramesAndInput(t *testing.T) {
	m, base := testManager(t)
	p, err := m.Page("portal-1")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := m.Page("portal-1"); again != p {
		t.Fatal("Page should return the open tab")
	}
	if err := p.Resize(600, 400); err != nil {
		t.Fatal(err)
	}
	if err := p.Navigate(base); err != nil {
		t.Fatal(err)
	}
	s := waitState(t, p, "page loaded", func(s State) bool { return s.Title == "Teste" && !s.Loading })
	if s.URL != base+"/" || s.Width != 600 || s.CanBack {
		t.Fatalf("state %+v", s)
	}

	// Frames flow while someone watches; they are JPEG.
	frames, stop := p.Subscribe()
	select {
	case f := <-frames:
		if !bytes.HasPrefix(f.Data, []byte{0xFF, 0xD8}) || time.Since(f.Time) > time.Minute {
			t.Fatalf("frame: % x… at %v", f.Data[:4], f.Time)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no frame")
	}

	// Real clicks and keys reach the page.
	click(t, p, "#nome")
	for _, k := range []KeyEvent{{Key: "o", Code: "KeyO", KeyCode: 79}, {Key: "i", Code: "KeyI", KeyCode: 73}} {
		k.Type = "down"
		_ = p.Key(k)
		k.Type = "up"
		_ = p.Key(k)
	}
	if err := p.InsertText("!"); err != nil {
		t.Fatal(err)
	}
	click(t, p, "#ok")
	if got := eval(t, p, `document.getElementById('saida').textContent`); got != "clicado: oi!" {
		t.Fatalf("after typing and clicking: %q", got)
	}
	if got := eval(t, p, `innerWidth + 'x' + innerHeight`); got != "600x400" {
		t.Fatalf("viewport %s", got)
	}
	// A change on screen sends a new frame.
	select {
	case <-frames:
	case <-time.After(10 * time.Second):
		t.Fatal("no frame after the page changed")
	}
	stop()

	// History.
	if err := p.Navigate(base + "/2"); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, "second page", func(s State) bool { return s.Title == "Dois" && s.CanBack && !s.Loading })
	if err := p.Back(); err != nil {
		t.Fatal(err)
	}
	waitState(t, p, "back", func(s State) bool { return s.Title == "Teste" && s.CanForward })

	// A refused connection is reported, then cleared by the next load.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := "http://" + ln.Addr().String()
	ln.Close()
	_ = p.Navigate(closed)
	waitState(t, p, "navigation error", func(s State) bool { return strings.Contains(s.Error, "ERR_CONNECTION_REFUSED") })
	_ = p.Navigate(base)
	waitState(t, p, "error cleared", func(s State) bool { return s.Error == "" && s.Title == "Teste" })
}

// Frames have the viewport's size even when the first load races the
// screencast, which Chromium then captures at the headless window's size.
func TestFramesHaveViewportSize(t *testing.T) {
	m, site := testManager(t)
	p, err := m.Page("p")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Resize(638, 479); err != nil {
		t.Fatal(err)
	}
	go func() { _ = p.Navigate(site) }()
	frames, stop := p.Subscribe()
	defer stop()
	expect := func(when string) {
		t.Helper()
		got := 0
		timeout := time.After(3 * time.Second)
		for {
			select {
			case f := <-frames:
				img, err := jpeg.DecodeConfig(bytes.NewReader(f.Data))
				if err != nil || img.Width != 638 || img.Height != 479 {
					t.Fatalf("%s: frame %dx%d (%v); want 638x479", when, img.Width, img.Height, err)
				}
				got++
			case <-timeout:
				if got == 0 {
					t.Fatalf("%s: no frame", when)
				}
				return
			}
		}
	}
	expect("first load")
	_ = p.Navigate(strings.Replace(site, "127.0.0.1", "localhost", 1) + "/2") // another site, another renderer
	expect("other site")
}

func TestCloseTabsAndIdleBrowser(t *testing.T) {
	m, base := testManager(t)
	a, _ := m.Page("a")
	b, _ := m.Page("b")
	_ = a.Navigate(base)
	_ = b.Navigate(base + "/2")
	waitState(t, b, "b loaded", func(s State) bool { return s.Title == "Dois" })
	m.Close("a")
	if m.Get("a") != nil || m.Get("b") == nil || !m.Running() {
		t.Fatal("closing a tab must keep the others and the browser")
	}
	m.Close("b")
	deadline := time.Now().Add(5 * time.Second)
	for m.Running() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Running() {
		t.Fatal("browser still running after its last tab")
	}
	// It starts again on demand.
	c, err := m.Page("c")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Navigate(base)
	waitState(t, c, "c loaded", func(s State) bool { return s.Title == "Teste" })
}

func TestFindHonorsSquadBrowser(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "my-chrome")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SQUAD_BROWSER", exe)
	if got := Find(); got != exe {
		t.Fatalf("Find = %q", got)
	}
	t.Setenv("SQUAD_BROWSER", filepath.Join(t.TempDir(), "missing"))
	if got := Find(); got != "" {
		t.Fatalf("missing SQUAD_BROWSER should give no browser, got %q", got)
	}
	m := New(Options{})
	if _, err := m.Page("x"); err != ErrNoBrowser {
		t.Fatalf("no browser: %v", err)
	}
}

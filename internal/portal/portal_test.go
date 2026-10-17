package portal

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

const token = "portal-test"

const site = `<!doctype html><html><head><title>Teste</title></head><body style="margin:0">
<input id="nome" style="position:absolute;left:20px;top:20px;width:200px;height:30px">
<button id="ok" style="position:absolute;left:20px;top:80px;width:100px;height:30px"
  onclick="document.getElementById('saida').textContent='clicado: '+document.getElementById('nome').value">Enviar</button>
<div id="saida" style="position:absolute;left:20px;top:140px">aguardando</div></body></html>`

type harness struct {
	t    *testing.T
	base string // torre server
	site string // test web site
	st   *store.Store
	br   *browser.Manager
}

func newHarness(t *testing.T, withBrowser bool) *harness {
	t.Helper()
	var br *browser.Manager
	if withBrowser {
		path := browser.Find()
		if path == "" {
			t.Skip("no Chromium-based browser (set TORRE_BROWSER)")
		}
		br = browser.New(browser.Options{ExecPath: path, Profile: t.TempDir()})
		t.Cleanup(br.Shutdown)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/2" {
			_, _ = io.WriteString(w, `<!doctype html><title>Dois</title>segunda`)
			return
		}
		_, _ = io.WriteString(w, site)
	}))
	t.Cleanup(web.Close)
	ln, port, err := server.ListenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "workspace.json"), store.Workspace{ID: "w", Name: "w", Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(server.Options{Port: port, Token: token, Store: st})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, srv.Bus(), br, port, nil)
	srv.HandleSession("GET /ws/portal/{id}", http.HandlerFunc(svc.HandleWS))
	svc.Start()
	hs := httptest.NewUnstartedServer(srv.Handler())
	hs.Listener = ln
	hs.Start()
	t.Cleanup(func() { hs.Close(); svc.Close(); srv.Shutdown() })
	return &harness{t: t, base: fmt.Sprintf("http://127.0.0.1:%d", port), site: web.URL, st: st, br: br}
}

func (h *harness) do(method, path string, body any) store.Node {
	h.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, h.base+path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		h.t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, data)
	}
	var n store.Node
	_ = json.Unmarshal(data, &n)
	return n
}

func (h *harness) dial(id string) *websocket.Conn {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.base, "http", "ws", 1)+"/ws/portal/"+id, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	c.SetReadLimit(16 << 20)
	h.t.Cleanup(func() { c.CloseNow() })
	return c
}

// stream reads a portal socket in the background.
type stream struct {
	states chan browser.State
	errs   chan string
	frames chan []byte
}

func read(c *websocket.Conn) *stream {
	s := &stream{states: make(chan browser.State, 64), errs: make(chan string, 4), frames: make(chan []byte, 64)}
	go func() {
		for {
			typ, data, err := c.Read(context.Background())
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				select {
				case s.frames <- data:
				default:
				}
				continue
			}
			var m struct {
				Type  string
				State browser.State
				Error string
			}
			_ = json.Unmarshal(data, &m)
			if m.Type == "error" {
				s.errs <- m.Error
			} else {
				s.states <- m.State
			}
		}
	}()
	return s
}

func (s *stream) waitState(t *testing.T, what string, cond func(browser.State) bool) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case st := <-s.states:
			if cond(st) {
				return
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %s", what)
		}
	}
}

func send(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func clickAt(t *testing.T, c *websocket.Conn, x, y float64) {
	for _, ev := range []string{"move", "down", "up"} {
		send(t, c, map[string]any{"type": "mouse", "event": ev, "x": x, "y": y, "button": "left", "clicks": 1})
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestPortalStreamInputAndAddress(t *testing.T) {
	h := newHarness(t, true)
	n := h.do("POST", "/api/nodes", store.Node{Kind: store.KindPortal, W: 560, H: 420, Portal: &store.PortalData{URL: h.site}})
	c := h.dial(n.ID)
	s := read(c)
	s.waitState(t, "page loaded", func(st browser.State) bool { return st.Title == "Teste" && !st.Loading })

	// Frames: 8-byte paint time, then a JPEG.
	select {
	case f := <-s.frames:
		ms := math.Float64frombits(binary.BigEndian.Uint64(f[:8]))
		if age := time.Since(time.UnixMilli(int64(ms))); age < -time.Second || age > time.Minute || !bytes.HasPrefix(f[8:], []byte{0xFF, 0xD8}) {
			t.Fatalf("frame header %v age %v", f[:10], age)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no frame")
	}

	// The viewport is the node's body; input from the socket reaches the page.
	p := h.br.Get(n.ID)
	js := func(code string) string { out, _ := p.Evaluate(code, 5*time.Second); return out }
	if got := js(`innerWidth + 'x' + innerHeight`); got != fmt.Sprintf("%dx%d", 560-chromeW, 420-chromeH) {
		t.Fatalf("viewport %s", got)
	}
	clickAt(t, c, 120, 35)
	for _, k := range []string{"o", "k"} {
		for _, ev := range []string{"down", "up"} {
			send(t, c, map[string]any{"type": "key", "event": ev, "key": k, "code": "Key" + strings.ToUpper(k), "keyCode": int(strings.ToUpper(k)[0])})
		}
	}
	send(t, c, map[string]any{"type": "text", "text": "!"})
	clickAt(t, c, 70, 95)
	waitFor(t, "clicked", func() bool { return js(`document.getElementById('saida').textContent`) == "clicado: ok!" })

	// Navigating inside the page updates the portal's address on the canvas.
	js(`location.href = '/2'`)
	waitFor(t, "address saved", func() bool { n2, _ := h.st.Node(n.ID); return n2.Portal.URL == h.site+"/2" })
	send(t, c, map[string]any{"type": "nav", "action": "back"})
	s.waitState(t, "back", func(st browser.State) bool { return st.Title == "Teste" && st.CanForward })

	// Editing the address navigates; resizing the node resizes the page;
	// moving it does neither.
	h.do("PATCH", "/api/nodes/"+n.ID, map[string]any{"portal": map[string]string{"url": h.site + "/2"}})
	s.waitState(t, "address edited", func(st browser.State) bool { return st.Title == "Dois" })
	h.do("PATCH", "/api/nodes/"+n.ID, map[string]any{"w": 760, "x": 300})
	waitFor(t, "resized", func() bool { return js(`innerWidth`) == fmt.Sprint(760-chromeW) })
	if st := p.State(); st.Title != "Dois" {
		t.Fatalf("moving the node changed the page: %+v", st)
	}

	// Removing the portal closes its tab.
	req, _ := http.NewRequest("DELETE", h.base+"/api/nodes/"+n.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %v %v", resp, err)
	}
	waitFor(t, "tab closed", func() bool { return h.br.Get(n.ID) == nil })
}

func TestPortalWithoutBrowser(t *testing.T) {
	h := newHarness(t, false)
	n := h.do("POST", "/api/nodes", store.Node{Kind: store.KindPortal, Portal: &store.PortalData{URL: h.site}})
	s := read(h.dial(n.ID))
	select {
	case e := <-s.errs:
		if !strings.Contains(e, "disabled") {
			t.Fatalf("error %q", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no error message")
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"localhost:5173":       "http://localhost:5173",
		"127.0.0.1:8080/x":     "http://127.0.0.1:8080/x",
		"example.com":          "https://example.com",
		"http://example.com":   "http://example.com",
		"https://a.b/c?d=1":    "https://a.b/c?d=1",
		"about:blank":          "about:blank",
		"  docs.torre.dev/a  ": "https://docs.torre.dev/a",
		"":                     "",
	} {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q; want %q", in, got, want)
		}
	}
}

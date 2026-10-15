//go:build !windows

package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/browser"
	"github.com/nathanribeiroo/squad/internal/portal"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

const portalSite = `<!doctype html><html><head><title>Cadastro</title></head><body>
<label>Nome <input id="nome"></label>
<button onclick="console.log('enviou', nome.value); saida.textContent = 'oi ' + nome.value">Enviar</button>
<div role="status" id="saida">vazio</div>
<a href="/2">Próxima</a></body></html>`

func newPortalEnv(t *testing.T) *env {
	t.Helper()
	path := browser.Find()
	if path == "" {
		t.Skip("no Chromium-based browser (set SQUAD_BROWSER)")
	}
	br := browser.New(browser.Options{ExecPath: path, Profile: t.TempDir()})
	t.Cleanup(br.Shutdown)
	ln, port, err := server.ListenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "workspace.json"), store.Workspace{ID: "w", Name: "test", Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{
		Server:   server.Options{Port: port, Token: "tok", Store: st, CLIPath: filepath.Join(binDir, "squad")},
		Activity: fastActivity, Limits: fastLimits, Browser: br,
	})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewUnstartedServer(a.Server.Handler())
	hs.Listener = ln
	hs.Start()
	a.Monitor.Start()
	a.Portals.Start()
	t.Cleanup(func() { hs.Close(); a.Close() })
	return &env{t: t, app: a, dir: dir}
}

// addPortal creates a portal the way the canvas does (named by the service).
func (e *env) addPortal(url string) store.Node {
	e.t.Helper()
	n := store.Node{Kind: store.KindPortal, W: 760, H: 560, Portal: &store.PortalData{URL: url}}
	if err := e.app.Portals.PrepareNode(&n); err != nil {
		e.t.Fatal(err)
	}
	n, err := e.app.Server.Store().AddNode(n)
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func ref(t *testing.T, snap, line string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^(@e\d+) ` + regexp.QuoteMeta(line)).FindStringSubmatch(snap)
	if m == nil {
		t.Fatalf("no %s in snapshot:\n%s", line, snap)
	}
	return m[1]
}

func TestPortalCommands(t *testing.T) {
	e := newPortalEnv(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/2" {
			_, _ = io.WriteString(w, `<!doctype html><title>Segunda</title><h1>Segunda</h1>`)
			return
		}
		_, _ = io.WriteString(w, portalSite)
	}))
	defer site.Close()

	a := e.agent("A")
	p1 := e.addPortal(site.URL)
	p2 := e.addPortal(strings.TrimPrefix(site.URL, "http://") + "/2") // typed without a scheme
	if p1.Portal.Name != "Portal" || p2.Portal.Name != "Portal 2" || p2.Portal.URL != site.URL+"/2" {
		t.Fatalf("names/urls: %+v %+v", p1.Portal, p2.Portal)
	}

	if r := e.run(a, "portal", "snapshot"); r.Code != 404 || !strings.Contains(r.Body, "no portal is connected") {
		t.Fatalf("not connected: %d %s", r.Code, r.Body)
	}
	e.connect(a, p1)
	if r := e.run(a, "list"); !strings.Contains(r.Body, `name: "Portal", url: `+site.URL) {
		t.Fatalf("squad list: %s", r.Body)
	}
	if r := e.run(a, "portal", "list"); !strings.Contains(r.Body, `- name: "Portal"`) {
		t.Fatalf("portal list: %s", r.Body)
	}

	events, unsub := e.app.Server.Bus().Subscribe()
	defer unsub()

	r := e.run(a, "portal", "snapshot")
	if r.Code != 200 || !strings.Contains(r.Body, "Page: Cadastro") || !strings.Contains(r.Body, `status "vazio"`) {
		t.Fatalf("snapshot: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "portal", "fill", ref(t, r.Body, `textbox "Nome"`), "Ana", "Maria"); r.Code != 200 || !strings.HasPrefix(r.Body, "Filled @") {
		t.Fatalf("fill: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "portal", "click", ref(t, r.Body, `button "Enviar"`)); r.Code != 200 || !strings.Contains(r.Body, `Clicked @`) {
		t.Fatalf("click: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "portal", "eval", "saida.textContent"); r.Body != "oi Ana Maria" {
		t.Fatalf("eval after fill+click: %q", r.Body)
	}
	if r := e.run(a, "portal", "console"); !strings.Contains(r.Body, "log: enviou Ana Maria") {
		t.Fatalf("console: %s", r.Body)
	}
	if r := e.run(a, "portal", "eval", "throw new Error('ruim')"); r.Code != 400 || !strings.Contains(r.Body, "ruim") {
		t.Fatalf("eval error: %d %s", r.Code, r.Body)
	}

	// The canvas hears about the clicks: who, what, where.
	deadline := time.After(5 * time.Second)
	for got := false; !got; {
		select {
		case ev := <-events:
			if act, ok := ev.Data.(portal.Action); ok && ev.Type == portal.EventAction && act.Kind == "click" {
				if act.By != a.ID || act.ByName != "A" || act.Portal != p1.ID || !act.HasXY || act.Label != `button "Enviar"` {
					t.Fatalf("action %+v", act)
				}
				got = true
			}
		case <-deadline:
			t.Fatal("no portal.action event")
		}
	}

	// Navigation is reported, and the address is saved on the canvas.
	r = e.run(a, "portal", "click", ref(t, e.run(a, "portal", "snapshot").Body, `link "Próxima"`))
	if !strings.Contains(r.Body, "The page is now "+site.URL+`/2 — "Segunda"`) {
		t.Fatalf("click link: %s", r.Body)
	}
	if n, _ := e.app.Server.Store().Node(p1.ID); n.Portal.URL != site.URL+"/2" {
		t.Fatalf("saved url %s", n.Portal.URL)
	}
	if r := e.run(a, "portal", "back"); !strings.Contains(r.Body, `"Cadastro"`) {
		t.Fatalf("back: %s", r.Body)
	}
	if r := e.run(a, "portal", "goto", strings.TrimPrefix(site.URL, "http://")+"/2"); !strings.Contains(r.Body, `"Segunda"`) {
		t.Fatalf("goto: %s", r.Body)
	}

	r = e.run(a, "portal", "screenshot")
	shot := strings.SplitN(r.Body, "\n", 2)[0]
	if png, err := os.ReadFile(shot); err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Fatalf("screenshot %q: %v", r.Body, err)
	}

	// Two portals: name one.
	e.connect(a, p2)
	if r := e.run(a, "portal", "snapshot"); r.Code != 400 || !strings.Contains(r.Body, `"Portal", "Portal 2"`) {
		t.Fatalf("ambiguous: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "portal", "Portal 2", "snapshot"); r.Code != 200 || !strings.Contains(r.Body, "Page: Segunda") {
		t.Fatalf("named: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "portal", "Nope", "snapshot"); r.Code != 404 {
		t.Fatalf("unknown name: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "portal", "Portal", "click", "@e999"); r.Code != 400 || !strings.Contains(r.Body, "new snapshot") {
		t.Fatalf("stale ref: %d %s", r.Code, r.Body)
	}
}

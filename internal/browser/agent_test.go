package browser

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

const agentPage = `<!doctype html><html><head><title>Formulário</title></head><body style="margin:0;font:16px sans-serif">
<h1>Cadastro</h1>
<label>Nome <input id="nome" value="antigo"></label>
<label><input type="checkbox" id="aceito"> Aceito os termos</label>
<button id="ok" onclick="console.log('enviado', nome.value, aceito.checked); saida.textContent = nome.value + '/' + aceito.checked">Enviar</button>
<div role="status" id="saida">aguardando</div>
<input id="atalho" placeholder="atalho" onkeydown="if (event.ctrlKey && event.key === 'k') this.value = 'ctrl+k'">
<div style="height:2000px"></div>
<a href="/fim" id="fim">Lá embaixo</a>
<script>console.warn('carregou'); setTimeout(() => { throw new Error('quebrou') }, 0)</script>
</body></html>`

// refOf finds the ref of the line matching role and name in a snapshot.
func refOf(t *testing.T, snap, role, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^(@e\d+) ` + role + ` "` + regexp.QuoteMeta(name) + `"`).FindStringSubmatch(snap)
	if m == nil {
		t.Fatalf("no %s %q in snapshot:\n%s", role, name, snap)
	}
	return m[1]
}

func TestAgentActions(t *testing.T) {
	path := Find()
	if path == "" {
		t.Skip("no Chromium-based browser (set SQUAD_BROWSER)")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(agentPage))
	}))
	defer srv.Close()
	m := New(Options{ExecPath: path, Profile: t.TempDir()})
	defer m.Shutdown()
	p, err := m.Page("agent")
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Resize(800, 600)
	_ = p.Navigate(srv.URL)
	p.Settle(10 * time.Second)

	snap, err := p.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Page: Formulário", "URL: " + srv.URL, "Viewport: 800x600", `@e1 heading "Cadastro"`, `checked=false`, `status "aguardando"`, `value="antigo"`} {
		if !strings.Contains(snap, want) {
			t.Fatalf("snapshot lacks %q:\n%s", want, snap)
		}
	}
	if !regexp.MustCompile(`link "Lá embaixo" \[[^\]]+\] offscreen`).MatchString(snap) {
		t.Fatalf("the link below the fold should be offscreen:\n%s", snap)
	}

	// Fill replaces the old value; click toggles and submits.
	if _, _, _, err := p.Fill(refOf(t, snap, "textbox", "Nome"), "Ana"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := p.Click(refOf(t, snap, "checkbox", "Aceito os termos")); err != nil {
		t.Fatal(err)
	}
	e, x, y, err := p.Click(refOf(t, snap, "button", "Enviar"))
	if err != nil || e.Label() != `button "Enviar"` || x <= 0 || y <= 0 {
		t.Fatalf("click: %v %v %v,%v", err, e.Label(), x, y)
	}
	p.Settle(time.Second)
	if got, _ := p.Evaluate(`saida.textContent`, time.Second); got != "Ana/true" {
		t.Fatalf("after fill + clicks: %q", got)
	}

	// Keys by name and combination.
	if _, _, _, err := p.Click(refOf(t, snap, "textbox", "atalho")); err != nil {
		t.Fatal(err)
	}
	if err := p.Press("Control+k"); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Evaluate(`atalho.value`, time.Second); got != "ctrl+k" {
		t.Fatalf("Control+k: %q", got)
	}
	if err := p.Press("Hyper+x"); err == nil {
		t.Fatal("unknown modifier should fail")
	}

	// Refs expire with a new snapshot of a changed page only when the page
	// navigates; an unknown ref says what to do.
	if _, _, _, err := p.Click("@e999"); err == nil || !strings.Contains(err.Error(), "new snapshot") {
		t.Fatalf("unknown ref: %v", err)
	}

	// Scroll brings the link into view.
	if err := p.Scroll(2000); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	snap2, _ := p.Snapshot()
	if regexp.MustCompile(`link "Lá embaixo" \[[^\]]+\] offscreen`).MatchString(snap2) {
		t.Fatalf("after scrolling the link should be on screen:\n%s", snap2)
	}

	png, err := p.Screenshot()
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Fatalf("screenshot: %v", err)
	}

	var levels, texts []string
	for _, c := range p.Console(0) {
		levels = append(levels, c.Level)
		texts = append(texts, c.Text)
	}
	joined := strings.Join(texts, " | ")
	if !strings.Contains(joined, "carregou") || !strings.Contains(joined, "enviado Ana true") || !strings.Contains(joined, "quebrou") {
		t.Fatalf("console: %v %v", levels, texts)
	}
}

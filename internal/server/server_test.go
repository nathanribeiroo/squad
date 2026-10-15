//go:build !windows

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

const testToken = "test-session-token"

type harness struct {
	t    *testing.T
	srv  *Server
	http *httptest.Server
	base string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ln, port, err := ListenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "workspace.json"), store.Workspace{ID: "w", Name: "test", Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{Port: port, Token: testToken, Store: st})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewUnstartedServer(srv.Handler())
	hs.Listener = ln
	hs.Start()
	t.Cleanup(func() { hs.Close(); srv.Shutdown() })
	return &harness{t: t, srv: srv, http: hs, base: fmt.Sprintf("http://127.0.0.1:%d", port)}
}

func (h *harness) do(method, path string, body any, mod func(*http.Request)) (*http.Response, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.base+path, rd)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if mod != nil {
		mod(req)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (h *harness) addTerminal(name string) store.Node {
	h.t.Helper()
	resp, data := h.do("POST", "/api/nodes", store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: name, Agent: "shell", Shell: "/bin/sh"}}, nil)
	if resp.StatusCode != http.StatusCreated {
		h.t.Fatalf("add terminal: %d %s", resp.StatusCode, data)
	}
	var n store.Node
	_ = json.Unmarshal(data, &n)
	return n
}

func TestGuardsAndSession(t *testing.T) {
	h := newHarness(t)
	if resp, _ := h.do("GET", "/api/state", nil, func(r *http.Request) { r.Header.Del("Authorization") }); resp.StatusCode != 401 {
		t.Fatalf("no auth: %d", resp.StatusCode)
	}
	if resp, _ := h.do("GET", "/api/state", nil, nil); resp.StatusCode != 200 {
		t.Fatalf("bearer: %d", resp.StatusCode)
	}
	if resp, _ := h.do("GET", "/api/state", nil, func(r *http.Request) { r.Host = "evil.example:80" }); resp.StatusCode != 403 {
		t.Fatalf("bad host: %d", resp.StatusCode)
	}
	if resp, _ := h.do("POST", "/api/nodes", map[string]string{}, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }); resp.StatusCode != 403 {
		t.Fatalf("bad origin: %d", resp.StatusCode)
	}
	if resp, _ := h.do("GET", "/?token=wrong", nil, func(r *http.Request) { r.Header.Del("Authorization") }); resp.StatusCode != 401 {
		t.Fatalf("bad url token: %d", resp.StatusCode)
	}
	resp, _ := h.do("GET", "/?token="+testToken, nil, func(r *http.Request) { r.Header.Del("Authorization") })
	if resp.StatusCode != http.StatusSeeOther || len(resp.Cookies()) == 0 {
		t.Fatalf("token login: %d cookies=%v", resp.StatusCode, resp.Cookies())
	}
	cookie := resp.Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags: %+v", cookie)
	}
	resp, body := h.do("GET", "/", nil, func(r *http.Request) { r.Header.Del("Authorization"); r.AddCookie(cookie) })
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Squad") {
		t.Fatalf("ui with cookie: %d", resp.StatusCode)
	}
}

func TestNodesEdgesAndEvents(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.base, "http", "ws", 1)+"/ws/events", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + testToken}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	var hello events.Event
	readEvent(t, ctx, c, &hello)
	if hello.Type != "hello" {
		t.Fatalf("first event %q", hello.Type)
	}

	a := h.addTerminal("Planejador")
	b := h.addTerminal("Implementador")
	resp, data := h.do("POST", "/api/edges", map[string]string{"a": a.ID, "b": b.ID}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("add edge: %d %s", resp.StatusCode, data)
	}
	if resp, _ := h.do("POST", "/api/edges", map[string]string{"a": b.ID, "b": a.ID}, nil); resp.StatusCode != 409 {
		t.Fatalf("duplicate edge: %d", resp.StatusCode)
	}
	if resp, _ := h.do("POST", "/api/nodes", store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Planejador"}}, nil); resp.StatusCode != 409 {
		t.Fatalf("duplicate name: %d", resp.StatusCode)
	}

	seen := map[string]int{}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && (seen[events.NodeAdded] < 2 || seen[events.EdgeAdded] < 1 || seen[events.TerminalState] < 2) {
		var ev events.Event
		readEvent(t, ctx, c, &ev)
		seen[ev.Type]++
	}
	if seen[events.NodeAdded] < 2 || seen[events.EdgeAdded] < 1 || seen[events.TerminalState] < 2 {
		t.Fatalf("events seen: %v", seen)
	}

	resp, data = h.do("GET", "/api/state", nil, nil)
	var st StateResponse
	_ = json.Unmarshal(data, &st)
	if resp.StatusCode != 200 || len(st.Workspace.Nodes) != 2 || st.Runtime[a.ID].Status != "running" {
		t.Fatalf("state: %d %+v", resp.StatusCode, st.Runtime)
	}

	if resp, _ := h.do("DELETE", "/api/nodes/"+a.ID, nil, nil); resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if len(h.srv.Store().Snapshot().Edges) != 0 || h.srv.Terminals().Get(a.ID) != nil {
		t.Fatal("node removal left edges or a running terminal")
	}
}

func readEvent(t *testing.T, ctx context.Context, c *websocket.Conn, v any) {
	t.Helper()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalWebSocket(t *testing.T) {
	h := newHarness(t)
	n := h.addTerminal("Shell")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.base, "http", "ws", 1)+"/ws/term/"+n.ID, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + testToken}, "Origin": {h.base}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	typ, data, err := c.Read(ctx)
	if err != nil || typ != websocket.MessageText || !strings.Contains(string(data), `"init"`) {
		t.Fatalf("init: %v %s", err, data)
	}
	if typ, _, err := c.Read(ctx); err != nil || typ != websocket.MessageBinary {
		t.Fatalf("initial render: %v", err)
	}
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","cols":100,"rows":30}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, websocket.MessageBinary, []byte("echo ws-$((6*7))\r")); err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for !strings.Contains(got.String(), "ws-42") {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v (got %q)", err, got.String())
		}
		got.Write(data)
	}
	if cols, rows := h.srv.Terminals().Get(n.ID).Screen().Size(); cols != 100 || rows != 30 {
		t.Fatalf("resize not applied: %dx%d", cols, rows)
	}
}

func TestCLIAuth(t *testing.T) {
	h := newHarness(t)
	n := h.addTerminal("Shell")
	resp, _ := h.do("POST", "/cli", map[string]any{"args": []string{"list"}}, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("cli without terminal creds: %d", resp.StatusCode)
	}
	tok := h.srv.TerminalToken(n.ID)
	resp, _ = h.do("POST", "/cli", map[string]any{"args": []string{"list"}}, func(r *http.Request) {
		r.Header.Set("X-Terminal-ID", n.ID)
		r.Header.Set("X-Squad-Token", "wrong")
	})
	if resp.StatusCode != 401 {
		t.Fatalf("wrong token: %d", resp.StatusCode)
	}
	resp, _ = h.do("POST", "/cli", map[string]any{"args": []string{"list"}}, func(r *http.Request) {
		r.Header.Set("X-Terminal-ID", n.ID)
		r.Header.Set("X-Squad-Token", tok)
	})
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("valid creds before B3: %d", resp.StatusCode)
	}
	// The terminal sees its identity in the environment.
	term := h.srv.Terminals().Get(n.ID)
	_ = term.Input([]byte("echo id=$SQUAD_TERMINAL_ID\r"))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(term.Snapshot(), "id="+n.ID) {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(term.Snapshot(), "id="+n.ID) {
		t.Fatalf("env not set:\n%s", term.Snapshot())
	}
}

func TestPatchTerminalSizeResizes(t *testing.T) {
	h := newHarness(t)
	n := h.addTerminal("sh")
	td := *n.Terminal
	td.Cols, td.Rows = 100, 30
	if resp, data := h.do("PATCH", "/api/nodes/"+n.ID, map[string]any{"w": 700, "h": 520, "terminal": td}, nil); resp.StatusCode != 200 {
		t.Fatalf("patch: %d %s", resp.StatusCode, data)
	}
	term := h.srv.terms.Get(n.ID)
	if term == nil {
		t.Fatal("terminal not running")
	}
	if c, r := term.Screen().Size(); c != 100 || r != 30 {
		t.Fatalf("size = %dx%d, want 100x30", c, r)
	}
}

// Agents address a portal by its name: a new address keeps it.
func TestPatchPortalKeepsName(t *testing.T) {
	h := newHarness(t)
	_, data := h.do("POST", "/api/nodes", store.Node{Kind: store.KindPortal, Portal: &store.PortalData{URL: "http://a", Name: "Docs"}}, nil)
	var n store.Node
	_ = json.Unmarshal(data, &n)
	for _, tc := range []struct {
		patch store.PortalData
		want  store.PortalData
	}{
		{store.PortalData{URL: "http://b"}, store.PortalData{URL: "http://b", Name: "Docs"}},
		{store.PortalData{URL: "http://b", Name: "  Loja "}, store.PortalData{URL: "http://b", Name: "Loja"}},
	} {
		resp, data := h.do("PATCH", "/api/nodes/"+n.ID, map[string]any{"portal": tc.patch}, nil)
		var got store.Node
		_ = json.Unmarshal(data, &got)
		if resp.StatusCode != 200 || got.Portal == nil || *got.Portal != tc.want {
			t.Fatalf("patch %+v: %d %s", tc.patch, resp.StatusCode, data)
		}
	}
}

//go:build !windows

package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

const testToken = "hub-test-token"

type harness struct {
	t    *testing.T
	hub  *Hub
	base string
	root string
}

func newHarness(t *testing.T, root string) *harness {
	t.Helper()
	t.Setenv("SQUAD_HOME", t.TempDir()) // agent hook files, attachments
	ln, port, err := server.ListenLoopback(0)
	if err != nil {
		t.Fatal(err)
	}
	if root == "" {
		root = t.TempDir()
	}
	h, err := New(Options{Port: port, Token: testToken, Root: root, Trash: filepath.Join(filepath.Dir(root), "trash")})
	if err != nil {
		t.Fatal(err)
	}
	h.Start()
	hs := httptest.NewUnstartedServer(h.Handler())
	hs.Listener = ln
	hs.Start()
	t.Cleanup(func() { hs.Close(); h.Close() })
	return &harness{t: t, hub: h, base: fmt.Sprintf("http://127.0.0.1:%d", port), root: root}
}

func (h *harness) do(method, path string, body any, hdr map[string]string) (int, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.base+path, rd)
	req.Header.Set("Authorization", "Bearer "+testToken)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func (h *harness) create(name string) Summary {
	h.t.Helper()
	code, data := h.do("POST", "/api/workspaces", map[string]string{"name": name, "cwd": h.t.TempDir()}, nil)
	if code != http.StatusCreated {
		h.t.Fatalf("create %s: %d %s", name, code, data)
	}
	var s Summary
	_ = json.Unmarshal(data, &s)
	return s
}

func (h *harness) addTerminal(ws, name string) store.Node {
	h.t.Helper()
	code, data := h.do("POST", "/w/"+ws+"/api/nodes", store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: name, Agent: "shell", Shell: "/bin/sh"}}, nil)
	if code != http.StatusCreated {
		h.t.Fatalf("add terminal: %d %s", code, data)
	}
	var n store.Node
	_ = json.Unmarshal(data, &n)
	return n
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestCreateOpenUpdateRemove(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	h := newHarness(t, root)
	if len(h.hub.List()) != 0 {
		t.Fatal("new root should have no workspaces")
	}
	dirA, dirB := t.TempDir(), t.TempDir()
	a, err := h.hub.Open(dirA)
	if err != nil || a.Name != filepath.Base(dirA) || a.Color != Colors[0] {
		t.Fatalf("open A: %+v %v", a, err)
	}
	if again, _ := h.hub.Open(dirA + "/"); again.ID != a.ID {
		t.Fatalf("open A twice made a new workspace: %s vs %s", again.ID, a.ID)
	}
	b, _ := h.hub.Create("Projeto B", dirB, "")
	if b.Color != Colors[1] || b.Name != "Projeto B" {
		t.Fatalf("B: %+v", b)
	}
	if l := h.hub.List(); len(l) != 2 || l[0].ID != a.ID || l[1].ID != b.ID {
		t.Fatalf("list order: %+v", l)
	}
	if _, err := h.hub.Create("", filepath.Join(dirA, "missing"), ""); !errors.Is(err, ErrInvalid) || !strings.HasPrefix(err.Error(), "folder not found") {
		t.Fatalf("missing folder: %v", err)
	}

	// Update: name, color, folder; bad values are refused.
	name, color, cwd := "Renomeado", "pink", t.TempDir()
	u, err := h.hub.Update(a.ID, Patch{Name: &name, Color: &color, Cwd: &cwd})
	if err != nil || u.Name != name || u.Color != color || u.Cwd != cwd {
		t.Fatalf("update: %+v %v", u, err)
	}
	bad := "chartreuse"
	if _, err := h.hub.Update(a.ID, Patch{Color: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad color: %v", err)
	}
	if code, _ := h.do("PATCH", "/api/workspaces/"+a.ID, map[string]string{"name": " "}, nil); code != 400 {
		t.Fatalf("empty name: %d", code)
	}
	if code, _ := h.do("PATCH", "/api/workspaces/nope", map[string]string{"name": "x"}, nil); code != 404 {
		t.Fatalf("unknown workspace: %d", code)
	}

	// Remove moves the files to the trash; the last one stays.
	if code, data := h.do("DELETE", "/api/workspaces/"+b.ID, nil, nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", code, data)
	}
	trash, _ := os.ReadDir(filepath.Join(filepath.Dir(root), "trash"))
	if len(trash) != 1 || !strings.HasPrefix(trash[0].Name(), b.ID+"-") {
		t.Fatalf("trash: %v", trash)
	}
	if _, err := os.Stat(filepath.Join(root, b.ID)); !os.IsNotExist(err) {
		t.Fatalf("B's folder still in root: %v", err)
	}
	if code, _ := h.do("DELETE", "/api/workspaces/"+a.ID, nil, nil); code != 400 {
		t.Fatalf("removing the last workspace: %d", code)
	}

	// A new hub on the same root loads what is left.
	h.hub.Close()
	h2 := newHarness(t, root)
	if l := h2.hub.List(); len(l) != 1 || l[0].ID != a.ID || l[0].Name != name || l[0].Cwd != cwd {
		t.Fatalf("reloaded: %+v", l)
	}
}

func TestLegacyWorkspaceGetsAColor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	_ = os.MkdirAll(filepath.Join(root, "default"), 0o700)
	if _, err := store.Open(filepath.Join(root, "default", "workspace.json"), store.Workspace{ID: "default", Name: "old", Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, root)
	b, _ := h.hub.Create("new", t.TempDir(), "")
	l := h.hub.List()
	if len(l) != 2 || l[0].ID != "default" || l[0].Color != Colors[0] || b.Color != Colors[1] {
		t.Fatalf("colors: %+v", l)
	}
}

func TestRoutingByWorkspaceAndTerminal(t *testing.T) {
	h := newHarness(t, "")
	a, b := h.create("A"), h.create("B")
	shellA, otherA := h.addTerminal(a.ID, "Shell"), h.addTerminal(a.ID, "Other")
	otherB := h.addTerminal(b.ID, "Other")
	if code, data := h.do("POST", "/w/"+a.ID+"/api/edges", map[string]string{"a": shellA.ID, "b": otherA.ID}, nil); code != http.StatusCreated {
		t.Fatalf("edge: %d %s", code, data)
	}

	// Each workspace's API only sees its own canvas.
	var st server.StateResponse
	_, data := h.do("GET", "/w/"+b.ID+"/api/state", nil, nil)
	_ = json.Unmarshal(data, &st)
	if len(st.Workspace.Nodes) != 1 || st.Workspace.Nodes[0].ID != otherB.ID {
		t.Fatalf("B's state: %s", data)
	}
	if code, _ := h.do("GET", "/w/nope/api/state", nil, nil); code != 404 {
		t.Fatalf("unknown workspace: %d", code)
	}
	if code, data := h.do("GET", "/api/state", nil, nil); code != 404 || !strings.Contains(string(data), "/w/<id>/api/") {
		t.Fatalf("old path: %d %s", code, data)
	}
	if code, _ := h.do("GET", "/w/"+a.ID+"/api/state", nil, map[string]string{"Authorization": "Bearer wrong"}); code != 401 {
		t.Fatalf("no session: %d", code)
	}

	// A CLI call from A's Shell reaches A's bus: "Other" is A's terminal.
	for _, n := range []store.Node{otherA, otherB} {
		marker := "screen-of-" + n.ID[:4]
		_ = h.hub.App(map[string]string{otherA.ID: a.ID, otherB.ID: b.ID}[n.ID]).Server.Terminals().Get(n.ID).Input([]byte("echo " + marker + "\r"))
	}
	tok := h.hub.App(a.ID).Server.TerminalToken(shellA.ID)
	cli := map[string]string{"X-Terminal-ID": shellA.ID, "X-Squad-Token": tok}
	waitFor(t, "check Other from A", func() bool {
		code, data := h.do("POST", "/cli", map[string]any{"args": []string{"check", "Other"}}, cli)
		return code == 200 && strings.Contains(string(data), "screen-of-"+otherA.ID[:4]) && !strings.Contains(string(data), "screen-of-"+otherB.ID[:4])
	})
	if code, _ := h.do("POST", "/cli", map[string]any{"args": []string{"list"}}, map[string]string{"X-Terminal-ID": shellA.ID, "X-Squad-Token": "wrong"}); code != 401 {
		t.Fatalf("wrong terminal token: %d", code)
	}
	if code, _ := h.do("POST", "/cli", map[string]any{"args": []string{"list"}}, map[string]string{"X-Terminal-ID": "nope"}); code != 401 {
		t.Fatalf("unknown terminal: %d", code)
	}

	// Removing a workspace stops its terminals.
	termB := h.hub.App(b.ID).Server.Terminals()
	if code, _ := h.do("DELETE", "/api/workspaces/"+b.ID, nil, nil); code != http.StatusNoContent {
		t.Fatalf("remove B: %d", code)
	}
	if termB.Get(otherB.ID) != nil && func() bool { st, _ := termB.Get(otherB.ID).State(); return st == "running" }() {
		t.Fatal("B's terminal still running")
	}
}

func TestWorkspacesStreamAndSummary(t *testing.T) {
	h := newHarness(t, "")
	a := h.create("A")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.base, "http", "ws", 1)+"/ws/workspaces", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + testToken}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	// next returns the next event of a type, skipping the others.
	next := func(typ string) json.RawMessage {
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var ev struct {
				Type string
				Data json.RawMessage
			}
			_ = json.Unmarshal(data, &ev)
			if ev.Type == typ {
				return ev.Data
			}
		}
	}
	read := func() []Summary {
		var l []Summary
		_ = json.Unmarshal(next(EventWorkspaces), &l)
		return l
	}
	readRoles := func() []roles.Role {
		var l []roles.Role
		_ = json.Unmarshal(next(EventRoles), &l)
		return l
	}
	if l := read(); len(l) != 1 || l[0].ID != a.ID {
		t.Fatalf("first list: %+v", l)
	}
	if l := readRoles(); len(l) != 4 || l[3].Name != "Testador" {
		t.Fatalf("first roles: %+v", l)
	}
	// A new role reaches every open page.
	if code, data := h.do("POST", "/api/roles", roles.Role{Name: "Designer", Prompt: "Cuide do visual."}, nil); code != http.StatusCreated {
		t.Fatalf("create role: %d %s", code, data)
	}
	if l := readRoles(); len(l) != 5 || l[4].ID != "designer" {
		t.Fatalf("roles after create: %+v", l)
	}
	h.addTerminal(a.ID, "Shell")
	for {
		if l := read(); len(l) == 1 && l[0].Terminals == 1 {
			break
		}
	}
	b := h.create("B")
	for {
		if l := read(); len(l) == 2 && l[1].ID == b.ID {
			break
		}
	}
}

func TestRolesAPI(t *testing.T) {
	h := newHarness(t, "")
	a := h.create("A")
	role := func(method, path string, body any, want int) roles.Role {
		t.Helper()
		code, data := h.do(method, path, body, nil)
		if code != want {
			t.Fatalf("%s %s: %d %s", method, path, code, data)
		}
		var r roles.Role
		_ = json.Unmarshal(data, &r)
		return r
	}
	ux := role("POST", "/api/roles", roles.Role{Name: "Revisão de UX", Prompt: "Olhe a interface.", Color: "teal", Icon: "palette"}, http.StatusCreated)
	role("POST", "/api/roles", roles.Role{Name: "revisão de ux"}, http.StatusBadRequest)
	ux.Prompt = "Olhe a interface com calma."
	if r := role("PUT", "/api/roles/"+ux.ID, ux, http.StatusOK); r.Prompt != ux.Prompt {
		t.Fatalf("update: %+v", r)
	}
	test := role("PUT", "/api/roles/test", roles.Role{Name: "QA", Prompt: "Teste.", Color: "test", Icon: "science"}, http.StatusOK)
	if !test.Edited || test.Name != "QA" {
		t.Fatalf("edit built-in: %+v", test)
	}
	if r := role("POST", "/api/roles/test/reset", nil, http.StatusOK); r.Name != "Testador" || r.Edited {
		t.Fatalf("reset: %+v", r)
	}
	role("DELETE", "/api/roles/test", nil, http.StatusBadRequest)
	role("PUT", "/api/roles/nope", ux, http.StatusNotFound)

	// A role in use can't be removed: its terminals would lose it silently.
	n := h.addTerminal(a.ID, "Ana")
	n.Terminal.Role = ux.ID
	if code, data := h.do("PATCH", "/w/"+a.ID+"/api/nodes/"+n.ID, map[string]any{"terminal": n.Terminal}, nil); code != http.StatusOK {
		t.Fatalf("assign: %d %s", code, data)
	}
	if code, data := h.do("DELETE", "/api/roles/"+ux.ID, nil, nil); code != http.StatusConflict || !strings.Contains(string(data), "Ana (A)") {
		t.Fatalf("delete in use: %d %s", code, data)
	}
	n.Terminal.Role = ""
	h.do("PATCH", "/w/"+a.ID+"/api/nodes/"+n.ID, map[string]any{"terminal": n.Terminal}, nil)
	role("DELETE", "/api/roles/"+ux.ID, nil, http.StatusNoContent)
	var list []roles.Role
	_, data := h.do("GET", "/api/roles", nil, nil)
	_ = json.Unmarshal(data, &list)
	if len(list) != 4 {
		t.Fatalf("list: %+v", list)
	}
}

func TestTokenRedirectKeepsWorkspace(t *testing.T) {
	h := newHarness(t, "")
	code, _ := h.do("GET", "/?token="+testToken+"&ws=abc", nil, map[string]string{"Authorization": ""})
	if code != http.StatusSeeOther {
		t.Fatalf("token login: %d", code)
	}
	req, _ := http.NewRequest("GET", h.base+"/?token="+testToken+"&ws=abc", nil)
	resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/?ws=abc" {
		t.Fatalf("redirect to %q", loc)
	}
}

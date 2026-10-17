//go:build !windows

package hub

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/server"
	"github.com/nathanribeiroo/squad/internal/store"
)

func (h *harness) sessions(ws string) SessionList {
	h.t.Helper()
	code, data := h.do("GET", "/api/workspaces/"+ws+"/sessions", nil, nil)
	if code != 200 {
		h.t.Fatalf("sessions: %d %s", code, data)
	}
	var l SessionList
	_ = json.Unmarshal(data, &l)
	return l
}

func (h *harness) state(ws string) server.StateResponse {
	h.t.Helper()
	var st server.StateResponse
	_, data := h.do("GET", "/w/"+ws+"/api/state", nil, nil)
	_ = json.Unmarshal(data, &st)
	return st
}

func (h *harness) node(ws string, kind string, body any) store.Node {
	h.t.Helper()
	code, data := h.do("POST", "/w/"+ws+"/api/nodes", body, nil)
	if code != http.StatusCreated {
		h.t.Fatalf("add %s: %d %s", kind, code, data)
	}
	var n store.Node
	_ = json.Unmarshal(data, &n)
	return n
}

func waitForLong(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (l SessionList) current() SessionSummary {
	for _, s := range l.Sessions {
		if s.Current {
			return s
		}
	}
	return SessionSummary{}
}

// A session is a whole canvas: a new one starts empty, and opening the old
// one brings back its terminals, notes (with their content), connections and
// positions, with its terminals running again.
func TestSessionsKeepWholeCanvases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	h := newHarness(t, root)
	ws := h.create("Xadrez")
	first := h.sessions(ws.ID)
	if len(first.Sessions) != 1 || !strings.HasPrefix(first.current().Name, "Nova sessão · ") || first.current().Named {
		t.Fatalf("first session: %+v", first)
	}
	term := h.addTerminal(ws.ID, "Plano")
	term.X, term.Y = 120, 80
	if code, data := h.do("PATCH", "/w/"+ws.ID+"/api/nodes/"+term.ID, map[string]float64{"x": 120, "y": 80}, nil); code != 200 {
		t.Fatalf("move: %d %s", code, data)
	}
	note := h.node(ws.ID, "note", store.Node{Kind: store.KindNote, Note: &store.NoteData{Title: "Plano de jogo"}})
	if code, _ := h.do("PUT", "/w/"+ws.ID+"/api/notes/"+note.ID, map[string]string{"content": "- [ ] tabuleiro"}, nil); code != 200 {
		t.Fatal("write note")
	}
	if code, _ := h.do("POST", "/w/"+ws.ID+"/api/edges", map[string]string{"a": term.ID, "b": note.ID}, nil); code != 201 {
		t.Fatal("edge")
	}
	oldTerms := h.hub.App(ws.ID).Server.Terminals()
	if l := h.sessions(ws.ID); l.current().Terminals != 1 || l.current().Notes != 1 {
		t.Fatalf("counts: %+v", l.current())
	}

	// A new session: an empty canvas; the old one's processes stopped.
	code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions", NewSession{Name: "Peças"}, nil)
	if code != http.StatusCreated {
		t.Fatalf("new session: %d %s", code, data)
	}
	l := h.sessions(ws.ID)
	if len(l.Sessions) != 2 || l.current().Name != "Peças" || !l.current().Named || l.Sessions[0].ID != l.Current {
		t.Fatalf("after new: %+v", l)
	}
	if st := h.state(ws.ID); len(st.Workspace.Nodes) != 0 || st.Workspace.Name != "Xadrez" {
		t.Fatalf("new canvas: %+v", st.Workspace)
	}
	if tm := oldTerms.Get(term.ID); tm != nil {
		if s, _ := tm.State(); s == "running" {
			t.Fatal("the old session's terminal still runs")
		}
	}
	h.addTerminal(ws.ID, "Bia")

	// Back to the first one, as it was.
	if code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions/"+first.Current+"/open", nil, nil); code != 200 {
		t.Fatalf("open: %d %s", code, data)
	}
	st := h.state(ws.ID)
	if len(st.Workspace.Nodes) != 2 || len(st.Workspace.Edges) != 1 {
		t.Fatalf("restored canvas: %+v", st.Workspace)
	}
	for _, n := range st.Workspace.Nodes {
		if n.ID == term.ID && (n.X != 120 || n.Y != 80) {
			t.Fatalf("position: %+v", n)
		}
	}
	if _, data := h.do("GET", "/w/"+ws.ID+"/api/notes/"+note.ID, nil, nil); !strings.Contains(string(data), "tabuleiro") {
		t.Fatalf("note content: %s", data)
	}
	waitFor(t, "the terminal running again", func() bool {
		return st.Runtime[term.ID].Status == "running" || h.state(ws.ID).Runtime[term.ID].Status == "running"
	})

	// Renamed, the name stays; the open session and unknown ones can't go.
	if code, _ := h.do("PATCH", "/api/workspaces/"+ws.ID+"/sessions/"+first.Current, map[string]string{"name": "  Regras   do jogo "}, nil); code != 200 {
		t.Fatal("rename")
	}
	if code, _ := h.do("PATCH", "/api/workspaces/"+ws.ID+"/sessions/"+first.Current, map[string]string{"name": " "}, nil); code != 400 {
		t.Fatal("empty name")
	}
	if code, _ := h.do("DELETE", "/api/workspaces/"+ws.ID+"/sessions/"+first.Current, nil, nil); code != 400 {
		t.Fatal("removed the open session")
	}
	if code, _ := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions/nope/open", nil, nil); code != 404 {
		t.Fatal("opened an unknown session")
	}

	// The workspace's name follows every session.
	name := "Xadrez 2"
	if _, err := h.hub.Update(ws.ID, Patch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	other := ""
	for _, s := range h.sessions(ws.ID).Sessions {
		if !s.Current {
			other = s.ID
		}
	}
	if code, _ := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions/"+other+"/open", nil, nil); code != 200 || h.state(ws.ID).Workspace.Name != "Xadrez 2" {
		t.Fatal("the name didn't follow")
	}

	// Removing a session moves its folder to the trash.
	if code, _ := h.do("DELETE", "/api/workspaces/"+ws.ID+"/sessions/"+first.Current, nil, nil); code != 200 {
		t.Fatal("remove")
	}
	if l := h.sessions(ws.ID); len(l.Sessions) != 1 || l.Current != other {
		t.Fatalf("after remove: %+v", l)
	}
	trash, _ := os.ReadDir(filepath.Join(filepath.Dir(root), "trash"))
	if len(trash) != 1 || !strings.HasPrefix(trash[0].Name(), ws.ID+"-"+first.Current+"-") {
		t.Fatalf("trash: %v", trash)
	}

	// A new hub on the same root opens the same session.
	h.hub.Close()
	h2 := newHarness(t, root)
	if l := h2.sessions(ws.ID); l.Current != other || l.current().Name != "Peças" {
		t.Fatalf("reloaded: %+v", l)
	}
	if s, _ := h2.hub.Get(ws.ID); s.Session.Name != "Peças" || s.Sessions != 1 {
		t.Fatalf("summary: %+v", s)
	}
}

// A new session can start with the open one's terminals, connections and
// roles, without their conversations.
func TestNewSessionWithSameTerminals(t *testing.T) {
	h := newHarness(t, "")
	ws := h.create("A")
	a, b := h.addTerminal(ws.ID, "Plano"), h.addTerminal(ws.ID, "Bia")
	note := h.node(ws.ID, "note", store.Node{Kind: store.KindNote, Note: &store.NoteData{Title: "N"}})
	h.do("POST", "/w/"+ws.ID+"/api/edges", map[string]string{"a": a.ID, "b": b.ID}, nil)
	h.do("POST", "/w/"+ws.ID+"/api/edges", map[string]string{"a": a.ID, "b": note.ID}, nil)
	if _, err := h.hub.App(ws.ID).Server.Store().UpdateNode(a.ID, func(n *store.Node) error {
		n.Terminal.Conversation, n.Terminal.Role = "c-1", "plan"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions", NewSession{KeepTerminals: true}, nil); code != 201 {
		t.Fatalf("new: %d %s", code, data)
	}
	st := h.state(ws.ID)
	var names []string
	for _, n := range st.Workspace.Nodes {
		if n.Terminal == nil || n.ID == a.ID || n.ID == b.ID || n.Terminal.Conversation == "c-1" {
			t.Fatalf("copied node: %+v", n)
		}
		names = append(names, n.Terminal.Name)
		if n.Terminal.Name == "Plano" && n.Terminal.Role != "plan" {
			t.Fatalf("role: %+v", n.Terminal)
		}
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "Bia,Plano" || len(st.Workspace.Edges) != 1 {
		t.Fatalf("copy: %v %+v", names, st.Workspace.Edges)
	}
}

// Leaving a session whose agents work needs a confirmation.
func TestLeavingWorkingAgentsNeedsConfirm(t *testing.T) {
	h := newHarness(t, "")
	ws := h.create("A")
	cmd := `i=0; while true; do i=$((i+1)); echo "$i $(printf '%0600d' $i)"; sleep 0.2; done`
	n := h.node(ws.ID, "terminal", store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Gem", Agent: "gemini", Command: cmd, Shell: "/bin/sh"}})
	waitFor(t, "the agent working", func() bool { return h.hub.App(ws.ID).Monitor.InTurn(n.ID) })
	code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions", NewSession{}, nil)
	var busy BusyError
	_ = json.Unmarshal(data, &busy)
	if code != http.StatusConflict || strings.Join(busy.Working, ",") != "Gem" || len(h.sessions(ws.ID).Sessions) != 1 {
		t.Fatalf("not confirmed: %d %s", code, data)
	}
	if code, data := h.do("POST", "/api/workspaces/"+ws.ID+"/sessions", NewSession{Confirm: true}, nil); code != 201 {
		t.Fatalf("confirmed: %d %s", code, data)
	}
}

// A workspace from before sessions becomes one with a single session that
// holds its canvas and notes.
func TestWorkspaceFromBeforeSessions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	dir := filepath.Join(root, "old1")
	_ = os.MkdirAll(filepath.Join(dir, "notes"), 0o700)
	if err := os.WriteFile(filepath.Join(dir, "notes", "plano.md"), []byte("conteúdo antigo"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "workspace.json"), store.Workspace{ID: "old1", Name: "Antigo", Cwd: t.TempDir(),
		Nodes: []store.Node{{ID: "n1", Kind: store.KindNote, Note: &store.NoteData{Title: "Plano", File: "plano.md"}}}})
	if err != nil || st == nil {
		t.Fatal(err)
	}
	h := newHarness(t, root)
	l := h.sessions("old1")
	if len(l.Sessions) != 1 || l.current().Name != "Sessão 1" || l.current().Notes != 1 {
		t.Fatalf("migrated: %+v", l)
	}
	for _, gone := range []string{"workspace.json", "notes"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s left in the workspace folder", gone)
		}
	}
	if _, data := h.do("GET", "/w/old1/api/notes/n1", nil, nil); !strings.Contains(string(data), "conteúdo antigo") {
		t.Fatalf("note after the migration: %s", data)
	}
}

// Nobody named the session: the first turn of an agent names it (here from
// its prompt: summaries are off, so no agent CLI writes a title).
func TestSessionNamedByFirstTurn(t *testing.T) {
	h := newHarness(t, "")
	ws := h.create("A")
	if code, _ := h.do("PUT", "/w/"+ws.ID+"/api/summary/settings", map[string]any{"cards": true, "engine": "off"}, nil); code != 200 {
		t.Fatal("summary settings")
	}
	cmd := `while read l; do echo "ok"; for i in $(seq 1 25); do echo "$i $(printf '%0600d' $i)"; sleep 0.2; done; done`
	n := h.node(ws.ID, "terminal", store.Node{Kind: store.KindTerminal, Terminal: &store.TerminalData{Name: "Gem", Agent: "gemini", Command: cmd, Shell: "/bin/sh"}})
	waitFor(t, "the terminal", func() bool { return h.state(ws.ID).Runtime[n.ID].Status == "running" })
	if code, data := h.do("POST", "/w/"+ws.ID+"/api/nodes/"+n.ID+"/send", map[string]any{"text": "Implementar o movimento do cavalo no tabuleiro de xadrez"}, nil); code != 200 {
		t.Fatalf("send: %d %s", code, data)
	}
	waitForLong(t, "the session's name", func() bool { return h.sessions(ws.ID).current().Named })
	if got := h.sessions(ws.ID).current().Name; got != "Implementar o movimento do cavalo no tabuleiro" {
		t.Fatalf("name: %q", got)
	}
}

//go:build !windows

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

func TestNotesCommands(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")

	// create connects the note to the caller and writes the file.
	r := e.run(a, "note", "create", "Plano: exportar CSV", "# Plano\n- [ ] parser\n- [ ] testes")
	if r.Code != 200 || !strings.Contains(r.Body, "plano-exportar-csv.md") {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	plan, ok := e.app.Server.Store().TerminalByName("A")
	if !ok || len(e.app.Server.Store().Neighbors(plan.ID)) != 1 {
		t.Fatal("note not connected to A")
	}
	if r := e.run(a, "note", "list"); !strings.Contains(r.Body, `"Plano: exportar CSV"`) {
		t.Fatalf("list: %s", r.Body)
	}
	r = e.run(a, "note", "read", "Plano: exportar CSV")
	if r.Code != 200 || !strings.HasPrefix(r.Body, "[3 lines total]\n1\t# Plano\n2\t- [ ] parser") {
		t.Fatalf("read: %q", r.Body)
	}
	if r := e.run(a, "note", "read", "plano-exportar-csv", "2", "1"); r.Body != "[3 lines total]\n2\t- [ ] parser\n" {
		t.Fatalf("read range: %q", r.Body)
	}
	if r := e.run(a, "note", "edit", "Plano: exportar CSV", "- [ ] parser", "- [x] parser"); r.Code != 200 {
		t.Fatalf("edit: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "note", "edit", "Plano: exportar CSV", "- [", "x"); r.Code != 409 || !strings.Contains(r.Body, "appears 2 times") {
		t.Fatalf("ambiguous edit: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "note", "edit", "Plano: exportar CSV", "nada disso", "x"); r.Code != 409 {
		t.Fatalf("missing old text: %d %s", r.Code, r.Body)
	}
	content, _ := os.ReadFile(filepath.Join(e.app.Notes.Dir(), "plano-exportar-csv.md"))
	if !strings.Contains(string(content), "- [x] parser") {
		t.Fatalf("file content:\n%s", content)
	}

	// B is not connected to the note.
	if r := e.run(b, "note", "read", "Plano: exportar CSV"); r.Code != 404 {
		t.Fatalf("unconnected read: %d %s", r.Code, r.Body)
	}

	// Locked notes refuse writes.
	note, _ := findNote(e, "Plano: exportar CSV")
	if _, err := e.app.Server.Store().UpdateNode(note.ID, func(n *store.Node) error { n.Note.Locked = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if r := e.run(a, "note", "write", "Plano: exportar CSV", "x"); r.Code != 423 {
		t.Fatalf("locked write: %d %s", r.Code, r.Body)
	}
}

func findNote(e *env, title string) (store.Node, bool) {
	for _, n := range e.app.Server.Store().Snapshot().Nodes {
		if n.Note != nil && n.Note.Title == title {
			return n, true
		}
	}
	return store.Node{}, false
}

func TestNoteChainsAndWatcher(t *testing.T) {
	e := newEnv(t, fastLimits)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.app.Notes.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	a := e.agent("A")
	st := e.app.Server.Store()
	n1 := store.Node{Kind: store.KindNote, Note: &store.NoteData{Title: "Um"}}
	n2 := store.Node{Kind: store.KindNote, Note: &store.NoteData{Title: "Dois"}}
	for _, n := range []*store.Node{&n1, &n2} {
		if err := e.app.Notes.PrepareNode(n); err != nil {
			t.Fatal(err)
		}
		added, err := st.AddNode(*n)
		if err != nil {
			t.Fatal(err)
		}
		*n = added
	}
	e.connect(a, n1)
	if _, err := st.AddEdge(n1.ID, n2.ID, nil); err != nil {
		t.Fatal(err)
	}
	// Chained note is reachable through the first one.
	if r := e.run(a, "note", "write", "Dois", "via cadeia"); r.Code != 200 {
		t.Fatalf("chained write: %d %s", r.Code, r.Body)
	}

	ch, unsub := e.app.Server.Bus().Subscribe()
	defer unsub()
	time.Sleep(1200 * time.Millisecond) // past the self-write window
	if err := os.WriteFile(filepath.Join(e.app.Notes.Dir(), n1.Note.File), []byte("editado fora"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == events.NoteEdited {
				d := ev.Data.(map[string]any)
				if d["id"] == n1.ID && d["author"] == "external" {
					if r := e.run(a, "note", "read", "Um"); !strings.Contains(r.Body, "editado fora") {
						t.Fatalf("read after external edit: %s", r.Body)
					}
					return
				}
			}
		case <-deadline:
			t.Fatal("no note.edited for external change")
		}
	}
}

func TestNoteWriteFromRealCLI(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	e.connect(a, b)
	if r := e.run(b, "note", "create", "Relatorio"); r.Code != 200 {
		t.Fatalf("create: %s", r.Body)
	}
	r := e.run(a, "ask", "B", "RUN:torre note write Relatorio 'escrito pelo B'")
	if r.Code != 200 {
		t.Fatalf("ask: %d %s", r.Code, r.Body)
	}
	content, _ := os.ReadFile(filepath.Join(e.app.Notes.Dir(), "relatorio.md"))
	if string(content) != "escrito pelo B" {
		t.Fatalf("content %q; B screen:\n%s", content, r.Body)
	}
}

func TestComposerSend(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	b := e.agent("B")
	c := e.agent("C")
	e.connect(a, b)
	note, err := e.app.Server.Store().AddNode(store.Node{Kind: store.KindNote, Note: &store.NoteData{Title: "Plano", File: "plano.md"}})
	if err != nil {
		t.Fatal(err)
	}
	e.connect(a, note)

	ch, cancel := e.app.Server.Bus().Subscribe()
	defer cancel()
	png := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	err = e.app.Send(a.ID, ComposeRequest{Text: "Revise com @B usando @Plano", Mentions: []string{b.ID, note.ID}, Images: []ComposeImage{{Name: "shot.png", Data: png}}})
	if err != nil {
		t.Fatal(err)
	}
	e.waitScreen(a.ID, "received: Revise com @B", 5*time.Second)
	msg := lastSubmit(t, e.log("A"))
	for _, want := range []string{"Revise com @B usando @Plano\n\n", "/attachments/", `[menções: @B = terminal ligado (torre ask "B" "…") · @Plano = nota ligada plano.md (torre note read "plano.md")]`} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	files, _ := filepath.Glob(filepath.Join(e.dir, "attachments", "*.png"))
	if len(files) != 1 {
		t.Errorf("attachments = %v", files)
	}
	deadline := time.After(2 * time.Second)
	for got := false; !got; {
		select {
		case ev := <-ch:
			got = ev.Type == events.MessageSent
		case <-deadline:
			t.Fatal("no message.sent event")
		}
	}

	// Only connected nodes can be mentioned; an empty message is refused.
	if err := e.app.Send(a.ID, ComposeRequest{Text: "oi @C", Mentions: []string{c.ID}}); !errors.Is(err, errNotConnected) {
		t.Errorf("unconnected mention: %v", err)
	}
	if err := e.app.Send(a.ID, ComposeRequest{Text: "  "}); !errors.Is(err, errEmptyMessage) {
		t.Errorf("empty: %v", err)
	}
	if err := e.app.Send(a.ID, ComposeRequest{Text: "x", Images: []ComposeImage{{Data: "data:text/plain;base64,eA=="}}}); !errors.Is(err, errBadAttachment) {
		t.Errorf("bad image: %v", err)
	}
}

// lastSubmit returns the last message the fakeagent submitted (its log
// records each one as SUBMIT "<quoted>").
func lastSubmit(t *testing.T, log string) string {
	t.Helper()
	i := strings.LastIndex(log, "SUBMIT ")
	if i < 0 {
		t.Fatalf("no SUBMIT in log:\n%s", log)
	}
	line, _, _ := strings.Cut(log[i+len("SUBMIT "):], "\n")
	msg, err := strconv.Unquote(line)
	if err != nil {
		t.Fatalf("unquote %q: %v", line, err)
	}
	return msg
}

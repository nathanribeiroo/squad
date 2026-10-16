//go:build !windows

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/store"
)

// A note made in the UI is a draft until named; the name the user confirms
// names the file. Renaming later keeps the file and the old name, so an
// agent that knew it still finds the note. Agents name their notes once.
func TestNoteNames(t *testing.T) {
	e := newEnv(t, fastLimits)
	a := e.agent("A")
	node := func(id string) store.Node {
		n, _ := e.app.Server.Store().Node(id)
		return n
	}
	draft := func() store.Node {
		t.Helper()
		code, body := e.api("POST", "/api/nodes", `{"kind":"note","x":0,"y":0,"note":{"title":"","file":"","draft":true}}`)
		if code != 201 && code != 200 {
			t.Fatalf("new note: %d %v", code, body)
		}
		return node(body["id"].(string))
	}
	exists := func(file string) bool {
		_, err := os.Stat(filepath.Join(e.app.Notes.Dir(), file))
		return err == nil
	}

	// Drafts: free default names, no file yet.
	d1, d2 := draft(), draft()
	if d1.Note.Title != "Nota" || d2.Note.Title != "Nota 2" || !d1.Note.Draft || d1.Note.File != "" || exists("nota.md") {
		t.Fatalf("drafts: %+v %+v", d1.Note, d2.Note)
	}

	// The first name names the file; a draft's provisional name is no alias.
	if code, body := e.api("PATCH", "/api/notes/"+d1.ID, `{"title":"  Plano de testes "}`); code != 200 || body["note"].(map[string]any)["file"] != "plano-de-testes.md" {
		t.Fatalf("name: %d %v", code, body)
	}
	if n := node(d1.ID); n.Note.Draft || n.Note.Title != "Plano de testes" || len(n.Note.Aliases) != 0 || !exists("plano-de-testes.md") {
		t.Fatalf("named: %+v", n.Note)
	}

	// Names are unique, ignoring case; invalid ones are refused.
	for body, want := range map[string]int{
		`{"title":"plano DE testes"}`: 409, `{"title":"  "}`: 400, `{"title":"-x"}`: 400,
		`{"title":"` + strings.Repeat("a", 61) + `"}`: 400, `{"title":"a\nb"}`: 400,
	} {
		if code, _ := e.api("PATCH", "/api/notes/"+d2.ID, body); code != want {
			t.Fatalf("PATCH %s: %d, want %d", body, code, want)
		}
	}

	// An agent finds it ignoring case.
	e.connect(a, d1)
	if r := e.run(a, "note", "write", "plano de testes", "- [ ] login"); r.Code != 200 {
		t.Fatalf("write by name: %d %s", r.Code, r.Body)
	}

	// Renaming keeps the file; the old name still works and says the new one.
	if code, body := e.api("PATCH", "/api/notes/"+d1.ID, `{"title":"Roteiro"}`); code != 200 {
		t.Fatalf("rename: %d %v", code, body)
	}
	if n := node(d1.ID); n.Note.File != "plano-de-testes.md" || strings.Join(n.Note.Aliases, ",") != "Plano de testes" {
		t.Fatalf("renamed: %+v", n.Note)
	}
	r := e.run(a, "note", "read", "Plano de testes")
	if r.Code != 200 || !strings.HasPrefix(r.Body, `(this note is now named "Roteiro"`) || !strings.Contains(r.Body, "- [ ] login") {
		t.Fatalf("read by the old name: %d %q", r.Code, r.Body)
	}
	if r := e.run(a, "note", "edit", "plano de testes", "login", "cadastro"); r.Code != 200 || !strings.Contains(r.Body, `now named "Roteiro"`) {
		t.Fatalf("edit by the old name: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "note", "list"); !strings.Contains(r.Body, `name: "Roteiro", file: plano-de-testes.md, formerly: "Plano de testes"`) {
		t.Fatalf("note list: %s", r.Body)
	}
	if r := e.run(a, "list"); !strings.Contains(r.Body, `formerly: "Plano de testes"`) {
		t.Fatalf("squad list: %s", r.Body)
	}

	// A former name stays taken; renaming back to it is fine.
	if code, _ := e.api("PATCH", "/api/notes/"+d2.ID, `{"title":"Plano de testes"}`); code != 409 {
		t.Fatalf("an alias was reused: %d", code)
	}
	if code, _ := e.api("PATCH", "/api/notes/"+d1.ID, `{"title":"Plano de testes"}`); code != 200 {
		t.Fatalf("rename back: %d", code)
	}
	if n := node(d1.ID); strings.Join(n.Note.Aliases, ",") != "Roteiro" {
		t.Fatalf("aliases after renaming back: %v", n.Note.Aliases)
	}

	// Agents name their notes once: unique names, and the user can't rename them.
	if r := e.run(a, "note", "create", "roteiro"); r.Code != 409 || !strings.Contains(r.Body, "taken by the note") {
		t.Fatalf("agent took a former name: %d %s", r.Code, r.Body)
	}
	if r := e.run(a, "note", "create", "Bugs"); r.Code != 200 {
		t.Fatalf("agent create: %d %s", r.Code, r.Body)
	}
	bugs, _ := findNote(e, "Bugs")
	if bugs.CreatedBy != a.ID {
		t.Fatalf("agent note author: %q", bugs.CreatedBy)
	}
	if code, body := e.api("PATCH", "/api/notes/"+bugs.ID, `{"title":"Erros"}`); code != 409 || !strings.Contains(body["error"].(string), "agent made this note") {
		t.Fatalf("renamed an agent's note: %d %v", code, body)
	}

	// The node PATCH changes a note's lock only.
	if code, _ := e.api("PATCH", "/api/nodes/"+d1.ID, `{"note":{"title":"X","file":"x.md","locked":true}}`); code != 200 {
		t.Fatalf("node patch: %d", code)
	}
	if n := node(d1.ID); n.Note.Title != "Plano de testes" || n.Note.File != "plano-de-testes.md" || !n.Note.Locked {
		t.Fatalf("node patch changed the name: %+v", n.Note)
	}

	// A draft ends with content (the file takes its provisional name)…
	if code, _ := e.api("PUT", "/api/notes/"+d2.ID, `{"content":"oi"}`); code != 200 {
		t.Fatalf("write a draft: %d", code)
	}
	if n := node(d2.ID); n.Note.Draft || n.Note.File != "nota-2.md" || !exists("nota-2.md") {
		t.Fatalf("draft after a write: %+v", n.Note)
	}
	// …when Squad starts (the watcher) and when it's connected.
	d3 := draft()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.app.Notes.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	if n := node(d3.ID); n.Note.Draft || n.Note.File == "" {
		t.Fatalf("draft after a start: %+v", n.Note)
	}
	d4 := draft()
	if _, err := e.app.Server.AddEdge(a.ID, d4.ID, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for node(d4.ID).Note.Draft && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := node(d4.ID); n.Note.Draft || !exists(n.Note.File) {
		t.Fatalf("draft after a connection: %+v", n.Note)
	}
}

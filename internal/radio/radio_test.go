package radio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/events"
)

type env struct {
	t     *testing.T
	file  string
	ev    *events.Bus
	texts map[string][2]string
}

func newEnv(t *testing.T) *env {
	return &env{t: t, file: filepath.Join(t.TempDir(), "radio.jsonl"), ev: events.NewBus(), texts: map[string][2]string{}}
}

// open starts a Rádio on the env's file (again, to read what was kept).
func (e *env) open() *Service {
	s := New(Options{File: e.file, Events: e.ev,
		Name:  func(id string) string { return map[string]string{"t1": "Implementador", "n1": "Plano"}[id] },
		Texts: func(id string) (string, string) { return e.texts[id][0], e.texts[id][1] }})
	e.t.Cleanup(s.Close)
	return s
}

var t0 = time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

func ev(typ string, at time.Duration, data map[string]any) events.Event {
	return events.Event{Type: typ, Time: t0.Add(at), Data: data}
}

func keys(items []Item) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Outcome)
	}
	return strings.Join(out, ",")
}

// Every kind of line, newest first; a request's line changes when it ends,
// and its texts are kept; all of it comes back from the file.
func TestLinesAndReload(t *testing.T) {
	e := newEnv(t)
	s := e.open()
	e.texts["r1"] = [2]string{"implemente o CSV inteiro", "[reply from Implementador · request r1]\nfeito: export.csv"}
	s.handle(ev(events.RequestSent, 0, map[string]any{"id": "r1", "from": "t0", "to": "t1", "fromName": "Planejador", "toName": "Implementador", "preview": "implemente o CSV…"}))
	s.handle(ev(events.ReplyReceived, time.Second, map[string]any{"id": "r1", "source": "claude-stop", "elapsedMs": 900}))
	s.handle(ev(events.RequestHeld, 2*time.Second, map[string]any{"from": "t0", "to": "t1", "fromName": "Planejador", "toName": "Implementador", "paused": false}))
	s.handle(ev(events.ConversationsPaused, 3*time.Second, map[string]any{"paused": true, "reason": "hop limit 6 reached A → B → A"}))
	s.handle(ev(events.ConversationsPaused, 4*time.Second, map[string]any{"paused": false}))
	s.handle(ev(events.MessageSent, 5*time.Second, map[string]any{"to": "t1", "toName": "Implementador", "preview": "oi", "images": 2}))
	s.handle(ev(events.TurnCard, 0, map[string]any{"terminal": "t1", "at": t0.Add(6 * time.Second), "message": "\n  pronto\nmais"}))
	s.handle(ev(events.TurnCard, 0, map[string]any{"terminal": "t1", "at": t0.Add(6 * time.Second), "message": "pronto", "summary": map[string]any{"text": "Exportou o CSV."}}))
	s.Answered("t1", attention.Attention{Kind: attention.Permission, Tool: "exec", Detail: "npm test", Since: t0.Add(7 * time.Second)}, "allow", t0.Add(11*time.Second))

	items, more := s.Page(time.Time{}, 50)
	if more || keys(items) != "answered,turn,delivered,held,held,replied" {
		t.Fatalf("lines: %s %v", keys(items), more)
	}
	if a := items[0]; a.From != "t1" || a.FromName != "Implementador" || a.Choice != "allow" || a.Tool != "exec" || a.Detail != "npm test" || a.ElapsedMs != 4000 {
		t.Fatalf("answered: %+v", a)
	}
	if tc := items[1]; tc.Text != "Exportou o CSV." || tc.FromName != "Implementador" {
		t.Fatalf("turn: %+v", tc)
	}
	if m := items[2]; m.By != "user" || m.Images != 2 || m.Text != "oi" {
		t.Fatalf("message: %+v", m)
	}
	if l, h := items[3], items[4]; l.Hold != "loop" || !strings.Contains(l.Text, "A → B → A") || h.Hold != "rate" {
		t.Fatalf("held: %+v %+v", l, h)
	}
	if r := items[5]; r.ID != "r1" || r.Source != "claude-stop" || r.ElapsedMs != 900 || !r.HasReply || r.Text != "implemente o CSV…" {
		t.Fatalf("request: %+v", r)
	}
	if req, reply, ok := s.Texts("r1"); !ok || req != "implemente o CSV inteiro" || reply != "feito: export.csv" {
		t.Fatalf("texts: %q %q", req, reply)
	}

	// Restarted: the same lines, the same texts.
	s.Close()
	again := e.open()
	if items2, _ := again.Page(time.Time{}, 50); keys(items2) != keys(items) || items2[1].Text != "Exportou o CSV." {
		t.Fatalf("reloaded: %s", keys(items2))
	}
	if _, reply, _ := again.Texts("r1"); reply != "feito: export.csv" {
		t.Fatalf("reloaded reply: %q", reply)
	}
}

// The edits of a note by one author within two minutes are one line; an
// agent, or a later edit, starts another.
func TestNoteEditsJoin(t *testing.T) {
	e := newEnv(t)
	s := e.open()
	note := func(at time.Duration, author string) {
		s.handle(ev(events.NoteEdited, at, map[string]any{"id": "n1", "file": "plano.md", "author": author}))
	}
	note(0, "user")
	note(30*time.Second, "user")
	note(100*time.Second, "user")
	note(110*time.Second, "Implementador")
	note(5*time.Minute, "user")
	items, _ := s.Page(time.Time{}, 50)
	if len(items) != 3 {
		t.Fatalf("lines: %+v", items)
	}
	if items[2].By != "user" || !items[2].Time.Equal(t0.Add(100*time.Second)) || items[2].ToName != "Plano" {
		t.Fatalf("joined: %+v", items[2])
	}
	if items[1].FromName != "Implementador" || items[1].By != "" || items[0].By != "user" {
		t.Fatalf("others: %+v %+v", items[1], items[0])
	}
}

// Only the last Limit lines are kept, the file is rewritten when it holds
// twice that, pages go back in time, and clearing empties it all.
func TestLimitPagesAndClear(t *testing.T) {
	e := newEnv(t)
	s := e.open()
	n := 2*Limit + 10
	for i := range n {
		s.handle(ev(events.MessageSent, time.Duration(i)*time.Second, map[string]any{"to": "t1", "toName": "Implementador", "preview": "m"}))
	}
	if b, _ := os.ReadFile(e.file); strings.Count(string(b), "\n") > 2*Limit {
		t.Fatalf("file not rewritten: %d lines", strings.Count(string(b), "\n"))
	}
	first, more := s.Page(time.Time{}, 200)
	if len(first) != 200 || !more || !first[0].Time.Equal(t0.Add(time.Duration(n-1)*time.Second)) {
		t.Fatalf("first page: %d %v %v", len(first), more, first[0].Time)
	}
	var all []Item
	for page, more := first, true; ; page, more = s.Page(page[len(page)-1].Time, 200) {
		all = append(all, page...)
		if !more {
			break
		}
	}
	if len(all) != Limit || !all[Limit-1].Time.Equal(t0.Add(time.Duration(n-Limit)*time.Second)) {
		t.Fatalf("kept %d, oldest %v", len(all), all[len(all)-1].Time)
	}
	s.Close()
	again := e.open()
	if items, _ := again.Page(time.Time{}, Limit+1); len(items) != Limit {
		t.Fatalf("reloaded %d", len(items))
	}
	if err := again.Clear(); err != nil {
		t.Fatal(err)
	}
	if items, _ := again.Page(time.Time{}, 10); len(items) != 0 {
		t.Fatalf("cleared: %d", len(items))
	}
	again.Close()
	if items, _ := e.open().Page(time.Time{}, 10); len(items) != 0 {
		t.Fatalf("cleared, reloaded: %d", len(items))
	}
}

// From the bus itself: lines arrive as events, and are told to the
// interface; a line cut by a crash is skipped; long texts are cut.
func TestFromEvents(t *testing.T) {
	e := newEnv(t)
	_ = os.WriteFile(e.file, []byte(`{"key":"msg-x","time":"2026-01-01T09:00:00Z","outcome":"delivered","text":"antes"}`+"\n"+`{"key":"cut`), 0o600)
	s := e.open()
	e.texts["r2"] = [2]string{strings.Repeat("é", MaxText), ""}
	ch, cancel := e.ev.Subscribe()
	defer cancel()
	e.ev.Publish(events.RequestSent, map[string]any{"id": "r2", "from": "a", "to": "b", "fromName": "A", "toName": "B", "preview": "é…"})
	deadline := time.After(3 * time.Second)
	for {
		select {
		case got := <-ch:
			if got.Type != events.RadioItem {
				continue
			}
			if it, ok := got.Data.(Item); !ok || it.ID != "r2" {
				t.Fatalf("radio.item: %+v", got.Data)
			}
			items, _ := s.Page(time.Time{}, 10)
			if keys(items) != "pending,delivered" {
				t.Fatalf("lines: %s", keys(items))
			}
			req, _, _ := s.Texts("r2")
			if len(req) > MaxText+len("…") || !strings.HasSuffix(req, "…") || !strings.HasPrefix(req, "é") {
				t.Fatalf("cut: %d bytes", len(req))
			}
			return
		case <-deadline:
			t.Fatal("no radio.item")
		}
	}
}

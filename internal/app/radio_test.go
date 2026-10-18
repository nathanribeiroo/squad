//go:build !windows

package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/radio"
)

// radioLine waits for the line of the Rádio that ok accepts.
func (e *env) radioLine(ok func(radio.Item) bool) radio.Item {
	e.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		items, _ := e.app.Radio.Page(time.Time{}, radio.Limit)
		for _, it := range items {
			if ok(it) {
				return it
			}
		}
	}
	items, _ := e.app.Radio.Page(time.Time{}, radio.Limit)
	e.t.Fatalf("no such line in the Rádio: %+v", items)
	return radio.Item{}
}

// The Rádio is kept by the server, next to the canvas: a request and its
// reply (whole, on demand), the user's message, and the asks answered from
// the interface, with the answer, for Claude Code and Devin alike.
func TestRadioKept(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	d := e.agentOfType("Devin", "devin", "FAKEAGENT_HOOK=devin")
	e.connect(plan, c)
	e.connect(plan, d)

	if r := e.run(plan, "ask", "Claude", "faça o relatório"); r.Code != 200 {
		t.Fatalf("ask: %+v", r)
	}
	req := e.radioLine(func(it radio.Item) bool { return it.Outcome == "replied" && it.FromName == "Plano" })
	if !req.HasReply || req.ToName != "Claude" || req.Source != "claude-stop" {
		t.Fatalf("request: %+v", req)
	}
	w := e.call("GET", "/api/requests/"+req.ID, nil)
	var texts struct{ Text, Reply string }
	_ = json.Unmarshal(w.Body.Bytes(), &texts)
	if w.Code != 200 || !strings.Contains(texts.Text, "faça o relatório") || !strings.HasPrefix(texts.Reply, "final: ") {
		t.Fatalf("texts: %d %s", w.Code, w.Body)
	}

	if err := e.app.Send(c.ID, ComposeRequest{Text: "oi do usuário"}); err != nil {
		t.Fatal(err)
	}
	e.radioLine(func(it radio.Item) bool {
		return it.Outcome == "delivered" && it.By == "user" && it.Text == "oi do usuário"
	})

	// Asks answered from the interface: Claude allowed, Devin denied.
	for _, f := range []struct {
		node   string
		choice attention.Choice
		tool   string
	}{{c.ID, attention.Allow, "Bash"}, {d.ID, attention.Deny, "exec"}} {
		name := map[string]string{c.ID: "Claude", d.ID: "Devin"}[f.node]
		go e.run(plan, "ask", name, "ASKPERM:make test")
		e.waitAttention(f.node, func(a *attention.Attention) bool { return a != nil && len(a.Choices) > 0 }, 10*time.Second)
		if code, body := e.respond(f.node, string(f.choice)); code != 204 {
			t.Fatalf("respond: %d %s", code, body)
		}
		ask := e.radioLine(func(it radio.Item) bool { return it.Outcome == "answered" && it.From == f.node })
		if ask.Choice != string(f.choice) || ask.Tool != f.tool || ask.Detail != "make test" || ask.FromName != name {
			t.Fatalf("ask of %s: %+v", name, ask)
		}
	}

	// The route the interface reads, and the file next to the canvas.
	w = e.call("GET", "/api/radio?limit=2", nil)
	var page struct {
		Items []radio.Item
		More  bool
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if w.Code != 200 || len(page.Items) != 2 || !page.More || page.Items[0].Outcome != "answered" {
		t.Fatalf("page: %d %s", w.Code, w.Body)
	}
	kept := radio.New(radio.Options{File: filepath.Join(e.dir, "radio.jsonl"), Events: events.NewBus()})
	defer kept.Close()
	if items, _ := kept.Page(time.Time{}, radio.Limit); len(items) < 4 {
		t.Fatalf("file: %+v", items)
	}

	if w := e.call("DELETE", "/api/radio", nil); w.Code != 204 {
		t.Fatalf("clear: %d %s", w.Code, w.Body)
	}
	if items, _ := e.app.Radio.Page(time.Time{}, 10); len(items) != 0 {
		t.Fatalf("cleared: %+v", items)
	}
}

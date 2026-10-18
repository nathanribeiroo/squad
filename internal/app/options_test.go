//go:build !windows

package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/attention"
	"github.com/nathanribeiroo/squad/internal/radio"
)

// respondOption answers terminal id's question or plan with one of the
// agent's options (and the user's text, on the text field).
func (e *env) respondOption(id, option, text string) (int, string) {
	b, _ := json.Marshal(map[string]string{"option": option, "text": text})
	code, body := e.api("POST", "/api/nodes/"+id+"/attention/respond", string(b))
	out, _ := json.Marshal(body)
	return code, string(out)
}

// yourTurn reports whether terminal id waits for the user after a turn of
// theirs (its runtime's yourTurn).
func (e *env) yourTurn(id string) bool {
	_, st := e.api("GET", "/api/state", "")
	rt, _ := st["runtime"].(map[string]any)[id].(map[string]any)
	return rt["yourTurn"] == true
}

// A Claude asking a question (AskUserQuestion) is answered from the
// interface with its own options, read on screen with the description under
// each; the text field takes the user's own answer, in one line; what isn't
// offered is refused; the Rádio says what was picked.
func TestQuestionOptions(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	e.connect(plan, c)

	done := make(chan agentbus.Result, 1)
	go func() { done <- e.run(plan, "ask", "Claude", "ASKCQ:Qual cor?") }()
	a := e.waitAttention(c.ID, func(a *attention.Attention) bool { return a != nil && len(a.Options) == 4 }, 10*time.Second)
	if a.Kind != attention.Question || a.Tool != "AskUserQuestion" || a.Text != "Qual cor?" || a.Detail != "" || a.Choices != nil {
		t.Fatalf("attention: %+v", a)
	}
	if o := a.Options[0]; o.Key != "1" || o.Text != "Azul" || o.Detail != "The first" || o.Input {
		t.Fatalf("option 1: %+v", o)
	}
	if o := a.Options[2]; !o.Input || a.Options[3].Text != "Chat about this" {
		t.Fatalf("text field: %+v", a.Options)
	}

	for _, c2 := range []struct{ option, text string }{{"9", ""}, {"1", "texto"}, {"3", ""}, {"3", "  "}} {
		if code, body := e.respondOption(c.ID, c2.option, c2.text); code != 400 || !strings.Contains(body, "attention.no_keys") {
			t.Fatalf("%+v: %d %s", c2, code, body)
		}
	}
	if code, body := e.respondOption(c.ID, "3", strings.Repeat("x", 2001)); code != 400 || !strings.Contains(body, "attention.too_long") {
		t.Fatalf("too long: %d %s", code, body)
	}
	if code, body := e.respond(c.ID, "allow"); code != 400 {
		t.Fatalf("a permission's answer on a question: %d %s", code, body)
	}
	if code, body := e.respondOption(c.ID, "3", "Roxo\ncom brilho"); code != 204 {
		t.Fatalf("respond: %d %s", code, body)
	}
	waitUntil(t, "the text typed", func() bool { return strings.Contains(e.log("Claude"), `ASK-TEXT "Roxo com brilho"`) })
	if r := <-done; r.Code != 200 {
		t.Fatalf("ask: %+v", r)
	}
	line := e.radioLine(func(it radio.Item) bool { return it.Outcome == "answered" })
	if line.Kind != attention.Question || line.Choice != "option" || line.Answer != "Roxo com brilho" || line.Text != "Qual cor?" {
		t.Fatalf("Rádio: %+v", line)
	}
	// A request's turn isn't the user's.
	if e.yourTurn(c.ID) {
		t.Fatal("your turn after a request's turn")
	}
}

// A Claude's plan (ExitPlanMode): the hook brings the plan, whole, and its
// title; an option approves it. The turn was the user's (typed from the
// interface), so when it ends the flight waits for them; the next turn of
// a request clears it.
func TestPlanOptionsAndYourTurn(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	e.connect(plan, c)

	if err := e.app.Send(c.ID, ComposeRequest{Text: "PLAN:Plan: create hello.txt"}); err != nil {
		t.Fatal(err)
	}
	a := e.waitAttention(c.ID, func(a *attention.Attention) bool { return a != nil && len(a.Options) == 3 }, 10*time.Second)
	if a.Kind != attention.Plan || a.Tool != "ExitPlanMode" || a.Detail != "Plan: create hello.txt" ||
		!strings.Contains(a.PlanText, "1. Create hello.txt") || !strings.HasPrefix(a.Text, "Claude has written up a plan") {
		t.Fatalf("attention: %+v", a)
	}
	if !a.Options[2].Input || a.Options[2].Detail != "shift+tab to approve with this feedback" {
		t.Fatalf("feedback field: %+v", a.Options[2])
	}
	if e.yourTurn(c.ID) {
		t.Fatal("your turn while asking")
	}
	if code, body := e.respondOption(c.ID, "2", ""); code != 204 {
		t.Fatalf("respond: %d %s", code, body)
	}
	waitUntil(t, "the option picked", func() bool { return strings.Contains(e.log("Claude"), "ASK-CHOSE 2") })
	waitUntil(t, "your turn", func() bool { return e.yourTurn(c.ID) })
	line := e.radioLine(func(it radio.Item) bool { return it.Outcome == "answered" })
	if line.Kind != attention.Plan || line.Answer != "Yes, manually approve edits" {
		t.Fatalf("Rádio: %+v", line)
	}

	// Its next turn, a request's, clears it and doesn't set it again.
	if r := e.run(plan, "ask", "Claude", "e agora?"); r.Code != 200 {
		t.Fatalf("ask: %+v", r)
	}
	time.Sleep(300 * time.Millisecond)
	if e.yourTurn(c.ID) {
		t.Fatal("still your turn after a request's turn")
	}
}

// A turn the user ended themselves (NEGAR from the interface) isn't a turn
// to tell them about; nor is a custom command's.
func TestYourTurnNotAfterDeny(t *testing.T) {
	e := newEnv(t, fastLimits)
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	custom := e.agent("Outro")

	if err := e.app.Send(c.ID, ComposeRequest{Text: "ASKPERM:rm -rf build"}); err != nil {
		t.Fatal(err)
	}
	e.waitAttention(c.ID, func(a *attention.Attention) bool { return a != nil && len(a.Choices) == 3 }, 10*time.Second)
	if code, body := e.respond(c.ID, "deny"); code != 204 {
		t.Fatalf("deny: %d %s", code, body)
	}
	waitUntil(t, "the attention cleared", func() bool { return e.app.Monitor.Attention(c.ID) == nil })
	if err := e.app.Send(custom.ID, ComposeRequest{Text: "oi"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the custom turn", func() bool { return strings.Contains(e.log("Outro"), "TURN-END") })
	time.Sleep(2 * time.Second) // the denied turn ends on the screen's quiet
	if e.yourTurn(c.ID) || e.yourTurn(custom.ID) {
		t.Fatalf("your turn: claude=%v custom=%v", e.yourTurn(c.ID), e.yourTurn(custom.ID))
	}
}

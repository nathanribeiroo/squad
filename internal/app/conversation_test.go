//go:build !windows

package app

import (
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/store"
)

// conversation is a terminal's remembered conversation id.
func (e *env) conversation(id string) string {
	n, _ := e.app.Server.Store().Node(id)
	return n.Terminal.Conversation
}

// starts returns the fake agent's launch arguments, one line per start.
func (e *env) starts(name string) []string {
	var out []string
	for _, line := range strings.Split(e.log(name), "\n") {
		if _, args, ok := strings.Cut(line, " START "); ok {
			out = append(out, args)
		}
	}
	return out
}

// terminal is a terminal's saved settings.
func (e *env) terminal(id string) store.TerminalData {
	n, _ := e.app.Server.Store().Node(id)
	return *n.Terminal
}

// talk gives an agent a turn and waits for its end: its conversation is
// then one to continue.
func (e *env) talk(n store.Node, text string) {
	e.t.Helper()
	if err := e.app.Send(n.ID, ComposeRequest{Text: text}); err != nil {
		e.t.Fatal(err)
	}
	waitUntil(e.t, "the turn", func() bool {
		t := e.terminal(n.ID)
		return !t.Empty && !t.Busy && strings.Contains(e.log(n.Terminal.Name), "TURN-END")
	})
}

// restart starts a terminal again as Torre does when it reopens.
func (e *env) restart(n store.Node) {
	e.t.Helper()
	if err := e.app.Server.StartTerminal(n.ID); err != nil {
		e.t.Fatal(err)
	}
	e.waitScreen(n.ID, "fakeagent ready", 10*time.Second)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A Claude terminal starts in a conversation whose id Torre picks, continues
// it when it starts again (once something was said in it), and starts a new
// one on request.
func TestConversationContinues(t *testing.T) {
	e := newEnv(t, fastLimits)
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	first := e.conversation(c.ID)
	if len(first) != 36 || !strings.Contains(e.starts("Claude")[0], `"--session-id" "`+first+`"`) || !e.terminal(c.ID).Empty {
		t.Fatalf("new conversation %q: %v %+v", first, e.starts("Claude"), e.terminal(c.ID))
	}

	evs, cancel := e.app.Server.Bus().Subscribe()
	defer cancel()
	// Nothing said yet: nothing to continue (the agent wouldn't find it).
	e.restart(c)
	conv := e.conversation(c.ID)
	if s := e.starts("Claude"); len(s) != 2 || conv == first || !strings.Contains(s[1], `"--session-id" "`+conv+`"`) || strings.Contains(s[1], "--resume") {
		t.Fatalf("empty conversation started again: %v %q", s, conv)
	}
	e.talk(c, "oi")
	e.restart(c)
	if s := e.starts("Claude"); len(s) != 3 || !strings.Contains(s[2], `"--resume" "`+conv+`"`) || e.conversation(c.ID) != conv {
		t.Fatalf("continued: %v %q", s, e.conversation(c.ID))
	}
	// It did continue: nothing to tell the user.
	for quiet := time.After(500 * time.Millisecond); ; {
		select {
		case ev := <-evs:
			if ev.Type == events.TerminalNotice {
				t.Fatalf("notice after a resume that worked: %v", ev.Data)
			}
			continue
		case <-quiet:
		}
		break
	}

	// The canvas sends back what it has: it can't change or drop the id.
	if code, _ := e.api("PATCH", "/api/nodes/"+c.ID, `{"terminal":{"name":"Claude","agent":"claude","command":"`+strings.ReplaceAll(c.Terminal.Command, `"`, `\"`)+`","shell":"/bin/sh","cols":100,"rows":30}}`); code != 200 || e.conversation(c.ID) != conv {
		t.Fatalf("patch: %d %q", code, e.conversation(c.ID))
	}

	// "Nova conversa".
	if code, _ := e.api("POST", "/api/nodes/"+c.ID+"/new-conversation", ""); code != 200 {
		t.Fatalf("new conversation: %d", code)
	}
	e.waitScreen(c.ID, "fakeagent ready", 10*time.Second)
	fresh := e.conversation(c.ID)
	if s := e.starts("Claude"); fresh == conv || len(s) != 4 || !strings.Contains(s[3], `"--session-id" "`+fresh+`"`) {
		t.Fatalf("fresh %q: %v", fresh, s)
	}

	// Another folder can't continue it (agents keep conversations per folder).
	if code, _ := e.api("PATCH", "/api/nodes/"+c.ID, `{"terminal":{"name":"Claude","agent":"claude","command":"x","cwd":"/tmp"}}`); code != 200 || e.conversation(c.ID) != "" {
		t.Fatalf("new folder: %d %q", code, e.conversation(c.ID))
	}
}

// An agent that can't find the conversation starts a new one with the same
// id, from the launch line itself, and the user is told.
func TestConversationNotFound(t *testing.T) {
	e := newEnv(t, fastLimits)
	notices, cancel := e.app.Server.Bus().Subscribe()
	defer cancel()
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude", "FAKEAGENT_RESUME_FAIL=1")
	conv := e.conversation(c.ID)
	e.talk(c, "oi")
	e.restart(c)
	s := e.starts("Claude")
	if len(s) != 3 || !strings.Contains(s[1], `"--resume" "`+conv+`"`) || !strings.Contains(s[2], `"--session-id" "`+conv+`"`) ||
		!strings.Contains(e.log("Claude"), "RESUME-FAIL") || e.conversation(c.ID) != conv {
		t.Fatalf("fallback: %v %q", s, e.conversation(c.ID))
	}
	deadline := time.After(5 * time.Second)
	for told := false; !told; {
		select {
		case ev := <-notices:
			d, ok := ev.Data.(map[string]string)
			told = ev.Type == events.TerminalNotice && ok && d["kind"] == "not-resumed" && d["name"] == "Claude"
		case <-deadline:
			t.Fatal("no notice")
		}
	}
	// The conversation it fell back to is a new one: the next start doesn't
	// try to continue it.
	waitUntil(t, "the new conversation", func() bool { return e.terminal(c.ID).Empty })
	e.restart(c)
	if s := e.starts("Claude"); len(s) != 4 || strings.Contains(s[3], "--resume") {
		t.Fatalf("after the fallback: %v", s)
	}
}

// Devin picks its own conversation id; its hooks report it.
func TestDevinConversationFromHooks(t *testing.T) {
	e := newEnv(t, fastLimits)
	d := e.agentOfType("Devin", "devin", "FAKEAGENT_HOOK=devin", "FAKEAGENT_SESSION=dv-1")
	if s := e.starts("Devin")[0]; strings.Contains(s, "--session-id") {
		t.Fatalf("devin can't take an id: %s", s)
	}
	waitUntil(t, "the id from the session start hook", func() bool { return e.conversation(d.ID) == "dv-1" && e.terminal(d.ID).Empty })
	e.talk(d, "oi")
	e.restart(d)
	if s := e.starts("Devin"); len(s) != 2 || !strings.Contains(s[1], `"--resume" "dv-1"`) {
		t.Fatalf("devin continued: %v", s)
	}
}

// An agent stopped in the middle of a turn comes back interrupted, until
// the user continues it (or dismisses the notice).
func TestInterruptedAgentContinues(t *testing.T) {
	e := newEnv(t, fastLimits)
	c := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	busy := func() bool { n, _ := e.app.Server.Store().Node(c.ID); return n.Terminal.Busy }
	interrupted := func() bool {
		_, st := e.api("GET", "/api/state", "")
		rt, _ := st["runtime"].(map[string]any)[c.ID].(map[string]any)
		return rt["interrupted"] == true
	}

	if err := e.app.Send(c.ID, ComposeRequest{Text: "SLEEP:5"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the turn start", busy)
	e.restart(c)
	if !busy() || !interrupted() {
		t.Fatalf("after the restart: busy=%v interrupted=%v", busy(), interrupted())
	}
	if code, body := e.api("POST", "/api/interrupted/continue", `{}`); code != 200 || len(body["ids"].([]any)) != 1 {
		t.Fatalf("continue: %d %v", code, body)
	}
	e.waitScreen(c.ID, "received: O seu trabalho foi interrompido", 5*time.Second)
	if interrupted() {
		t.Fatal("still interrupted after continuing")
	}
	waitUntil(t, "the turn end", func() bool { return !busy() })

	// Dismissing forgets it, also for the next start.
	if err := e.app.Send(c.ID, ComposeRequest{Text: "SLEEP:5"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the turn start", busy)
	e.restart(c)
	if code, _ := e.api("POST", "/api/interrupted/dismiss", `{}`); code != 200 || interrupted() || busy() {
		t.Fatalf("dismiss: %d interrupted=%v busy=%v", code, interrupted(), busy())
	}
	e.restart(c)
	if interrupted() {
		t.Fatal("interrupted again after dismissing")
	}
}

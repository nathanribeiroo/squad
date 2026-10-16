//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/events"
	"github.com/nathanribeiroo/squad/internal/summary"
)

// An agent's turn ends through its hook: its card carries the final message
// and the prompt; "Resumir" runs the claude CLI found on PATH.
func TestTurnCardsAndSummary(t *testing.T) {
	bin := t.TempDir()
	answer := `{"result": "{\"resumo\": \"Respondeu pong.\", \"passos\": [\"Peça o próximo passo\"]}", "is_error": false}`
	script := "#!/bin/sh\ncat > " + filepath.Join(bin, "stdin.txt") + "\ncat <<'OUT'\n" + answer + "\nOUT\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	e := newEnv(t, fastLimits)
	ch, cancel := e.app.Server.Bus().Subscribe()
	defer cancel()
	a := e.agent("A")
	b := e.agentOfType("Claude", "claude", "FAKEAGENT_HOOK=claude")
	e.setRole(b, "impl")
	e.connect(a, b)
	if r := e.run(a, "ask", "Claude", "diga pong"); r.Code != 200 {
		t.Fatalf("ask: %d %s", r.Code, r.Body)
	}
	next := func(ok func(summary.Card) bool) summary.Card {
		t.Helper()
		timeout := time.After(10 * time.Second)
		for {
			select {
			case ev := <-ch:
				if c, is := ev.Data.(summary.Card); is && ev.Type == events.TurnCard && ok(c) {
					return c
				}
			case <-timeout:
				t.Fatal("no turn.card")
			}
		}
	}
	c := next(func(c summary.Card) bool { return c.Terminal == b.ID })
	if c.Source != "hook" || !strings.HasPrefix(c.Message, "final: [de: A · pedido ") || !strings.Contains(c.Prompt, "diga pong") || c.Start.IsZero() {
		t.Fatalf("card: %+v", c)
	}
	// The fake agent "A" has no hooks: its turns don't make cards here (custom).
	if code, body := e.api("POST", "/api/nodes/"+a.ID+"/summarize", ""); code != 404 {
		t.Fatalf("summarize a terminal without a turn: %d %v", code, body)
	}
	if code, body := e.api("POST", "/api/nodes/"+b.ID+"/summarize", ""); code != 202 || body["summarizing"] != true {
		t.Fatalf("summarize: %d %v", code, body)
	}
	done := next(func(c summary.Card) bool { return c.Terminal == b.ID && c.Summary != nil })
	if done.Summary.By != "claude" || done.Summary.Text != "Respondeu pong." || len(done.Summary.Steps) != 1 {
		t.Fatalf("summary: %+v", done.Summary)
	}
	material, _ := os.ReadFile(filepath.Join(bin, "stdin.txt"))
	if !strings.Contains(string(material), `"Claude" (agente claude, papel Implementador, térreo`) || !strings.Contains(string(material), "diga pong") {
		t.Fatalf("what claude read:\n%s", material)
	}
	if code, body := e.api("GET", "/api/summary", ""); code != 200 || len(body["cards"].([]any)) != 1 {
		t.Fatalf("cards: %d %v", code, body)
	}
	if code, body := e.api("GET", "/api/summary/settings", ""); code != 200 || body["engine"] != "auto" || body["cards"] != true {
		t.Fatalf("settings: %d %v", code, body)
	}
	if code, _ := e.api("PUT", "/api/summary/settings", `{"cards":false,"engine":"off"}`); code != 200 {
		t.Fatalf("set settings: %d", code)
	}
	if code, body := e.api("POST", "/api/summary/workspace", ""); code != 409 {
		t.Fatalf("overview with summaries off: %d %v", code, body)
	}
}

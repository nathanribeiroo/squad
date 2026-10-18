//go:build !windows

package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/usage"
)

// statusLine runs `torre hook claude-statusline` as Claude Code does: the
// payload on stdin, HOME pointing at home (the user's own settings).
func (e *env) statusLine(n store.Node, home, payload string) string {
	e.t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "torre"), "hook", "claude-statusline")
	cmd.Env = append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "SQUAD_URL="+e.app.Server.URL(), "SQUAD_TERMINAL_ID="+n.ID, "SQUAD_TOKEN="+e.app.Server.TerminalToken(n.ID))
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		e.t.Fatalf("torre hook claude-statusline: %v", err)
	}
	return string(out)
}

// Claude Code's status line brings a flight's usage and the plan's limits;
// the user's own status line still shows.
func TestUsageFromClaudeStatusLine(t *testing.T) {
	e := newEnv(t, fastLimits)
	cl := e.agentOfType("Cl", "claude")
	home := t.TempDir()
	tp := filepath.Join(home, "transcript.jsonl")
	if err := os.WriteFile(tp, []byte(`{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":7,"output_tokens":300,"cache_read_input_tokens":20000,"cache_creation_input_tokens":4000}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"session_id": "c1", "transcript_path": tp, "cwd": e.dir, "workspace": map[string]any{"project_dir": e.dir},
		"model": map[string]any{"id": "claude-opus-5-5"}, "cost": map[string]any{"total_cost_usd": 0.42},
		"context_window": map[string]any{"context_window_size": 200000, "used_percentage": 12,
			"current_usage": map[string]any{"input_tokens": 7, "cache_creation_input_tokens": 4000, "cache_read_input_tokens": 20000}},
		"rate_limits": map[string]any{"five_hour": map[string]any{"used_percentage": 81, "resets_at": time.Now().Add(time.Hour).Unix()},
			"seven_day": map[string]any{"used_percentage": 33, "resets_at": time.Now().Add(72 * time.Hour).Unix()}},
	})

	// Without a status line of the user's, nothing is printed.
	if out := e.statusLine(cl, home, string(payload)); out != "" {
		t.Fatalf("printed %q", out)
	}
	var got struct {
		Flights  []usage.Flight  `json:"flights"`
		Accounts []usage.Account `json:"accounts"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w := e.call("GET", "/api/usage", nil)
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if len(got.Flights) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(got.Flights) != 1 || got.Flights[0].ID != cl.ID || got.Flights[0].Tokens.CacheRead != 20000 || got.Flights[0].CostUSD != 0.42 || got.Flights[0].Context.Percent != 12 {
		t.Fatalf("flights: %+v", got.Flights)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].Agent != "claude" || got.Accounts[0].Windows[0].Used != 81 {
		t.Fatalf("accounts: %+v", got.Accounts)
	}

	// The user's status line (project settings first, then ~/.claude) runs
	// with the same payload; Torre's own is never run again.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"statusLine":{"type":"command","command":"read x; echo \"minha linha $(printf %s \"$x\" | head -c 14)\""}}`), 0o644)
	if out := e.statusLine(cl, home, string(payload)); out != "minha linha {\"context_wind\n" {
		t.Fatalf("user status line: %q", out)
	}
	os.MkdirAll(filepath.Join(e.dir, ".claude"), 0o755)
	os.WriteFile(filepath.Join(e.dir, ".claude", "settings.json"), []byte(`{"statusLine":{"type":"command","command":"\"/x/torre\" hook claude-statusline"}}`), 0o644)
	os.WriteFile(filepath.Join(e.dir, ".claude", "settings.local.json"), []byte(`{"statusLine":{"type":"command","command":"echo do projeto"}}`), 0o644)
	if out := e.statusLine(cl, home, string(payload)); out != "do projeto\n" {
		t.Fatalf("project status line: %q", out)
	}
	os.Remove(filepath.Join(e.dir, ".claude", "settings.local.json"))
	if out := e.statusLine(cl, home, string(payload)); out != "minha linha {\"context_wind\n" {
		t.Fatalf("Torre's own status line was not skipped: %q", out)
	}

	// Removed with its terminal.
	if w := e.call("DELETE", "/api/nodes/"+cl.ID, nil); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
	w := e.call("GET", "/api/usage", nil)
	got.Flights = nil
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Flights) != 0 || len(got.Accounts) != 1 {
		t.Fatalf("after removal: %+v", got)
	}
}

// Devin's quota comes from its screen, when the user opens /usage there; the
// accounts are the same in every workspace.
func TestUsageFromDevinScreen(t *testing.T) {
	e := newEnv(t, fastLimits)
	dv := e.agentOfType("Dv", "devin")
	term := e.app.Server.Terminals().Get(dv.ID)
	_ = term.Raw([]byte(`SHOW:❭ /usage\n\n Fetching account usage…\n\n Daily   ■■■■■■■■■■  41% used  · resets in 3h 5m\n Weekly  ■■■■■■■■■■  12% used  · resets Oct 25, 8:00 AM (UTC)\n\n This session\n Model     SWE-1.6 Slow` + "\r"))
	var got struct {
		Accounts []usage.Account `json:"accounts"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		w := e.call("GET", "/api/usage", nil)
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if len(got.Accounts) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no account: %s\n%s", w.Body, term.Snapshot())
		}
		time.Sleep(100 * time.Millisecond)
	}
	a := got.Accounts[0]
	if a.Agent != "devin" || len(a.Windows) != 2 || a.Windows[0].Kind != usage.Daily || a.Windows[0].Used != 41 || a.Windows[1].Used != 12 ||
		time.Until(a.Windows[0].ResetsAt) < 3*time.Hour || time.Until(a.Windows[0].ResetsAt) > 3*time.Hour+10*time.Minute {
		t.Fatalf("account: %+v", a)
	}
}

package usage

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A status line as Claude Code 2.1.302 sent it (paths shortened).
const statusLineJSON = `{"session_id":"34790c28-ca28-54ee-b83e-6f345875f8d1","transcript_path":"TRANSCRIPT","cwd":"/home/user/squad",
"model":{"id":"claude-sonnet-5-5","display_name":"Sonnet 5.5"},"workspace":{"current_dir":"/home/user/squad","project_dir":"/home/user/squad"},
"version":"2.1.302","cost":{"total_cost_usd":0.166712,"total_duration_ms":11656},
"context_window":{"total_input_tokens":41638,"total_output_tokens":4,"context_window_size":1000000,
"current_usage":{"input_tokens":2,"output_tokens":4,"cache_creation_input_tokens":41636,"cache_read_input_tokens":0},"used_percentage":4,"remaining_percentage":96},
"rate_limits":{"five_hour":{"used_percentage":74,"resets_at":1792365600},"seven_day":{"used_percentage":45,"resets_at":1792800000}}}`

// A reply is written once per content block, with the same usage.
const transcript = `{"type":"user","message":{"role":"user","content":"oi"}}
{"type":"assistant","requestId":"req_1","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":2,"cache_creation_input_tokens":9166,"cache_read_input_tokens":29617,"output_tokens":203}}}
{"type":"assistant","requestId":"req_1","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":2,"cache_creation_input_tokens":9166,"cache_read_input_tokens":29617,"output_tokens":203}}}
{"type":"assistant","requestId":"req_2","message":{"id":"msg_2","model":"claude-opus-5-5","usage":{"input_tokens":3,"cache_creation_input_tokens":735,"cache_read_input_tokens":38783,"output_tokens":233}}}
`

const costStateLine = `{"type":"cost-state","sessionId":"s","totalCostUSD":0.1137561,"modelUsage":{"claude-haiku-5-5":{"inputTokens":1235,"outputTokens":18,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"costUSD":0.0001325},"claude-opus-5-5":{"inputTokens":6,"outputTokens":522,"cacheReadInputTokens":107918,"cacheCreationInputTokens":10197,"costUSD":0.1136236}}}
`

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, data string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
}

type events struct {
	flights []Flight
}

func newTestTracker(t *testing.T, terms map[string]Terminal, o Options) (*Tracker, *events, *Accounts) {
	ev := &events{}
	acc := OpenAccounts(filepath.Join(t.TempDir(), "accounts.json"))
	o.Accounts = acc
	o.Terminal = func(id string) (Terminal, bool) { x, ok := terms[id]; return x, ok }
	o.Publish = func(typ string, data any) {
		if fl, ok := data.(Flight); ok && typ == "usage.flight" {
			ev.flights = append(ev.flights, fl)
		}
	}
	o.Now = func() time.Time { return time.Date(2026, 10, 18, 21, 0, 0, 0, time.UTC) }
	return NewTracker(o), ev, acc
}

func TestClaudeStatusLineAndTranscript(t *testing.T) {
	dir := t.TempDir()
	tp := filepath.Join(dir, "projects", "-home-user-squad", "s1.jsonl")
	write(t, tp, transcript)
	tr, ev, acc := newTestTracker(t, map[string]Terminal{"T": {Agent: "claude", Conversation: "s1"}}, Options{ClaudeHome: dir})

	tr.StatusLine("T", []byte(strings.Replace(statusLineJSON, "TRANSCRIPT", filepath.ToSlash(tp), 1)))
	fl := tr.Flights()[0]
	// Each reply counted once.
	if fl.Tokens != (Tokens{Input: 5, Output: 436, CacheRead: 68400, CacheWrite: 9901}) || fl.CostUSD != 0.166712 {
		t.Fatalf("flight: %+v", fl)
	}
	if fl.Context == nil || fl.Context.Percent != 4 || fl.Context.Size != 1000000 || fl.Context.Used != 41638 {
		t.Fatalf("context: %+v", fl.Context)
	}
	a, ok := acc.Get("claude")
	if !ok || len(a.Windows) != 2 || a.Windows[0].Kind != FiveHour || a.Windows[0].Used != 74 || a.Windows[1].Kind != SevenDay ||
		!a.Windows[0].ResetsAt.Equal(time.Unix(1792365600, 0)) {
		t.Fatalf("account: %+v", a)
	}

	// Claude Code's own running total wins once it is written (it counts
	// side requests too); the transcript is read on from where it stopped.
	appendFile(t, tp, costStateLine)
	tr.Refresh("T")
	fl = tr.Flights()[0]
	if fl.Tokens != (Tokens{Input: 1241, Output: 540, CacheRead: 107918, CacheWrite: 10197}) || fl.Context == nil || fl.CostUSD != 0.166712 {
		t.Fatalf("after cost-state: %+v", fl)
	}
	if len(ev.flights) != 2 {
		t.Fatalf("events: %d", len(ev.flights))
	}
	tr.Refresh("T") // nothing new: no event
	if len(ev.flights) != 2 {
		t.Fatalf("an unchanged flight was announced again")
	}

	// After a restart, the transcript is found by the conversation id.
	tr2, _, _ := newTestTracker(t, map[string]Terminal{"T": {Agent: "claude", Conversation: "s1"}}, Options{ClaudeHome: dir})
	tr2.Refresh("T")
	if f := tr2.Flights(); len(f) != 1 || f[0].Tokens.CacheRead != 107918 {
		t.Fatalf("found by id: %+v", f)
	}
	// An API key: no rate limits, the account keeps what it had.
	noLimits := strings.Replace(statusLineJSON, `"rate_limits"`, `"no_limits"`, 1)
	tr.StatusLine("T", []byte(noLimits))
	if a2, _ := acc.Get("claude"); len(a2.Windows) != 2 {
		t.Fatalf("account lost: %+v", a2)
	}
}

func TestAccountsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	a := OpenAccounts(path)
	var got []Account
	stop := a.Subscribe(func(x Account) { got = append(got, x) })
	a.Set(Account{Agent: "claude", Windows: []Window{{Kind: FiveHour, Used: 10}}})
	stop()
	a.Set(Account{Agent: "devin", ACUs: &ACUs{Used: 3.5, Limit: 250}})
	if len(got) != 1 {
		t.Fatalf("subscriber: %d", len(got))
	}
	b := OpenAccounts(path)
	if all := b.All(); len(all) != 2 || all[0].Agent != "claude" || all[1].ACUs.Limit != 250 {
		t.Fatalf("reloaded: %+v", all)
	}
}

func TestCodexRollout(t *testing.T) {
	home := t.TempDir()
	id := "0199a213-81c0-7800-8aa1-bbab2a035a53"
	lines := []map[string]any{
		{"timestamp": "2026-10-18T20:00:00Z", "type": "session_meta", "payload": map[string]any{"id": id}},
		{"timestamp": "2026-10-18T20:00:01Z", "type": "turn_context", "payload": map[string]any{"model": "gpt-5-codex"}},
		{"timestamp": "2026-10-18T20:00:05Z", "type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{
			"total_token_usage": map[string]any{"input_tokens": 12000, "cached_input_tokens": 8000, "output_tokens": 900, "reasoning_output_tokens": 300},
			"last_token_usage":  map[string]any{"input_tokens": 12000}, "model_context_window": 272000},
			"rate_limits": map[string]any{"primary": map[string]any{"used_percent": 31.5, "window_minutes": 300, "resets_at": 1792380000},
				"secondary": map[string]any{"used_percent": 12, "window_minutes": 10080, "resets_in_seconds": 3600}}}},
	}
	var sb strings.Builder
	for _, l := range lines {
		b, _ := json.Marshal(l)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	write(t, filepath.Join(home, "sessions", "2026", "10", "18", "rollout-2026-10-18T20-00-00-"+id+".jsonl"), sb.String())
	tr, _, acc := newTestTracker(t, map[string]Terminal{"C": {Agent: "codex", Conversation: id}}, Options{CodexHome: home})
	tr.Refresh("C")
	f := tr.Flights()
	if len(f) != 1 || f[0].Tokens != (Tokens{Input: 4000, Output: 900, CacheRead: 8000}) || f[0].Models[0] != "gpt-5-codex" || f[0].Context.Size != 272000 {
		t.Fatalf("codex: %+v", f)
	}
	a, ok := acc.Get("codex")
	if !ok || len(a.Windows) != 2 || a.Windows[0].Kind != FiveHour || a.Windows[0].Used != 31.5 || a.Windows[1].Kind != SevenDay ||
		!a.Windows[1].ResetsAt.Equal(time.Date(2026, 10, 18, 21, 0, 5, 0, time.UTC)) {
		t.Fatalf("codex account: %+v", a)
	}
}

func TestGeminiSession(t *testing.T) {
	home := t.TempDir()
	id := "8f3c2a1b-1111-2222-3333-444455556666"
	doc := `{"sessionId":"` + id + `","projectHash":"x","messages":[
{"id":"u1","type":"user","content":"oi"},
{"id":"g1","type":"gemini","model":"gemini-3-pro","tokens":{"input":5000,"output":120,"cached":3000,"thoughts":40,"tool":0,"total":5160}},
{"id":"g2","type":"gemini","model":"gemini-3-pro","tokens":{"input":6000,"output":80,"cached":0,"thoughts":0,"tool":0,"total":6080}}]}`
	write(t, filepath.Join(home, "tmp", "proj", "chats", "session-2026-10-18T20-00-8f3c2a1b.json"), doc)
	// Another session with a similar name is not it.
	write(t, filepath.Join(home, "tmp", "proj", "chats", "session-2026-10-18T21-00-8f3c2a1b.json"), `{"sessionId":"other","messages":[]}`)
	tr, _, _ := newTestTracker(t, map[string]Terminal{"G": {Agent: "gemini", Conversation: id}}, Options{GeminiHome: home})
	tr.Refresh("G")
	f := tr.Flights()
	if len(f) != 1 || f[0].Tokens != (Tokens{Input: 8000, Output: 240, CacheRead: 3000}) {
		t.Fatalf("gemini: %+v", f)
	}

	// The .jsonl form: a message written again counts once (its last record).
	jl := `{"sessionId":"` + id + `","kind":"main"}
{"id":"g1","type":"gemini","tokens":{"input":100,"output":10,"cached":0,"thoughts":0}}
{"id":"g1","type":"gemini","tokens":{"input":100,"output":20,"cached":50,"thoughts":5}}
`
	home2 := t.TempDir()
	write(t, filepath.Join(home2, "tmp", "p", "chats", "session-1-8f3c2a1b.jsonl"), jl)
	tr2, _, _ := newTestTracker(t, map[string]Terminal{"G": {Agent: "gemini", Conversation: id}}, Options{GeminiHome: home2})
	tr2.Refresh("G")
	if f := tr2.Flights(); len(f) != 1 || f[0].Tokens != (Tokens{Input: 50, Output: 25, CacheRead: 50}) {
		t.Fatalf("gemini jsonl: %+v", f)
	}
}

func TestDevinSessionsDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE message_nodes (row_id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, node_id INTEGER NOT NULL, parent_node_id INTEGER, chat_message TEXT NOT NULL, created_at INTEGER NOT NULL, metadata TEXT)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('d1', 1, '{"role":"user","content":"oi","metadata":{"is_user_input":true}}', 1)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('d1', 2, '{"role":"assistant","metadata":{"metrics":{"input_tokens":1000,"output_tokens":50,"cache_read_tokens":4000,"cache_creation_tokens":200},"committed_acu_cost":0.25}}', 2)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('d1', 3, '{"role":"assistant","metadata":{"metrics":{"input_tokens":10,"output_tokens":5,"cache_read_tokens":5000,"cache_creation_tokens":0},"committed_acu_cost":0.05}}', 3)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('d2', 1, '{"role":"assistant","metadata":{"metrics":{"input_tokens":999,"output_tokens":999},"committed_acu_cost":9}}', 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	tr, _, _ := newTestTracker(t, map[string]Terminal{"D": {Agent: "devin", Conversation: "d1"}}, Options{DevinDB: path})
	tr.Refresh("D")
	f := tr.Flights()
	if len(f) != 1 || f[0].Tokens != (Tokens{Input: 1010, Output: 55, CacheRead: 9000, CacheWrite: 200}) || f[0].ACUs < 0.2999 || f[0].ACUs > 0.3001 {
		t.Fatalf("devin: %+v", f)
	}
}

func TestDevinDBPath(t *testing.T) {
	t.Setenv("DEVIN_SESSIONS_DIR", "/tmp/x")
	if p := DevinDB(); p != filepath.Join("/tmp/x", "sessions.db") {
		t.Fatalf("DEVIN_SESSIONS_DIR: %s", p)
	}
	t.Setenv("DEVIN_SESSIONS_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/data")
	if p := DevinDB(); !strings.HasSuffix(filepath.ToSlash(p), "devin/cli/sessions.db") {
		t.Fatalf("default: %s", p)
	}
}

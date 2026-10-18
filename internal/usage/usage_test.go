package usage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	mu      sync.Mutex
	flights []Flight
}

func (e *events) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.flights)
}

func newTestTracker(t *testing.T, terms map[string]Terminal, o Options) (*Tracker, *events, *Accounts) {
	ev := &events{}
	acc := OpenAccounts(filepath.Join(t.TempDir(), "accounts.json"))
	o.Accounts = acc
	o.Terminal = func(id string) (Terminal, bool) { x, ok := terms[id]; return x, ok }
	o.Publish = func(typ string, data any) {
		if fl, ok := data.(Flight); ok && typ == "usage.flight" {
			ev.mu.Lock()
			ev.flights = append(ev.flights, fl)
			ev.mu.Unlock()
		}
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Date(2026, 10, 18, 21, 0, 0, 0, time.UTC) }
	}
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
	if ev.count() != 2 {
		t.Fatalf("events: %d", ev.count())
	}
	tr.Refresh("T") // nothing new: no event
	if ev.count() != 2 {
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

// Devin 3000.11.92's sessions.db: a reply is stored once per branch of the
// session's tree (the same request_id); the session row has its cost.
func TestDevinSessionsDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	reply := func(msg, req string, in, out, cache, ctx int, at string) string {
		return fmt.Sprintf(`{"message_id":%q,"role":"assistant","content":"oi","metadata":{"num_tokens":%d,"request_id":%q,"metrics":{"ttft_ms":558,"input_tokens":%d,"output_tokens":%d,"cache_read_tokens":%d,"cache_creation_tokens":null,"context_input_tokens":%d},"finish_reason":"stop","created_at":%q,"generation_model":"swe-1-6-slow"}}`, msg, out, req, in, out, cache, ctx, at)
	}
	for _, q := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY NOT NULL, working_directory TEXT NOT NULL, backend_type TEXT, model TEXT, agent_mode TEXT, created_at INTEGER NOT NULL, last_activity_at INTEGER NOT NULL, title TEXT, main_chain_id INTEGER, metadata TEXT)`,
		`CREATE TABLE message_nodes (row_id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, node_id INTEGER NOT NULL, parent_node_id INTEGER, chat_message TEXT NOT NULL, created_at INTEGER NOT NULL, metadata TEXT)`,
		`INSERT INTO sessions VALUES ('childlike-metacarpal', '/p', 'windsurf', 'swe-1-6-slow', 'normal', 1, 2, 't', 29, '{"total_credit_cost":0,"total_acu_cost":1.25}')`,
		`INSERT INTO sessions VALUES ('other', '/p', 'windsurf', 'swe-1-6-slow', 'normal', 1, 2, 't', 1, '{"total_acu_cost":9}')`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('childlike-metacarpal', 1, '{"message_id":"u1","role":"user","content":"oi","metadata":{"is_user_input":true,"metrics":null}}', 1)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('childlike-metacarpal', 22, '` + reply("b96e", "aed1", 1084, 68, 11840, 12924, "2026-10-18T22:42:44.81Z") + `', 2)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('childlike-metacarpal', 23, '` + reply("b96e", "aed1", 1084, 68, 11840, 12924, "2026-10-18T22:42:44.81Z") + `', 2)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('childlike-metacarpal', 25, '` + reply("acab", "8cac", 46, 95, 12896, 12942, "2026-10-18T22:47:01Z") + `', 3)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('childlike-metacarpal', 28, '` + reply("ba7b", "a621", 155, 30, 12928, 13083, "2026-10-18T22:47:03Z") + `', 4)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('childlike-metacarpal', 29, '` + reply("ba7b", "a621", 155, 30, 12928, 13083, "2026-10-18T22:47:03Z") + `', 4)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('other', 1, '` + reply("x", "y", 999, 999, 0, 999, "2026-10-18T22:00:00Z") + `', 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	tr, ev, _ := newTestTracker(t, map[string]Terminal{"D": {Agent: "devin", Conversation: "childlike-metacarpal"}, "N": {Agent: "devin", Conversation: "unknown"}}, Options{DevinDB: path})
	tr.Refresh("D")
	tr.Refresh("N")
	f := tr.Flights()
	// The same totals as the session's transcript (38949 prompt tokens, 37664 of them cached, 193 out).
	if len(f) != 1 || f[0].Tokens != (Tokens{Input: 1285, Output: 193, CacheRead: 37664}) || f[0].ACUs != 1.25 || f[0].Models[0] != "swe-1-6-slow" {
		t.Fatalf("devin: %+v", f)
	}
	if c := f[0].Context; c == nil || c.Used != 13113 || c.Size != 0 {
		t.Fatalf("context without the screen: %+v", f[0].Context)
	}

	// The footer gives the window's size; the file, the exact count.
	tr.DevinScreen("D", devinScreenText)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f = tr.Flights(); f[0].Context.Size > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if c := f[0].Context; c.Used != 13113 || c.Size != 200000 || c.Percent < 6.5 || c.Percent > 6.6 {
		t.Fatalf("context with the screen: %+v", c)
	}
	if ev.count() != 2 {
		t.Fatalf("events: %d", ev.count())
	}
}

// Devin's screen after /usage and a reply (Devin 3000.11.92, a free account).
const devinScreenText = `❭ Responda apenas com a palavra: oi

 oi

❭ /usage

 Fetching account usage…

 Daily   ■■■■■■■■■■■■■■■■■■■■  4% used  · resets in 9h 15m
 Weekly  ■■■■■■■■■■■■■■■■■■■■  2% used  · resets Oct 25, 8:00 AM (UTC)

 This session
 Activity  1 turn · 0 tool calls · 0 files · 0 commands
 Tokens    13k in · 68 out  (~13k per turn)
 Model     SWE-1.6 Slow

❭ Leia o arquivo a.txt e responda apenas com o conteúdo dele

 ⏺ Read ./a.txt
 └ 1 lines

 torre consumo

────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
❭ Ask Devin to build features, fix bugs, or work on your code
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
SWE-1.6 Slow                                                                             Context: 13k / 200k tokens (6%)
`

func TestDevinUsageScreen(t *testing.T) {
	now := time.Date(2026, 10, 18, 22, 44, 50, 0, time.UTC)
	tr, _, acc := newTestTracker(t, map[string]Terminal{"D": {Agent: "devin"}}, Options{Now: func() time.Time { return now }})
	var sets int
	acc.Subscribe(func(Account) { sets++ })
	tr.DevinScreen("D", devinScreenText)
	a, ok := acc.Get("devin")
	if !ok || len(a.Windows) != 2 || a.ACUs != nil {
		t.Fatalf("account: %+v", a)
	}
	if w := a.Windows[0]; w.Kind != Daily || w.Used != 4 || !w.ResetsAt.Equal(time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("daily: %+v", w)
	}
	if w := a.Windows[1]; w.Kind != Weekly || w.Used != 2 || !w.ResetsAt.Equal(time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekly: %+v", w)
	}
	// The same block again (the screen changed elsewhere): read once.
	now = now.Add(5 * time.Minute)
	tr.DevinScreen("D", devinScreenText+"x")
	if sets != 1 {
		t.Fatalf("set %d times", sets)
	}

	// Still fetching: nothing; a company account: no quota, its ACUs.
	tr.DevinScreen("D", "❭ /usage\n\n Fetching account usage…\n")
	if sets != 1 {
		t.Fatalf("set while fetching")
	}
	tr.DevinScreen("D", "❭ /usage\n\n Fetching account usage…\n\n No quota data available.\n 1,234.5 ACUs consumed\n ACU limit 2,000\n\n This session\n ACUs      0.75 ACUs consumed\n")
	a, _ = acc.Get("devin")
	if len(a.Windows) != 0 || a.ACUs == nil || a.ACUs.Used != 1234.5 || a.ACUs.Limit != 2000 {
		t.Fatalf("company account: %+v %+v", a, a.ACUs)
	}
	tr.DevinScreen("D", "❭ /usage\n Fetching account usage…\n No quota data available.\n This session\n No ACUs consumed yet in this session.\n")
	if a, _ = acc.Get("devin"); len(a.Windows) != 0 || a.ACUs != nil {
		t.Fatalf("no quota: %+v", a)
	}
}

func TestDevinResets(t *testing.T) {
	now := time.Date(2026, 12, 30, 10, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"in 2d 3h":                  now.Add(51 * time.Hour),
		"in 45m":                    now.Add(45 * time.Minute),
		"Jan 2, 8:00 AM (UTC)":      time.Date(2027, 1, 2, 8, 0, 0, 0, time.UTC),
		"Dec 31, 11:30 PM (-03:00)": time.Date(2027, 1, 1, 2, 30, 0, 0, time.UTC),
		"":                          {},
		"soon":                      {},
	} {
		if got := devinResets(in, now); !got.Equal(want) {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
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

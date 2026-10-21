package usage

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var reportNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// protobuf fields, for the tests' payloads.
func pbVarint(num, v uint64) []byte {
	b := binary.AppendUvarint(nil, num<<3)
	return binary.AppendUvarint(b, v)
}

func pbDouble(num uint64, f float64) []byte {
	b := binary.AppendUvarint(nil, num<<3|1)
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(f))
}

func pbBytes(num uint64, data ...[]byte) []byte {
	d := bytes.Join(data, nil)
	b := binary.AppendUvarint(nil, num<<3|2)
	b = binary.AppendUvarint(b, uint64(len(d)))
	return append(b, d...)
}

func writeAt(t *testing.T, path, data string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// Devin 3000.11.92 keeps the account's status in JSON with the server's
// protobuf reply in base64 (user_status.<digest>.bin). The report shows its
// numbers, the dates they hold, one item of each list, and no text.
func TestReportDevinCache(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".cache", "devin", "cli")
	model := pbBytes(1, pbBytes(1, []byte("swe-1-6")), pbVarint(13, 128000))
	payload := bytes.Join([][]byte{
		pbBytes(3, []byte("Devin Enterprise")),
		pbBytes(5, []byte("someone@example.com")),
		pbBytes(13,
			pbVarint(14, 96), pbVarint(15, 98),
			pbVarint(17, 1792396800), pbVarint(18, 1792915200),
			pbDouble(19, 599.41), pbBytes(22, []byte("2026-10-01"))),
		pbBytes(33, model, model, model),
	}, nil)
	file := fmt.Sprintf(`{"version":1,"identity_digest":"%s","fetched_at_secs":%d,"payload":%q}`,
		strings.Repeat("ab", 32), reportNow.Add(-90*time.Minute).Unix(), base64.StdEncoding.EncodeToString(payload))
	writeAt(t, filepath.Join(cache, "user_status.3fda6d47f1a2f1af.bin"), file, reportNow.Add(-90*time.Minute))
	writeAt(t, filepath.Join(cache, "team_settings.3fda6d47f1a2f1af.bin"), `{"secret":"x"}`, reportNow)

	var out bytes.Buffer
	Report(&out, ReportOptions{Now: reportNow, Home: home, DevinDB: filepath.Join(home, "nope", "sessions.db"), DevinDirs: []string{cache, filepath.Join(home, "missing")}})
	s := out.String()
	for _, want := range []string{
		"sessions.db: not found (" + filepath.Join("~", "nope", "sessions.db") + ")",
		filepath.Join("~", ".cache", "devin", "cli", "user_status.3fda6d47f1a2f1af.bin") + ": ",
		"team_settings.3fda6d47f1a2f1af.bin: 14 bytes",
		"JSON with fetched_at_secs, identity_digest, payload, version",
		"(1h30m ago)",
		"protobuf:",
		"3: (text, 16 chars)",
		"5: (text, 19 chars)",
		"14: 96 (varint)",
		"17: 1792396800 (varint) = 2026-10-19 08:00 UTC",
		"19: 599.41 (double)",
		`22: "2026-10-01" (text)`,
		"13: 128000 (varint)",
		"1 {…} × 2 more",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	for _, secret := range []string{"someone@example.com", "Enterprise", "swe-1-6", "abab", "secret"} {
		if strings.Contains(s, secret) {
			t.Errorf("the report shows %q:\n%s", secret, s)
		}
	}
}

// A file that isn't protobuf still shows what looks like a cost or a date.
func TestReportBinaryNumbers(t *testing.T) {
	b := make([]byte, 24)
	b = binary.LittleEndian.AppendUint64(b, math.Float64bits(599.41))
	b = binary.LittleEndian.AppendUint64(b, uint64(1792396800))
	lines := strings.Join(binaryNumbers(b), "\n")
	if !strings.HasPrefix(lines, "not protobuf") || !strings.Contains(lines, "24: 599.41 (double)") || !strings.Contains(lines, "32: 1792396800 (int64) = 2026-10-19 08:00 UTC") {
		t.Fatalf("binary numbers:\n%s", lines)
	}
}

// The sessions.db part: the sessions active this month and whether each
// reply keeps its own cost (counted once per request, by when it was made).
func TestReportDevinDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	reply := func(req, at, cost string) string {
		c := ""
		if cost != "" {
			c = `,"committed_acu_cost":` + cost + `,"committed_credit_cost":0`
		}
		return fmt.Sprintf(`{"message_id":"m-%s","role":"assistant","content":"segredo","metadata":{"request_id":%q,"metrics":{"input_tokens":1},"created_at":%q%s}}`, req, req, at, c)
	}
	oct, sep := reportNow.Add(-24*time.Hour).UnixMilli(), reportNow.AddDate(0, -1, 0).UnixMilli()
	for _, q := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY NOT NULL, working_directory TEXT NOT NULL, backend_type TEXT, model TEXT, agent_mode TEXT, created_at INTEGER NOT NULL, last_activity_at INTEGER NOT NULL, title TEXT, main_chain_id INTEGER, metadata TEXT)`,
		`CREATE TABLE message_nodes (row_id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, node_id INTEGER NOT NULL, parent_node_id INTEGER, chat_message TEXT NOT NULL, created_at INTEGER NOT NULL, metadata TEXT)`,
		fmt.Sprintf(`INSERT INTO sessions VALUES ('a', '/p', 'windsurf', 'm', 'normal', %d, %d, 't', 1, '{"total_acu_cost":38.4671}')`, sep, oct),
		fmt.Sprintf(`INSERT INTO sessions VALUES ('b', '/p', 'windsurf', 'm', 'normal', %d, %d, 't', 1, '{"total_acu_cost":2}')`, sep, sep),
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('a', 1, '{"message_id":"u","role":"user","content":"oi","metadata":{"metrics":null}}', 1)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('a', 2, '` + reply("r1", "2026-09-30T23:00:00Z", "1.5") + `', 2)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('a', 3, '` + reply("r2", "2026-10-20T10:00:00.5Z", "0.25") + `', 3)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('a', 4, '` + reply("r2", "2026-10-20T10:00:00.5Z", "0.25") + `', 3)`,
		`INSERT INTO message_nodes (session_id, node_id, chat_message, created_at) VALUES ('b', 1, '` + reply("r3", "2026-10-20T11:00:00Z", "") + `', 4)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	var out bytes.Buffer
	if err := reportDevinDB(&out, path, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"sessions: 2 (1 active this month)",
		"session ACUs (total_acu_cost): all 40.4671 · active this month 38.4671",
		"replies: 3, 2 of them with their own cost (committed_acu_cost)",
		"reply ACUs: all 1.75 · made this month 0.25 · oldest ",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "segredo") {
		t.Errorf("the report shows a message:\n%s", s)
	}
}

// Claude Code: the conversations changed this month, which of them have
// cost-state lines, their versions, the cost of the runs started this month
// (a copied transcript counts once) and the provider.
func TestReportClaude(t *testing.T) {
	home := t.TempDir()
	ch := filepath.Join(home, ".claude")
	state := func(session string, start time.Time, cost float64) string {
		return fmt.Sprintf(`{"type":"cost-state","sessionId":%q,"startTime":%d,"totalCostUSD":%g}`+"\n", session, start.UnixMilli(), cost)
	}
	conv := `{"type":"user","version":"2.1.236","message":{"content":"o \"version\":\"9.9.9\" do pacote"}}` + "\n"
	run := reportNow.Add(-48 * time.Hour)
	writeAt(t, filepath.Join(ch, "projects", "-p", "s1.jsonl"), conv+state("s1", run, 0.5)+state("s1", run, 1.25), reportNow)
	writeAt(t, filepath.Join(ch, "projects", "-p--floors-x", "s1.jsonl"), conv+state("s1", run, 1.25), reportNow) // a floor's copy
	writeAt(t, filepath.Join(ch, "projects", "-p", "s2.jsonl"), conv+state("s2", reportNow.AddDate(0, -1, 0), 7), reportNow)
	writeAt(t, filepath.Join(ch, "projects", "-p", "s3.jsonl"), `{"type":"user","version":"2.1.240"}`+"\n", reportNow)
	writeAt(t, filepath.Join(ch, "projects", "-p", "old.jsonl"), conv+state("old", reportNow.AddDate(0, -2, 0), 9), reportNow.AddDate(0, -2, 0))
	writeAt(t, filepath.Join(ch, "settings.json"), `{"env":{"CLAUDE_CODE_USE_BEDROCK":"1","AWS_REGION":"us-east-1"}}`, reportNow)
	managed := filepath.Join(home, "managed-settings.json")
	writeAt(t, managed, `{"env":{"ANTHROPIC_BEDROCK_BASE_URL":"https://gateway.example.com"}}`, reportNow)

	var out bytes.Buffer
	env := map[string]string{"ANTHROPIC_BASE_URL": "https://proxy.example.com"}
	Report(&out, ReportOptions{Now: reportNow, Home: home, ClaudeHome: ch, ClaudeSettings: []string{filepath.Join(ch, "settings.json"), managed, filepath.Join(home, "none.json")}, Getenv: func(k string) string { return env[k] }})
	s := out.String()
	for _, want := range []string{
		"folder: " + filepath.Join("~", ".claude"),
		"conversations: 5 (4 changed this month)",
		"changed this month with a cost-state line: 3 of 4",
		"Claude Code versions in them: 2.1.236 (3), 2.1.240 (1)",
		"estimated cost of the runs started this month (cost-state): US$ 1.25 in 1 runs",
		"provider: ANTHROPIC_BASE_URL in the environment",
		"provider: CLAUDE_CODE_USE_BEDROCK in " + filepath.Join("~", ".claude", "settings.json"),
		"provider: ANTHROPIC_BEDROCK_BASE_URL in " + filepath.Join("~", "managed-settings.json"),
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "example.com") || strings.Contains(s, "9.9.9") {
		t.Errorf("the report shows a URL or a message:\n%s", s)
	}
}

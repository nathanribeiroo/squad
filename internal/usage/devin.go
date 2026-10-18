package usage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Devin's session database (pure Go: no C compiler on Windows)
)

// Devin CLI keeps its sessions in a SQLite database, sessions.db, in its
// data folder: ~/.local/share/devin/cli on macOS and Linux
// ($XDG_DATA_HOME/devin/cli), %APPDATA%\devin\cli on Windows;
// DEVIN_SESSIONS_DIR moves it. Each model reply (message_nodes.chat_message)
// carries the request's tokens; the session (sessions.metadata) its cost in
// ACUs or credits. Torre opens it read-only.

// DevinDB is the path of Devin's sessions.db.
func DevinDB() string {
	if d := os.Getenv("DEVIN_SESSIONS_DIR"); d != "" {
		return filepath.Join(d, "sessions.db")
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "devin", "cli", "sessions.db")
		}
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "devin", "cli", "sessions.db")
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".local", "share", "devin", "cli", "sessions.db")
}

// readDevin sums a session's model requests. A message shows up once per
// branch of the session's tree, so each request counts once (request_id).
// The session row keeps its ACUs and credits.
func readDevin(path, session string) (Flight, error) {
	if path == "" || session == "" {
		return Flight{}, errNoData
	}
	if _, err := os.Stat(path); err != nil {
		return Flight{}, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro&_pragma=busy_timeout(2000)"}
	if runtime.GOOS == "windows" {
		u.Path = "/" + u.Path // file:///C:/…
	}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return Flight{}, err
	}
	defer db.Close()
	fl := Flight{Agent: "devin"}
	var model, meta sql.NullString
	switch err := db.QueryRow(`SELECT model, metadata FROM sessions WHERE id = ?1`, session).Scan(&model, &meta); {
	case errors.Is(err, sql.ErrNoRows):
		return Flight{}, errNoData
	case err != nil:
		return Flight{}, err
	}
	var cost struct {
		ACUs    float64 `json:"total_acu_cost"`
		Credits float64 `json:"total_credit_cost"`
	}
	if meta.Valid {
		_ = json.Unmarshal([]byte(meta.String), &cost)
	}
	fl.ACUs, fl.Credits = cost.ACUs, cost.Credits
	if model.String != "" {
		fl.Models = []string{model.String}
	}
	rows, err := db.Query(`SELECT chat_message FROM message_nodes WHERE session_id = ?1 ORDER BY node_id`, session)
	if err != nil {
		return Flight{}, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var last time.Time
	for rows.Next() {
		var msg string
		if rows.Scan(&msg) != nil {
			continue
		}
		var m devinMessage
		if json.Unmarshal([]byte(msg), &m) != nil || m.Metadata.Metrics == nil {
			continue
		}
		md, mt := m.Metadata, m.Metadata.Metrics
		key := md.RequestID
		if key == "" {
			key = m.MessageID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		fl.Tokens = fl.Tokens.Add(Tokens{Input: mt.Input, Output: mt.Output, CacheRead: mt.CacheRead, CacheWrite: mt.CacheWrite})
		if !md.CreatedAt.Before(last) && mt.Context > 0 {
			last = md.CreatedAt
			fl.Context = &Context{Used: mt.Context + mt.Output}
		}
		if !md.CreatedAt.IsZero() && md.CreatedAt.After(fl.At) {
			fl.At = md.CreatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return Flight{}, err
	}
	if len(seen) == 0 && fl.ACUs == 0 && fl.Credits == 0 {
		return Flight{}, errNoData
	}
	return fl, nil
}

type devinMessage struct {
	MessageID string `json:"message_id"`
	Metadata  struct {
		RequestID string    `json:"request_id"`
		CreatedAt time.Time `json:"created_at"`
		Metrics   *struct {
			Input      int64 `json:"input_tokens"` // without the cache
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_tokens"`
			CacheWrite int64 `json:"cache_creation_tokens"`
			Context    int64 `json:"context_input_tokens"` // the whole prompt
		} `json:"metrics"`
	} `json:"metadata"`
}

// Devin's screen tells what its files don't: the context window's size,
// in the footer after a reply ("Context: 13k / 200k tokens (6%)"), and the
// account's quota, when the user opens /usage:
//
//	❭ /usage
//	 Fetching account usage…
//	 Daily   ■■■■■■■■■■■■■■■■■■■■  4% used  · resets in 9h 17m
//	 Weekly  ■■■■■■■■■■■■■■■■■■■■  2% used  · resets Oct 25, 8:00 AM (UTC)
//	 This session
//	 …
//
// An account without a quota says "No quota data available."; one billed
// in ACUs shows them (" ACUs consumed", "ACU limit ").

var (
	devinFooter = regexp.MustCompile(`Context:\s*([\d.,]+)([kM]?)\s*/\s*([\d.,]+)([kM]?)\s*tokens\s*\((\d+(?:\.\d+)?)%\)`)
	devinWindow = regexp.MustCompile(`^\s*(Daily|Weekly)\b.*?(\d+(?:\.\d+)?)%\s*(used|remaining)(?:.*?resets\s+(.+?))?\s*$`)
	devinACUs   = regexp.MustCompile(`(?i)([\d][\d.,]*)\s*ACUs?\s+consumed|ACUs?\s+consumed\W*([\d][\d.,]*)`)
	devinLimit  = regexp.MustCompile(`(?i)ACU\s+limit\W*([\d][\d.,]*)`)
	devinIn     = regexp.MustCompile(`(\d+)\s*([dhms])`)
	devinZone   = regexp.MustCompile(`\s*\(([^)]*)\)\s*$`)
)

// devinContext reads the footer's context window.
func devinContext(snap string) (Context, bool) {
	m := devinFooter.FindAllStringSubmatch(snap, -1)
	if len(m) == 0 {
		return Context{}, false
	}
	f := m[len(m)-1]
	used, size := devinNumber(f[1], f[2]), devinNumber(f[3], f[4])
	pct, _ := strconv.ParseFloat(f[5], 64)
	if size <= 0 {
		return Context{}, false
	}
	return Context{Used: used, Size: size, Percent: pct}, true
}

func devinNumber(n, unit string) int64 {
	f, err := strconv.ParseFloat(strings.ReplaceAll(n, ",", ""), 64)
	if err != nil {
		return 0
	}
	switch unit {
	case "k":
		f *= 1e3
	case "M":
		f *= 1e6
	}
	return int64(f)
}

// devinUsageBlock is the last /usage on the screen, up to the prompt box.
func devinUsageBlock(snap string) string {
	i := strings.LastIndex(snap, "Fetching account usage")
	if i < 0 {
		return ""
	}
	b := snap[i:]
	if j := strings.Index(b, "────"); j >= 0 {
		b = b[:j]
	}
	return b
}

// devinAccount reads the account's quota in a /usage block.
func devinAccount(block string, now time.Time) (Account, bool) {
	a := Account{Agent: "devin", Seen: now}
	found := false
	for _, line := range strings.Split(block, "\n") {
		t := strings.TrimSpace(line)
		if t == "This session" {
			break // what follows is the session's, not the account's
		}
		if strings.HasPrefix(t, "No quota data available") {
			found = true
			continue
		}
		if m := devinWindow.FindStringSubmatch(line); m != nil {
			used, _ := strconv.ParseFloat(m[2], 64)
			if m[3] == "remaining" {
				used = 100 - used
			}
			kind := Daily
			if m[1] == "Weekly" {
				kind = Weekly
			}
			a.Windows = append(a.Windows, Window{Kind: kind, Used: used, ResetsAt: devinResets(m[4], now)})
			found = true
			continue
		}
		if m := devinLimit.FindStringSubmatch(t); m != nil {
			if a.ACUs == nil {
				a.ACUs = &ACUs{}
			}
			a.ACUs.Limit = devinFloat(m[1])
			found = true
		}
		if m := devinACUs.FindStringSubmatch(t); m != nil {
			if a.ACUs == nil {
				a.ACUs = &ACUs{}
			}
			a.ACUs.Used = devinFloat(m[1] + m[2])
			found = true
		}
	}
	return a, found
}

func devinFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return f
}

// devinResets reads "in 9h 17m" or "Oct 25, 8:00 AM (UTC)".
func devinResets(s string, now time.Time) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if rest, ok := strings.CutPrefix(s, "in "); ok {
		var d time.Duration
		for _, m := range devinIn.FindAllStringSubmatch(rest, -1) {
			n, _ := strconv.Atoi(m[1])
			d += time.Duration(n) * map[string]time.Duration{"d": 24 * time.Hour, "h": time.Hour, "m": time.Minute, "s": time.Second}[m[2]]
		}
		if d == 0 {
			return time.Time{}
		}
		// The screen drops the seconds: the nearest minute to the middle
		// of the one it shows.
		return now.Add(d + 29*time.Second).Round(time.Minute).UTC()
	}
	loc := time.Local
	if m := devinZone.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
		if z, err := time.Parse("-07:00", m[1]); err == nil {
			loc = z.Location()
		} else if m[1] == "UTC" || m[1] == "GMT" {
			loc = time.UTC
		}
	}
	for _, layout := range []string{"Jan 2, 3:04 PM", "Jan 2, 15:04", "Jan 2 3:04 PM", "Jan 2, 2006, 3:04 PM"} {
		t, err := time.ParseInLocation(layout, s, loc)
		if err != nil {
			continue
		}
		if t.Year() == 0 {
			t = t.AddDate(now.In(loc).Year(), 0, 0)
			if t.Before(now.Add(-24 * time.Hour)) {
				t = t.AddDate(1, 0, 0)
			}
		}
		return t.UTC()
	}
	return time.Time{}
}

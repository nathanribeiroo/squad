package usage

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Report is `torre usage-report`: what Torre can read about the agents'
// usage on this computer, to send when the Consumo box doesn't add up and
// to map the files of company accounts. It prints numbers only: the files
// also hold the user's e-mail, team and conversations, and no text from
// them is printed (paths start at ~).

// ReportOptions says where to look; DefaultReportOptions is this computer.
type ReportOptions struct {
	Now        time.Time
	Home       string // printed as ~
	DevinDB    string
	DevinDirs  []string // where Devin may keep its caches (user_status.bin)
	ClaudeHome string
	// ClaudeSettings are the settings files that can turn on Bedrock: the
	// user's, then the company's (managed).
	ClaudeSettings []string
	Getenv         func(string) string
}

// DefaultReportOptions looks where the agents keep their files here.
func DefaultReportOptions() ReportOptions {
	home, _ := os.UserHomeDir()
	o := ReportOptions{Now: time.Now(), Home: home, DevinDB: DevinDB(), ClaudeHome: ClaudeHome(), Getenv: os.Getenv}
	dirs := []string{filepath.Dir(o.DevinDB)}
	add := func(base string, elem ...string) {
		if base != "" {
			dirs = append(dirs, filepath.Join(append([]string{base}, elem...)...))
		}
	}
	add(os.Getenv("XDG_CACHE_HOME"), "devin")
	add(os.Getenv("XDG_CONFIG_HOME"), "devin")
	add(os.Getenv("APPDATA"), "devin")
	add(os.Getenv("LOCALAPPDATA"), "devin")
	add(home, ".cache", "devin")
	add(home, ".config", "devin")
	add(home, ".local", "share", "devin")
	if d, err := os.UserCacheDir(); err == nil {
		add(d, "devin") // ~/Library/Caches on macOS
	}
	if d, err := os.UserConfigDir(); err == nil {
		add(d, "devin") // ~/Library/Application Support on macOS
	}
	o.DevinDirs = dirs
	if o.ClaudeHome != "" {
		o.ClaudeSettings = append(o.ClaudeSettings, filepath.Join(o.ClaudeHome, "settings.json"))
	}
	switch runtime.GOOS {
	case "darwin":
		o.ClaudeSettings = append(o.ClaudeSettings, "/Library/Application Support/ClaudeCode/managed-settings.json")
	case "windows":
		o.ClaudeSettings = append(o.ClaudeSettings, `C:\Program Files\ClaudeCode\managed-settings.json`, `C:\ProgramData\ClaudeCode\managed-settings.json`)
	default:
		o.ClaudeSettings = append(o.ClaudeSettings, "/etc/claude-code/managed-settings.json")
	}
	return o
}

// Report writes the report.
func Report(w io.Writer, o ReportOptions) {
	if o.Getenv == nil {
		o.Getenv = func(string) string { return "" }
	}
	since := time.Date(o.Now.Year(), o.Now.Month(), 1, 0, 0, 0, 0, o.Now.Location())
	fmt.Fprintf(w, "Numbers only: no text from the files (no e-mail, team or conversation).\n")
	fmt.Fprintf(w, "Now: %s · this month since %s\n\n", o.Now.Format("2006-01-02 15:04 -07:00"), since.Format("Jan 2"))
	reportDevin(w, o, since)
	fmt.Fprintln(w)
	reportClaude(w, o, since)
}

func (o ReportOptions) short(p string) string {
	if o.Home != "" {
		if rel, err := filepath.Rel(o.Home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return p
}

func (o ReportOptions) ago(t time.Time) string {
	d := o.Now.Sub(t).Round(time.Minute)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm ago", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

// num prints a cost with up to four decimals.
func num(f float64) string {
	s := strconv.FormatFloat(f, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		s = "0"
	}
	return s
}

// ---- Devin

func reportDevin(w io.Writer, o ReportOptions, since time.Time) {
	fmt.Fprintln(w, "Devin")
	if o.DevinDB == "" {
		fmt.Fprintln(w, "  sessions.db: no folder")
	} else if _, err := os.Stat(o.DevinDB); err != nil {
		fmt.Fprintf(w, "  sessions.db: not found (%s)\n", o.short(o.DevinDB))
	} else {
		fmt.Fprintf(w, "  sessions.db: %s\n", o.short(o.DevinDB))
		if err := reportDevinDB(w, o.DevinDB, since); err != nil {
			fmt.Fprintf(w, "    could not read it: %v\n", err)
		}
	}
	files := devinCaches(o.DevinDirs)
	if len(files) == 0 {
		fmt.Fprintln(w, "  user_status.bin: not found in:")
		for _, d := range uniqueDirs(o.DevinDirs) {
			fmt.Fprintf(w, "    %s\n", o.short(d))
		}
		return
	}
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "  %s: %d bytes, changed %s (%s)\n", o.short(f), fi.Size(), fi.ModTime().Format("2006-01-02 15:04"), o.ago(fi.ModTime()))
		if !strings.HasPrefix(filepath.Base(f), "user_status.") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(w, "    could not read it: %v\n", err)
			continue
		}
		for _, l := range o.devinCache(b) {
			fmt.Fprintf(w, "    %s\n", l)
		}
	}
}

// devinCache reads a Devin cache file: JSON with the time it was fetched
// and the server's reply, in base64 (Devin 3000.11.92); anything else is
// read as binary.
func (o ReportOptions) devinCache(b []byte) []string {
	var env struct {
		Version   *int   `json:"version"`
		FetchedAt int64  `json:"fetched_at_secs"`
		Payload   string `json:"payload"`
	}
	if json.Unmarshal(b, &env) != nil || env.Payload == "" {
		return binaryNumbers(b)
	}
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(b, &keys)
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	slices.Sort(names)
	out := []string{"JSON with " + strings.Join(names, ", ")}
	if env.Version != nil {
		out = append(out, fmt.Sprintf("version: %d", *env.Version))
	}
	if env.FetchedAt > 0 {
		at := time.Unix(env.FetchedAt, 0)
		out = append(out, fmt.Sprintf("fetched_at_secs: %s (%s)", at.Local().Format("2006-01-02 15:04"), o.ago(at)))
	}
	var payload []byte
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if p, err := enc.DecodeString(env.Payload); err == nil {
			payload = p
			break
		}
	}
	if payload == nil {
		return append(out, fmt.Sprintf("payload: %d chars, not base64", len(env.Payload)))
	}
	out = append(out, fmt.Sprintf("payload: %d bytes in base64, as", len(payload)))
	return append(out, binaryNumbers(payload)...)
}

func uniqueDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if d != "" && d != "." && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// devinCaches finds Devin's .bin caches (user_status.bin and its
// neighbours) in dirs, two levels down at most.
func devinCaches(dirs []string) []string {
	var out []string
	for _, d := range uniqueDirs(dirs) {
		root := filepath.Clean(d)
		filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				rel, _ := filepath.Rel(root, p)
				if rel != "." && (strings.Count(rel, string(filepath.Separator)) >= 1 || slices.Contains([]string{"_versions", "versions", "logs", "agent_logs", "session_exports", "transcripts"}, e.Name())) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(e.Name(), ".bin") && !slices.Contains(out, p) {
				out = append(out, p)
			}
			return nil
		})
	}
	return out
}

// reportDevinDB counts the sessions and their ACUs, and whether each reply
// keeps its own cost (committed_acu_cost) with the time it was made.
func reportDevinDB(w io.Writer, path string, since time.Time) error {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro&_pragma=busy_timeout(2000)"}
	if runtime.GOOS == "windows" {
		u.Path = "/" + u.Path
	}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT last_activity_at, metadata FROM sessions`)
	if err != nil {
		return err
	}
	var n, recent int
	var acus, recentACUs, credits, recentCredits float64
	for rows.Next() {
		var last sql.NullInt64
		var meta sql.NullString
		if rows.Scan(&last, &meta) != nil {
			continue
		}
		n++
		var c struct {
			ACUs    float64 `json:"total_acu_cost"`
			Credits float64 `json:"total_credit_cost"`
		}
		if meta.Valid {
			_ = json.Unmarshal([]byte(meta.String), &c)
		}
		acus += c.ACUs
		credits += c.Credits
		if !epoch(last.Int64).Before(since) {
			recent++
			recentACUs += c.ACUs
			recentCredits += c.Credits
		}
	}
	rows.Close()
	fmt.Fprintf(w, "    sessions: %d (%d active this month)\n", n, recent)
	fmt.Fprintf(w, "    session ACUs (total_acu_cost): all %s · active this month %s\n", num(acus), num(recentACUs))
	if credits > 0 {
		fmt.Fprintf(w, "    session credits (total_credit_cost): all %s · active this month %s\n", num(credits), num(recentCredits))
	}

	// A reply shows up once per branch of the session's tree: each request
	// counts once.
	rows, err = db.Query(`SELECT json_extract(chat_message, '$.metadata.request_id'),
		json_extract(chat_message, '$.message_id'),
		json_extract(chat_message, '$.metadata.created_at'),
		json_type(chat_message, '$.metadata.committed_acu_cost'),
		json_extract(chat_message, '$.metadata.committed_acu_cost'),
		json_extract(chat_message, '$.metadata.committed_credit_cost')
		FROM message_nodes
		WHERE json_valid(chat_message) AND json_type(chat_message, '$.metadata.metrics') = 'object'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var replies, withCost int
	var sum, recentSum, recentCreditSum float64
	var first time.Time
	for rows.Next() {
		var req, msg, at, kind sql.NullString
		var acu, credit sql.NullFloat64
		if rows.Scan(&req, &msg, &at, &kind, &acu, &credit) != nil {
			continue
		}
		key := req.String
		if key == "" {
			key = msg.String
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		replies++
		if !kind.Valid || kind.String == "null" { // the key isn't there
			continue
		}
		withCost++
		sum += acu.Float64
		t, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil {
			continue
		}
		if first.IsZero() || t.Before(first) {
			first = t
		}
		if !t.Before(since) {
			recentSum += acu.Float64
			recentCreditSum += credit.Float64
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	fmt.Fprintf(w, "    replies: %d, %d of them with their own cost (committed_acu_cost)\n", replies, withCost)
	if withCost > 0 {
		fmt.Fprintf(w, "    reply ACUs: all %s · made this month %s", num(sum), num(recentSum))
		if recentCreditSum > 0 {
			fmt.Fprintf(w, " (credits %s)", num(recentCreditSum))
		}
		fmt.Fprintf(w, " · oldest %s\n", first.Local().Format("2006-01-02"))
	}
	return nil
}

// epoch reads seconds or milliseconds.
func epoch(n int64) time.Time {
	if n > 1e12 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}

// ---- Claude

var claudeVersion = regexp.MustCompile(`"version":"(\d+\.\d+\.\d+)`)

func reportClaude(w io.Writer, o ReportOptions, since time.Time) {
	fmt.Fprintln(w, "Claude Code")
	if o.ClaudeHome == "" {
		fmt.Fprintln(w, "  no folder")
		return
	}
	fmt.Fprintf(w, "  folder: %s\n", o.short(o.ClaudeHome))
	files, _ := filepath.Glob(filepath.Join(o.ClaudeHome, "projects", "*", "*.jsonl"))
	var recent []string
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil && !fi.ModTime().Before(since) {
			recent = append(recent, f)
		}
	}
	fmt.Fprintf(w, "  conversations: %d (%d changed this month)\n", len(files), len(recent))

	versions := map[string]int{}
	costs := map[string]float64{} // the last total of each run (session and start)
	var withState int
	for _, f := range recent {
		v, has := scanClaude(f, since, costs)
		if v != "" {
			versions[v]++
		}
		if has {
			withState++
		}
	}
	if len(recent) > 0 {
		fmt.Fprintf(w, "  changed this month with a cost-state line: %d of %d\n", withState, len(recent))
		var vs []string
		for v, n := range versions {
			vs = append(vs, fmt.Sprintf("%s (%d)", v, n))
		}
		slices.Sort(vs)
		fmt.Fprintf(w, "  Claude Code versions in them: %s\n", strings.Join(vs, ", "))
		var total float64
		for _, c := range costs {
			total += c
		}
		fmt.Fprintf(w, "  estimated cost of the runs started this month (cost-state): US$ %s in %d runs\n", num(total), len(costs))
	}
	reportClaudeProvider(w, o)
}

// scanClaude reads a transcript's version and its cost-state lines (a run's
// running total, by session and start; a copied transcript counts once).
func scanClaude(path string, since time.Time, costs map[string]float64) (version string, hasState bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && len(line) <= maxLine {
			if version == "" {
				if m := claudeVersion.FindSubmatch(line); m != nil {
					version = string(m[1])
				}
			}
			if bytes.Contains(line, []byte(`"cost-state"`)) {
				var c struct {
					Type      string  `json:"type"`
					SessionID string  `json:"sessionId"`
					Start     int64   `json:"startTime"`
					Total     float64 `json:"totalCostUSD"`
				}
				if json.Unmarshal(line, &c) == nil && c.Type == "cost-state" {
					hasState = true
					if !epoch(c.Start).Before(since) {
						k := c.SessionID + "/" + strconv.FormatInt(c.Start, 10)
						costs[k] = max(costs[k], c.Total)
					}
				}
			}
		}
		if err != nil {
			return version, hasState
		}
	}
}

// reportClaudeProvider says where Claude Code sends its requests: Bedrock,
// Vertex or a gateway (the URLs aren't printed).
func reportClaudeProvider(w io.Writer, o ReportOptions) {
	vars := []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "ANTHROPIC_BASE_URL", "ANTHROPIC_BEDROCK_BASE_URL"}
	var found []string
	for _, v := range vars {
		if o.Getenv(v) != "" {
			found = append(found, v+" in the environment")
		}
	}
	for _, p := range o.ClaudeSettings {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s struct {
			Env map[string]any `json:"env"`
		}
		if json.Unmarshal(b, &s) != nil {
			found = append(found, o.short(p)+" is not valid JSON")
			continue
		}
		for _, v := range vars {
			if x, ok := s.Env[v]; ok && fmt.Sprint(x) != "" {
				found = append(found, v+" in "+o.short(p))
			}
		}
	}
	if len(found) == 0 {
		fmt.Fprintln(w, "  provider: no sign of Bedrock, Vertex or a gateway")
		return
	}
	for _, f := range found {
		fmt.Fprintf(w, "  provider: %s\n", f)
	}
}

// ---- binary files

// binaryNumbers shows a binary file's numbers: its protobuf fields when it
// is one (the field number and its value; text only as its length), or else
// the values that look like costs and dates, with their position.
func binaryNumbers(b []byte) []string {
	for skip := 0; skip <= 16 && skip < len(b); skip++ {
		fields, ok := parseProto(b[skip:], 0)
		if !ok || len(fields) < 2 {
			continue
		}
		var out []string
		if skip > 0 {
			out = append(out, fmt.Sprintf("protobuf after a %d-byte header:", skip))
			for _, l := range scanNumbers(b[:skip], 0) {
				out = append(out, "  header "+l)
			}
		} else {
			out = append(out, "protobuf:")
		}
		return append(out, printProto(fields, "  ", 0)...)
	}
	out := []string{"not protobuf; numbers that look like values (byte: value):"}
	return append(out, scanNumbers(b, 200)...)
}

type protoField struct {
	num  uint64
	wire int
	v    uint64 // varint, fixed64, fixed32
	data []byte
	sub  []protoField // data read as a message
}

func parseProto(b []byte, depth int) ([]protoField, bool) {
	var out []protoField
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, false
		}
		b = b[n:]
		f := protoField{num: tag >> 3, wire: int(tag & 7)}
		if f.num == 0 || f.num > 1<<29-1 {
			return nil, false
		}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return nil, false
			}
			f.v, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return nil, false
			}
			f.v, b = binary.LittleEndian.Uint64(b), b[8:]
		case 5:
			if len(b) < 4 {
				return nil, false
			}
			f.v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return nil, false
			}
			f.data, b = b[n:n+int(l)], b[n+int(l):]
			if depth < 10 && len(f.data) > 0 && !isText(f.data) {
				if sub, ok := parseProto(f.data, depth+1); ok {
					f.sub = sub
				}
			}
		default:
			return nil, false
		}
		out = append(out, f)
	}
	return out, true
}

// isText: printable UTF-8 (a field's text, not a message).
func isText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// plainNumber is text safe to print: a number or a date.
var plainNumber = regexp.MustCompile(`^[0-9][0-9 .,:/TZ+-]{0,39}$`)

// maxProtoLines bounds a nested message's lines (the top level is whole).
const maxProtoLines = 150

// printProto prints a message's fields. A list (the same field repeated as
// messages, one after the other) shows its first item and how many follow.
func printProto(fields []protoField, indent string, limit int) []string {
	var out []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if limit > 0 && len(out) >= limit {
			return append(out, fmt.Sprintf("%s… %d more fields", indent, len(fields)-i))
		}
		switch {
		case f.wire == 0:
			out = append(out, fmt.Sprintf("%s%d: %d (varint)%s", indent, f.num, f.v, asDate(int64(f.v))))
		case f.wire == 1:
			d := math.Float64frombits(f.v)
			out = append(out, fmt.Sprintf("%s%d: %s (double) · %d (int64)%s", indent, f.num, strconv.FormatFloat(d, 'g', 10, 64), int64(f.v), asDate(int64(f.v))))
		case f.wire == 5:
			out = append(out, fmt.Sprintf("%s%d: %s (float) · %d (int32)", indent, f.num, strconv.FormatFloat(float64(math.Float32frombits(uint32(f.v))), 'g', 7, 32), int32(f.v)))
		case f.sub != nil:
			out = append(out, fmt.Sprintf("%s%d {", indent, f.num))
			out = append(out, printProto(f.sub, indent+"  ", maxProtoLines)...)
			out = append(out, indent+"}")
			more := 0
			for i+1 < len(fields) && fields[i+1].num == f.num && fields[i+1].sub != nil {
				i++
				more++
			}
			if more > 0 {
				out = append(out, fmt.Sprintf("%s%d {…} × %d more", indent, f.num, more))
			}
		case len(f.data) == 0:
			out = append(out, fmt.Sprintf("%s%d: (empty)", indent, f.num))
		case isText(f.data) && plainNumber.Match(f.data):
			out = append(out, fmt.Sprintf("%s%d: %q (text)", indent, f.num, f.data))
		case isText(f.data):
			out = append(out, fmt.Sprintf("%s%d: (text, %d chars)", indent, f.num, utf8.RuneCount(f.data)))
		default:
			out = append(out, fmt.Sprintf("%s%d: (bytes, %d)", indent, f.num, len(f.data)))
		}
	}
	return out
}

// asDate names a Unix time in seconds or milliseconds from 2020 to 2040.
func asDate(n int64) string {
	const from, to = 1577836800, 2208988800
	switch {
	case n >= from && n < to:
		return " = " + time.Unix(n, 0).UTC().Format("2006-01-02 15:04 UTC")
	case n >= from*1000 && n < to*1000:
		return " = " + time.UnixMilli(n).UTC().Format("2006-01-02 15:04 UTC") + " (ms)"
	}
	return ""
}

// scanNumbers finds, at each byte, a little-endian double that looks like a
// cost (0.0001 to 10 million, at most four decimals) or a 64-bit Unix time.
func scanNumbers(b []byte, limit int) []string {
	var out []string
	for i := 0; i+8 <= len(b); i++ {
		if limit > 0 && len(out) >= limit {
			return append(out, "…")
		}
		u := binary.LittleEndian.Uint64(b[i:])
		if d := asDate(int64(u)); d != "" {
			out = append(out, fmt.Sprintf("%d: %d (int64)%s", i, int64(u), d))
			continue
		}
		f := math.Float64frombits(u)
		if a := math.Abs(f); a >= 1e-4 && a <= 1e7 && math.Abs(f*1e4-math.Round(f*1e4)) < 1e-6 {
			out = append(out, fmt.Sprintf("%d: %s (double)", i, strconv.FormatFloat(f, 'g', 10, 64)))
		}
	}
	return out
}

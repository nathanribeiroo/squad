package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// StatusLine is what Claude Code passes on stdin to a status line command
// (code.claude.com/docs/en/statusline): the session's estimated cost, its
// context window and, for claude.ai Pro and Max, the plan's rate limits.
type StatusLine struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	Workspace      struct {
		ProjectDir string `json:"project_dir"`
	} `json:"workspace"`
	Model struct {
		ID string `json:"id"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
	ContextWindow struct {
		Size         int64    `json:"context_window_size"`
		UsedPercent  *float64 `json:"used_percentage"`
		CurrentUsage *struct {
			Input      int64 `json:"input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	RateLimits *struct {
		FiveHour *claudeWindow `json:"five_hour"`
		SevenDay *claudeWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type claudeWindow struct {
	UsedPercent float64 `json:"used_percentage"`
	ResetsAt    int64   `json:"resets_at"` // epoch seconds
}

// ParseStatusLine reads a status line payload.
func ParseStatusLine(b []byte) (StatusLine, error) {
	var s StatusLine
	err := json.Unmarshal(b, &s)
	return s, err
}

// Context is how full the context window is (nil before the first reply).
func (s StatusLine) Context() *Context {
	cw := s.ContextWindow
	if cw.CurrentUsage == nil || cw.Size <= 0 {
		return nil
	}
	used := cw.CurrentUsage.Input + cw.CurrentUsage.CacheWrite + cw.CurrentUsage.CacheRead
	pct := float64(used) * 100 / float64(cw.Size)
	if cw.UsedPercent != nil {
		pct = *cw.UsedPercent
	}
	return &Context{Used: used, Size: cw.Size, Percent: pct}
}

// Account is the plan's rate limits, when Claude Code reports them.
func (s StatusLine) Account(now time.Time) (Account, bool) {
	if s.RateLimits == nil || (s.RateLimits.FiveHour == nil && s.RateLimits.SevenDay == nil) {
		return Account{}, false
	}
	a := Account{Agent: "claude", Seen: now}
	for kind, w := range map[string]*claudeWindow{FiveHour: s.RateLimits.FiveHour, SevenDay: s.RateLimits.SevenDay} {
		if w == nil {
			continue
		}
		x := Window{Kind: kind, Used: w.UsedPercent}
		if w.ResetsAt > 0 {
			x.ResetsAt = time.Unix(w.ResetsAt, 0).UTC()
		}
		a.Windows = append(a.Windows, x)
	}
	slices.SortFunc(a.Windows, func(x, y Window) int { return windowOrder(x.Kind) - windowOrder(y.Kind) })
	return a, true
}

func windowOrder(k string) int {
	return slices.Index([]string{FiveHour, Daily, SevenDay, Weekly}, k)
}

// claudeTranscript reads a Claude Code transcript as it grows: each read
// continues where the last one stopped.
type claudeTranscript struct {
	mu     sync.Mutex
	path   string
	offset int64
	seen   map[string]bool // message ids already counted (a reply is written once per content block)
	sum    Tokens
	models []string
	// cost is the latest "cost-state" line, Claude Code's own running total
	// (it also counts side requests and subagents; preferred when present).
	cost *costState
}

type costState struct {
	TotalCostUSD float64 `json:"totalCostUSD"`
	ModelUsage   map[string]struct {
		Input      int64 `json:"inputTokens"`
		Output     int64 `json:"outputTokens"`
		CacheRead  int64 `json:"cacheReadInputTokens"`
		CacheWrite int64 `json:"cacheCreationInputTokens"`
	} `json:"modelUsage"`
}

// maxLine is the longest transcript line read (longer ones are skipped).
const maxLine = 16 << 20

// read takes in the lines written since the last read; a file that shrank
// (rewritten) is read again from the start.
func (t *claudeTranscript) read() error {
	f, err := os.Open(t.path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() < t.offset {
		t.offset, t.seen, t.sum, t.models, t.cost = 0, nil, Tokens{}, nil, nil
	}
	if t.seen == nil {
		t.seen = map[string]bool{}
	}
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			// A line still being written is read next time.
			return nil
		}
		t.offset += int64(len(line))
		if len(line) > maxLine {
			continue
		}
		t.line(line)
	}
}

func (t *claudeTranscript) line(line []byte) {
	switch {
	case bytes.Contains(line, []byte(`"cost-state"`)):
		var c struct {
			Type string `json:"type"`
			costState
		}
		if json.Unmarshal(line, &c) == nil && c.Type == "cost-state" {
			cs := c.costState
			t.cost = &cs
		}
	case bytes.Contains(line, []byte(`"usage"`)) && bytes.Contains(line, []byte(`"assistant"`)):
		var m struct {
			Type      string `json:"type"`
			RequestID string `json:"requestId"`
			Message   struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage *struct {
					Input      int64 `json:"input_tokens"`
					Output     int64 `json:"output_tokens"`
					CacheRead  int64 `json:"cache_read_input_tokens"`
					CacheWrite int64 `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &m) != nil || m.Type != "assistant" || m.Message.Usage == nil {
			return
		}
		key := m.Message.ID
		if key == "" {
			key = m.RequestID
		}
		if key != "" {
			if t.seen[key] {
				return
			}
			t.seen[key] = true
		}
		u := m.Message.Usage
		t.sum = t.sum.Add(Tokens{u.Input, u.Output, u.CacheRead, u.CacheWrite})
		if m.Message.Model != "" && !slices.Contains(t.models, m.Message.Model) && m.Message.Model != "<synthetic>" {
			t.models = append(t.models, m.Message.Model)
		}
	}
}

// totals is the conversation's tokens, models and estimated cost so far.
func (t *claudeTranscript) totals() (Tokens, []string, float64) {
	if t.cost == nil {
		return t.sum, slices.Clone(t.models), 0
	}
	var sum Tokens
	var models []string
	for m, u := range t.cost.ModelUsage {
		sum = sum.Add(Tokens{u.Input, u.Output, u.CacheRead, u.CacheWrite})
		models = append(models, m)
	}
	slices.Sort(models)
	return sum, models, t.cost.TotalCostUSD
}

// ClaudeHome is Claude Code's folder: $CLAUDE_CONFIG_DIR, else ~/.claude.
func ClaudeHome() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".claude")
}

// findClaudeTranscript finds a conversation's transcript under
// <ClaudeHome>/projects/<folder>/<id>.jsonl (the folder is the working
// directory, encoded).
func findClaudeTranscript(home, id string) string {
	if home == "" || id == "" || !safeID(id) {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(home, "projects", "*", id+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	newest, at := "", time.Time{}
	for _, p := range m {
		if fi, err := os.Stat(p); err == nil && fi.ModTime().After(at) {
			newest, at = p, fi.ModTime()
		}
	}
	return newest
}

// safeID: an id is safe in a file name pattern (letters, digits, - and _).
func safeID(id string) bool {
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return id != ""
}

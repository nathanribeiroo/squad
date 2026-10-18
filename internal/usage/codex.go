package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Codex CLI writes each conversation to
// $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<time>-<id>.jsonl (CODEX_HOME is
// ~/.codex by default). Its "token_count" events carry the conversation's
// running totals and, signed in with ChatGPT, the plan's rate limits
// (primary: about 5 hours; secondary: about a week; window_minutes says).

// CodexHome is $CODEX_HOME, else ~/.codex.
func CodexHome() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".codex")
}

func findCodexRollout(home, id string) string {
	if home == "" || !safeID(id) {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*"+id+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1]
}

type codexLimit struct {
	UsedPercent     float64 `json:"used_percent"`
	WindowMinutes   int64   `json:"window_minutes"`
	ResetsAt        int64   `json:"resets_at"`         // epoch seconds (newer versions)
	ResetsInSeconds int64   `json:"resets_in_seconds"` // older versions
}

// readCodex returns a rollout's latest totals and rate limits.
func readCodex(path string, now time.Time) (Flight, *Account, error) {
	f, err := os.Open(path)
	if err != nil {
		return Flight{}, nil, err
	}
	defer f.Close()
	var fl Flight
	var acc *Account
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	var stamp time.Time
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, []byte(`"token_count"`)) && !bytes.Contains(line, []byte(`"turn_context"`)) {
			continue
		}
		var ev struct {
			Timestamp time.Time `json:"timestamp"`
			Type      string    `json:"type"`
			Payload   struct {
				Type  string `json:"type"`
				Model string `json:"model"`
				Info  *struct {
					Total struct {
						Input     int64 `json:"input_tokens"`
						Cached    int64 `json:"cached_input_tokens"`
						CacheWrit int64 `json:"cache_write_input_tokens"`
						Output    int64 `json:"output_tokens"`
					} `json:"total_token_usage"`
					Window int64 `json:"model_context_window"`
					Last   struct {
						Input int64 `json:"input_tokens"`
					} `json:"last_token_usage"`
				} `json:"info"`
				RateLimits *struct {
					Primary   *codexLimit `json:"primary"`
					Secondary *codexLimit `json:"secondary"`
				} `json:"rate_limits"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Type == "turn_context" && ev.Payload.Model != "" {
			fl.Models = []string{ev.Payload.Model}
			continue
		}
		if ev.Payload.Type != "token_count" {
			continue
		}
		if !ev.Timestamp.IsZero() {
			stamp = ev.Timestamp
		}
		if in := ev.Payload.Info; in != nil {
			found = true
			t := in.Total
			cached := min(t.Cached, t.Input) // the input counts the cached part
			fl.Tokens = Tokens{Input: t.Input - cached, Output: t.Output, CacheRead: cached, CacheWrite: t.CacheWrit}
			if in.Window > 0 && in.Last.Input > 0 {
				fl.Context = &Context{Used: in.Last.Input, Size: in.Window, Percent: float64(in.Last.Input) * 100 / float64(in.Window)}
			}
		}
		if rl := ev.Payload.RateLimits; rl != nil && (rl.Primary != nil || rl.Secondary != nil) {
			seen := stamp
			if seen.IsZero() {
				seen = now
			}
			a := Account{Agent: "codex", Seen: seen}
			for _, l := range []*codexLimit{rl.Primary, rl.Secondary} {
				if l == nil {
					continue
				}
				kind := FiveHour
				if l.WindowMinutes > 24*60 {
					kind = SevenDay
				}
				w := Window{Kind: kind, Used: l.UsedPercent}
				switch {
				case l.ResetsAt > 0:
					w.ResetsAt = time.Unix(l.ResetsAt, 0).UTC()
				case l.ResetsInSeconds > 0:
					w.ResetsAt = seen.Add(time.Duration(l.ResetsInSeconds) * time.Second).UTC()
				}
				a.Windows = append(a.Windows, w)
			}
			acc = &a
		}
	}
	if !found {
		return Flight{}, acc, errNoData
	}
	fl.Agent, fl.At = "codex", stamp
	return fl, acc, nil
}

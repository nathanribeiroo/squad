package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Gemini CLI records each conversation under
// ~/.gemini/tmp/<project>/chats/session-<time>-<id>.json (.jsonl in newer
// versions, one record per line), with each reply's tokens: input
// (counting the cached part), output, cached, thoughts. It reports no plan
// limits there.

// GeminiHome is ~/.gemini.
func GeminiHome() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".gemini")
}

type geminiTokens struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Cached   int64 `json:"cached"`
	Thoughts int64 `json:"thoughts"`
}

type geminiMessage struct {
	ID     string        `json:"id"`
	Type   string        `json:"type"`
	Model  string        `json:"model"`
	Tokens *geminiTokens `json:"tokens"`
}

// findGeminiSession finds the session file whose sessionId is id (its name
// carries the id's first characters).
func findGeminiSession(home, id string) string {
	if home == "" || !safeID(id) {
		return ""
	}
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	var cands []string
	for _, ext := range []string{".json", ".jsonl"} {
		m, _ := filepath.Glob(filepath.Join(home, "tmp", "*", "chats", "session-*"+short+"*"+ext))
		cands = append(cands, m...)
	}
	for _, p := range cands {
		if geminiSessionID(p) == id {
			return p
		}
	}
	return ""
}

// geminiSessionID reads the sessionId at the start of a session file.
func geminiSessionID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := f.Read(head)
	head = head[:n]
	i := bytes.Index(head, []byte(`"sessionId"`))
	if i < 0 {
		return ""
	}
	rest := head[i+len(`"sessionId"`):]
	rest = bytes.TrimLeft(rest, " \t\r\n:")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	end := bytes.IndexByte(rest[1:], '"')
	if end < 0 {
		return ""
	}
	return string(rest[1 : 1+end])
}

// readGemini sums a session's replies (each message once: a .jsonl may
// write a message again when it changes).
func readGemini(path string) (Flight, error) {
	var msgs []geminiMessage
	if strings.HasSuffix(path, ".jsonl") {
		f, err := os.Open(path)
		if err != nil {
			return Flight{}, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), maxLine)
		for sc.Scan() {
			if !bytes.Contains(sc.Bytes(), []byte(`"tokens"`)) {
				continue
			}
			var m geminiMessage
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				msgs = append(msgs, m)
			}
		}
	} else {
		b, err := os.ReadFile(path)
		if err != nil {
			return Flight{}, err
		}
		var doc struct {
			Messages []geminiMessage `json:"messages"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return Flight{}, err
		}
		msgs = doc.Messages
	}
	byID := map[string]geminiTokens{}
	var order []string
	fl := Flight{Agent: "gemini"}
	for i, m := range msgs {
		if m.Tokens == nil || m.Type != "" && m.Type != "gemini" {
			continue
		}
		key := m.ID
		if key == "" {
			key = "#" + strconv.Itoa(i)
		}
		if _, ok := byID[key]; !ok {
			order = append(order, key)
		}
		byID[key] = *m.Tokens // the last record of a message wins
		if m.Model != "" && !slices.Contains(fl.Models, m.Model) {
			fl.Models = append(fl.Models, m.Model)
		}
	}
	if len(order) == 0 {
		return Flight{}, errNoData
	}
	for _, k := range order {
		t := byID[k]
		cached := min(t.Cached, t.Input)
		fl.Tokens = fl.Tokens.Add(Tokens{Input: t.Input - cached, Output: t.Output + t.Thoughts, CacheRead: cached})
	}
	return fl, nil
}

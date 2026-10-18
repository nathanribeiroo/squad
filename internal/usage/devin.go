package usage

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite" // Devin's session database (pure Go: no C compiler on Windows)
)

// Devin CLI keeps its sessions in a SQLite database, sessions.db, in its
// data folder: ~/.local/share/devin/cli on macOS and Linux
// ($XDG_DATA_HOME/devin/cli), %APPDATA%\devin\cli on Windows;
// DEVIN_SESSIONS_DIR moves it. Each message (message_nodes.chat_message)
// carries the model request's tokens and its cost in ACUs. Torre opens it
// read-only.

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

// readDevin sums a session's messages.
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
	rows, err := db.Query(`SELECT chat_message FROM message_nodes WHERE session_id = ?1 ORDER BY node_id`, session)
	if err != nil {
		return Flight{}, err
	}
	defer rows.Close()
	fl := Flight{Agent: "devin"}
	found := false
	for rows.Next() {
		var msg string
		if rows.Scan(&msg) != nil {
			continue
		}
		var m devinMessage
		if json.Unmarshal([]byte(msg), &m) != nil {
			continue
		}
		md := m.Metadata
		if mt := md.Metrics; mt != nil {
			found = true
			fl.Tokens = fl.Tokens.Add(Tokens{Input: mt.Input, Output: mt.Output, CacheRead: mt.CacheRead, CacheWrite: mt.CacheWrite})
		}
		if md.ACUCost != nil {
			found = true
			fl.ACUs += *md.ACUCost
		}
	}
	if err := rows.Err(); err != nil {
		return Flight{}, err
	}
	if !found {
		return Flight{}, errNoData
	}
	return fl, nil
}

type devinMessage struct {
	Metadata struct {
		Metrics *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_tokens"`
			CacheWrite int64 `json:"cache_creation_tokens"`
		} `json:"metrics"`
		ACUCost *float64 `json:"committed_acu_cost"`
	} `json:"metadata"`
}

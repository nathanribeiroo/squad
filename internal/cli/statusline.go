package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/terminal"
	agentusage "github.com/nathanribeiroo/squad/internal/usage"
)

// statusLineHook is the hook name of Claude Code's status line (Torre sets
// it in the settings it passes to Claude Code).
const statusLineHook = "claude-statusline"

// runStatusLine is `torre hook claude-statusline`: Claude Code runs it on
// each update with the session's state on stdin. Torre gets the payload
// (tokens, context, plan limits), then the user's own status line runs as
// it would without Torre, and its output is the line shown.
func runStatusLine(stdout, stderr io.Writer) int {
	payload, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	done := make(chan struct{})
	go func() { // to Torre, while the user's status line runs
		defer close(done)
		postStatusLine(payload, stderr)
	}()
	if cmd := userStatusLine(payload); cmd != "" {
		runUserStatusLine(cmd, payload, stdout)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	return 0
}

func postStatusLine(payload []byte, stderr io.Writer) {
	env, err := readClientEnv()
	if err != nil {
		debugf(stderr, "%v", err)
		return
	}
	req, err := http.NewRequest("POST", strings.TrimRight(env.url, "/")+"/hooks/"+statusLineHook, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Terminal-ID", env.id)
	req.Header.Set("X-Torre-Token", env.token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		debugf(stderr, "%v", err)
		return
	}
	resp.Body.Close()
}

// userStatusLine is the status line command the user configured for
// Claude Code, as Claude Code would pick it without Torre's settings: the
// project's settings.local.json, then its settings.json, then the user's.
func userStatusLine(payload []byte) string {
	s, _ := agentusage.ParseStatusLine(payload)
	var files []string
	for _, dir := range []string{s.Workspace.ProjectDir, s.Cwd} {
		if dir != "" {
			files = append(files, filepath.Join(dir, ".claude", "settings.local.json"), filepath.Join(dir, ".claude", "settings.json"))
		}
	}
	if h := agentusage.ClaudeHome(); h != "" {
		files = append(files, filepath.Join(h, "settings.json"))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc struct {
			StatusLine *struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"statusLine"`
		}
		if json.Unmarshal(b, &doc) != nil || doc.StatusLine == nil {
			continue
		}
		c := strings.TrimSpace(doc.StatusLine.Command)
		if c == "" || strings.Contains(c, "hook "+statusLineHook) { // never Torre's own
			continue
		}
		return c
	}
	return ""
}

// runUserStatusLine runs the user's status line command through a shell, as
// Claude Code does (Git Bash on Windows, cmd without it), with the payload
// on stdin; its output is printed.
func runUserStatusLine(command string, payload []byte, stdout io.Writer) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS != "windows":
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	case gitBash() != "":
		cmd = exec.CommandContext(ctx, gitBash(), "-c", command)
	default:
		cmd = exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	}
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = io.Discard
	_ = cmd.Run()
}

// gitBash is the bash Claude Code uses on Windows: CLAUDE_CODE_GIT_BASH_PATH,
// else Git for Windows'.
func gitBash() string {
	if p := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); p != "" {
		return p
	}
	return terminal.GitBash()
}

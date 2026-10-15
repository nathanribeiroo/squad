package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// clientEnv is the identity a Squad terminal gives its processes.
type clientEnv struct {
	url, id, token string
}

func readClientEnv() (clientEnv, error) {
	e := clientEnv{os.Getenv("SQUAD_URL"), os.Getenv("SQUAD_TERMINAL_ID"), os.Getenv("SQUAD_TOKEN")}
	if e.url == "" || e.id == "" || e.token == "" {
		return e, fmt.Errorf("squad: this command only works inside a Squad terminal (SQUAD_TERMINAL_ID is not set)")
	}
	return e, nil
}

// stdinMessage returns piped stdin (for heredocs) when stdin isn't a TTY.
func stdinMessage() (string, bool) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return "", false
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return "", false
	}
	return strings.TrimRight(string(b), "\n"), true
}

// withStdin fills in a message from stdin for `ask "Name"`, `reply <id>` and
// `note write "Name"`, or when the message argument is "-".
func withStdin(args []string) []string {
	needs := func() bool {
		switch {
		case len(args) == 2 && args[0] == "ask" && !strings.HasPrefix(args[1], "--"):
			return true
		case len(args) == 2 && args[0] == "reply":
			return true
		case len(args) == 4 && args[0] == "note" && args[1] == "write" && args[3] == "-":
			return true
		case len(args) > 0 && args[len(args)-1] == "-":
			return true
		}
		return false
	}
	if !needs() {
		return args
	}
	msg, ok := stdinMessage()
	if !ok {
		return args
	}
	if args[len(args)-1] == "-" {
		return append(append([]string{}, args[:len(args)-1]...), msg)
	}
	return append(append([]string{}, args...), msg)
}

// blocking reports whether a command may wait for another agent.
func blocking(args []string) bool {
	return len(args) > 1 && args[0] == "ask" && !(len(args) == 4 && args[2] == "--raw")
}

func runClient(args []string, stdout, stderr io.Writer) int {
	env, err := readClientEnv()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	args = withStdin(args)
	cwd, _ := os.Getwd()
	body, _ := json.Marshal(map[string]any{"args": args, "cwd": cwd})
	req, err := http.NewRequest("POST", strings.TrimRight(env.url, "/")+"/cli", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(stderr, "squad:", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Terminal-ID", env.id)
	req.Header.Set("X-Squad-Token", env.token)
	client := &http.Client{Timeout: 60 * time.Second}
	if blocking(args) {
		client.Timeout = 0 // the agent's own tool timeout bounds the wait
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "squad: can't reach the Squad server at %s: %v\nIf the request was sent, it may still be processed; check before repeating it.\n", env.url, err)
		return 1
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		_, _ = stderr.Write(out)
		return 1
	}
	_, _ = stdout.Write(out)
	return 0
}

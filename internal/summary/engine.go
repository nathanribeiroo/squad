package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Engines are the agent CLIs that can write a summary, in the order "auto"
// falls back on.
var Engines = []string{"claude", "devin", "codex", "gemini"}

// EngineInfo is an engine as the settings show it.
type EngineInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"` // where it was found ("" if not installed)
	Available bool   `json:"available"`
}

// LookPath finds a program (exec.LookPath; tests replace it).
type LookPath func(name string) (string, error)

// detect returns the installed engines.
func detect(look LookPath) []EngineInfo {
	out := make([]EngineInfo, 0, len(Engines))
	for _, name := range Engines {
		e := EngineInfo{Name: name}
		if p, err := look(name); err == nil {
			e.Path, e.Available = p, true
		}
		out = append(out, e)
	}
	return out
}

// job is one request to an engine: instructions and the material to read.
type job struct {
	system string // what to do and the JSON to answer with
	input  string // the turn or the workspace
}

// command builds how an engine runs a job in dir, where the job's files are.
// No free text goes on the command line: the instructions and the material
// travel in files or on stdin, so nothing needs quoting (cmd shims on
// Windows), and no engine gets tools that touch the project.
func command(engine, path, dir string, j job) (args []string, stdin string, outFile string, err error) {
	sysFile := filepath.Join(dir, "instructions.txt")
	promptFile := filepath.Join(dir, "prompt.txt")
	whole := j.system + "\n\n---\n\n" + j.input
	switch engine {
	case "claude":
		// A small, fast model; no tools, MCP servers, settings (so none of
		// the user's hooks) or saved session. Not --bare: it ignores the
		// user's login.
		if err := os.WriteFile(sysFile, []byte(j.system), 0o600); err != nil {
			return nil, "", "", err
		}
		return []string{path, "-p", "--model", "haiku", "--tools", "", "--no-session-persistence",
			"--setting-sources", "", "--strict-mcp-config", "--output-format", "json",
			"--system-prompt-file", sysFile}, j.input, "", nil
	case "devin":
		// Single-turn mode; the empty folder isn't trusted, and print mode
		// can't ask.
		if err := os.WriteFile(promptFile, []byte(whole), 0o600); err != nil {
			return nil, "", "", err
		}
		return []string{path, "-p", "--prompt-file", promptFile, "--respect-workspace-trust", "false"}, "", "", nil
	case "codex":
		out := filepath.Join(dir, "answer.txt")
		return []string{path, "exec", "--skip-git-repo-check", "--sandbox", "read-only",
			"--output-last-message", out, "-"}, whole, out, nil
	case "gemini":
		// -p is appended to what comes on stdin.
		return []string{path, "-p", "Siga as instruções acima e responda só com o JSON pedido."}, whole, "", nil
	}
	return nil, "", "", fmt.Errorf("unknown engine %q", engine)
}

// run executes a job with an engine in a fresh empty folder and returns its
// answer text.
func run(ctx context.Context, engine, path string, j job, timeout time.Duration) (string, error) {
	dir, err := os.MkdirTemp("", "squad-summary-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	args, stdin, outFile, err := command(engine, path, dir, j)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = terminal.BuildEnv(os.Environ(), nil, "") // without a parent agent session's markers
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 5 * time.Second
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s took more than %s", engine, timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return "", fmt.Errorf("%s failed (%v): %s", engine, err, msg)
	}
	out := stdout.String()
	if outFile != "" {
		if b, err := os.ReadFile(outFile); err == nil {
			out = string(b)
		}
	}
	if engine == "claude" {
		// --output-format json wraps the answer: {"result": "...", "is_error": …}.
		var wrap struct {
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &wrap) == nil {
			if wrap.IsError {
				return "", fmt.Errorf("claude: %s", wrap.Result)
			}
			out = wrap.Result
		}
	}
	return out, nil
}

// decode reads the JSON object in an answer, tolerating text or a code
// fence around it.
func decode(answer string, v any) error {
	i, j := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if i < 0 || j < i {
		return errors.New("the answer has no JSON object")
	}
	if err := json.Unmarshal([]byte(answer[i:j+1]), v); err != nil {
		return fmt.Errorf("the answer's JSON is invalid: %w", err)
	}
	return nil
}

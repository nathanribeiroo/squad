package agents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

func TestParseHook(t *testing.T) {
	ev, ok := ParseHook("claude-stop", []byte(`{"hook_event_name":"Stop","last_assistant_message":"pong","stop_hook_active":false}`))
	if !ok || !ev.TurnEnd || ev.Message != "pong" {
		t.Fatalf("claude-stop: %+v %v", ev, ok)
	}
	ev, ok = ParseHook("claude-stop-failure", []byte(`{"error":"rate_limit","error_details":"429"}`))
	if !ok || !ev.Failure || !strings.Contains(ev.Message, "rate_limit") {
		t.Fatalf("failure: %+v", ev)
	}
	if ev, ok := ParseHook("claude-prompt", []byte(`{}`)); !ok || !ev.TurnStart {
		t.Fatalf("prompt: %+v", ev)
	}
	ev, ok = ParseHook("codex-notify", []byte(`{"type":"agent-turn-complete","thread-id":"t","last-assistant-message":"done!"}`))
	if !ok || ev.Message != "done!" {
		t.Fatalf("codex: %+v %v", ev, ok)
	}
	if _, ok := ParseHook("codex-notify", []byte(`{"type":"other"}`)); ok {
		t.Fatal("codex non-turn event accepted")
	}
	ev, ok = ParseHook("devin-stop", []byte(`{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"feito","session_id":"s","prompt_id":"p"}`))
	if !ok || !ev.TurnEnd || ev.Failure || ev.Message != "feito" {
		t.Fatalf("devin-stop: %+v %v", ev, ok)
	}
	if ev, ok := ParseHook("devin-prompt", []byte(`{"hook_event_name":"UserPromptSubmit","prompt":"oi"}`)); !ok || !ev.TurnStart {
		t.Fatalf("devin-prompt: %+v", ev)
	}
	if _, ok := ParseHook("unknown", nil); ok {
		t.Fatal("unknown hook accepted")
	}
}

func TestTranscriptFallback(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"final answer"}]}}`,
		`{"type":"system","subtype":"x"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev, ok := ParseHook("claude-stop", []byte(`{"transcript_path":"`+p+`"}`))
	if !ok || ev.Message != "final answer" {
		t.Fatalf("transcript fallback: %+v", ev)
	}
}

func TestPrepareLaunchLines(t *testing.T) {
	t.Setenv("SQUAD_HOME", t.TempDir())
	l, err := NewLauncher("/opt/squad/bin/squad")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(l.ClaudeSettings)
	if err != nil || !strings.Contains(string(settings), `\"/opt/squad/bin/squad\" hook claude-stop`) {
		t.Fatalf("settings file: %v\n%s", err, settings)
	}
	spec := terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	if err := l.Prepare(store.Node{}, &spec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(spec.Command, "claude --settings ") || !strings.Contains(spec.Command, "claude-hooks.json") {
		t.Fatalf("claude command: %s", spec.Command)
	}
	spec = terminal.Spec{Agent: "codex", Command: "codex --model x", Shell: "/bin/sh"}
	_ = l.Prepare(store.Node{}, &spec)
	want := `codex --model x -c 'notify=["/opt/squad/bin/squad","hook","codex-notify"]'`
	if spec.Command != want {
		t.Fatalf("codex command:\n got %s\nwant %s", spec.Command, want)
	}
	devinCfg, err := os.ReadFile(l.DevinConfig)
	if err != nil || !strings.Contains(string(devinCfg), `\"/opt/squad/bin/squad\" hook devin-stop`) ||
		!strings.Contains(string(devinCfg), `hook devin-prompt`) || strings.Contains(string(devinCfg), "StopFailure") {
		t.Fatalf("devin config: %v\n%s", err, devinCfg)
	}
	spec = terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
	_ = l.Prepare(store.Node{}, &spec)
	if want := "devin --config " + terminal.Quote(terminal.ShellPOSIX, l.DevinConfig); spec.Command != want {
		t.Fatalf("devin command:\n got %s\nwant %s", spec.Command, want)
	}
	spec = terminal.Spec{Agent: "shell"}
	_ = l.Prepare(store.Node{}, &spec)
	if spec.Command != "" {
		t.Fatalf("shell command: %q", spec.Command)
	}
	if !HasHooks("claude") || !HasHooks("devin") || HasHooks("gemini") || HasHooks("whatever") {
		t.Fatal("HasHooks")
	}
}

func TestRoleDelivery(t *testing.T) {
	t.Setenv("SQUAD_HOME", t.TempDir())
	l, err := NewLauncher("/opt/squad/bin/squad")
	if err != nil {
		t.Fatal(err)
	}
	const text = "# Papel no Squad: Testador\n\nUse `squad ask \"Nome\"`; 100% $HOME! 'ok' ção \U0001F600\n"
	asked := 0
	l.Role = func(n store.Node) (string, bool) {
		asked++
		return text, n.Terminal != nil && n.Terminal.Role != ""
	}
	node := func(id, role string) store.Node {
		return store.Node{ID: id, Terminal: &store.TerminalData{Name: "Ana", Role: role}}
	}

	// Claude Code: a file per terminal, appended to the system prompt.
	spec := terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	if err := l.Prepare(node("t1", "test"), &spec); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(l.RoleDir, "t1.md")
	if !strings.HasSuffix(spec.Command, " --append-system-prompt-file "+terminal.Quote(terminal.ShellPOSIX, file)) {
		t.Fatalf("claude: %s", spec.Command)
	}
	if data, _ := os.ReadFile(file); string(data) != text {
		t.Fatalf("role file: %q", data)
	}
	spec = terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	_ = l.Prepare(node("t1", ""), &spec)
	if strings.Contains(spec.Command, "append-system-prompt") {
		t.Fatalf("no role, no flag: %s", spec.Command)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("a role taken away removes the file")
	}

	// Codex: developer_instructions as an ASCII TOML string, safe in any shell.
	spec = terminal.Spec{Agent: "codex", Shell: "/bin/sh"}
	_ = l.Prepare(node("t2", "test"), &spec)
	_, arg, ok := strings.Cut(spec.Command, " -c 'developer_instructions=")
	if !ok {
		t.Fatalf("codex: %s", spec.Command)
	}
	value := strings.TrimSuffix(arg, "'")
	for _, c := range value[1 : len(value)-1] {
		if c > 127 || strings.ContainsRune("\"'$%!`\n&|<>^", c) {
			t.Fatalf("codex value has %q: %s", c, value)
		}
	}
	if got, err := strconv.Unquote(value); err != nil || got != text { // TOML escapes are Go's
		t.Fatalf("codex value decodes to %q (%v)", got, err)
	}

	// Devin: the SessionStart hook asks for the role.
	if cfg, _ := os.ReadFile(l.DevinConfig); !strings.Contains(string(cfg), `"SessionStart"`) || !strings.Contains(string(cfg), "hook devin-session-start") {
		t.Fatalf("devin config: %s", cfg)
	}

	// Gemini: the role is the first prompt, from the environment.
	spec = terminal.Spec{Agent: "gemini", Shell: "/bin/sh"}
	_ = l.Prepare(node("t3", "test"), &spec)
	if spec.Command != `gemini -i "$SQUAD_ROLE"` || spec.Env["SQUAD_ROLE"] != text {
		t.Fatalf("gemini: %s %q", spec.Command, spec.Env["SQUAD_ROLE"])
	}
	if envRef(terminal.ShellPowerShell, "X") != "$env:X" || envRef(terminal.ShellCmd, "X") != `"%X%"` {
		t.Fatal("envRef")
	}

	// Shells and custom commands never get it: typing it there could run it.
	asked = 0
	for _, agent := range []string{"shell", "custom"} {
		spec = terminal.Spec{Agent: agent, Command: "aider", Shell: "/bin/sh"}
		_ = l.Prepare(node("t4", "test"), &spec)
		if spec.Command != "aider" || len(spec.Env) != 0 {
			t.Fatalf("%s: %s %v", agent, spec.Command, spec.Env)
		}
	}
	if asked != 0 {
		t.Fatal("the role of a shell is not even looked up")
	}
}

func TestSessionStartHook(t *testing.T) {
	if ev, ok := ParseHook("devin-session-start", []byte(`{"hook_event_name":"SessionStart","source":"startup"}`)); !ok || !ev.SessionStart {
		t.Fatalf("parse: %+v %v", ev, ok)
	}
	h := HookHandler(nil, func(r *http.Request) string { return r.Header.Get("X-Terminal-ID") }, func(id string) (string, bool) {
		return "Seja o Testador.", id == "with-role"
	})
	call := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/hooks/devin-session-start", strings.NewReader(`{"source":"startup"}`))
		r.SetPathValue("agent", "devin-session-start")
		r.Header.Set("X-Terminal-ID", id)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("with-role")
	var out struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.HookSpecificOutput.HookEventName != "SessionStart" || out.HookSpecificOutput.AdditionalContext != "Seja o Testador." {
		t.Fatalf("with role: %d %s", w.Code, w.Body)
	}
	if w := call("no-role"); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("no role: %d %s", w.Code, w.Body)
	}
}

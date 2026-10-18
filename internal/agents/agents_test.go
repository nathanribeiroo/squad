package agents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/attention"
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
	t.Setenv("TORRE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no user Devin config
	t.Setenv("APPDATA", t.TempDir())
	l, err := NewLauncher("/opt/torre/bin/torre")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(l.ClaudeSettings)
	if err != nil || !strings.Contains(string(settings), `\"/opt/torre/bin/torre\" hook claude-stop`) {
		t.Fatalf("settings file: %v\n%s", err, settings)
	}
	spec := terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	if _, err := l.Prepare(&store.Node{}, &spec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(spec.Command, "claude --settings ") || !strings.Contains(spec.Command, "claude-hooks.json") ||
		!strings.Contains(spec.Command, " --mcp-config "+terminal.Quote(terminal.ShellPOSIX, l.ClaudeMCP)) {
		t.Fatalf("claude command: %s", spec.Command)
	}
	// torre mcp for Claude Code, with the safe tools pre-approved and the
	// others (keys, new_shell, portal_eval) left to the user.
	var mcpCfg struct {
		MCPServers map[string]struct {
			Command string
			Args    []string
		} `json:"mcpServers"`
	}
	if data, err := os.ReadFile(l.ClaudeMCP); err != nil || json.Unmarshal(data, &mcpCfg) != nil ||
		mcpCfg.MCPServers["torre"].Command != "/opt/torre/bin/torre" || strings.Join(mcpCfg.MCPServers["torre"].Args, " ") != "mcp" {
		t.Fatalf("claude mcp config: %v %+v", err, mcpCfg)
	}
	var cs struct {
		Permissions struct{ Allow []string }
	}
	_ = json.Unmarshal(settings, &cs)
	if !slices.Contains(cs.Permissions.Allow, "mcp__torre__ask") || !slices.Contains(cs.Permissions.Allow, "mcp__torre__portal") ||
		slices.Contains(cs.Permissions.Allow, "mcp__torre__keys") || slices.Contains(cs.Permissions.Allow, "mcp__torre__new_shell") ||
		slices.Contains(cs.Permissions.Allow, "mcp__torre__portal_eval") || slices.Contains(cs.Permissions.Allow, "mcp__torre__floor_land") ||
		!slices.Contains(cs.Permissions.Allow, "mcp__torre__floors") {
		t.Fatalf("pre-approved tools: %v", cs.Permissions.Allow)
	}
	spec = terminal.Spec{Agent: "codex", Command: "codex --model x", Shell: "/bin/sh"}
	_, _ = l.Prepare(&store.Node{}, &spec)
	want := `codex --model x -c 'notify=["/opt/torre/bin/torre","hook","codex-notify"]'` +
		` -c 'mcp_servers.torre.command="/opt/torre/bin/torre"' -c 'mcp_servers.torre.args=["mcp"]'` +
		` -c 'mcp_servers.torre.env_vars=["TORRE_URL","TORRE_TERMINAL_ID","TORRE_TOKEN"]' -c mcp_servers.torre.tool_timeout_sec=1800`
	if spec.Command != want {
		t.Fatalf("codex command:\n got %s\nwant %s", spec.Command, want)
	}
	spec = terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
	_, _ = l.Prepare(&store.Node{}, &spec)
	if want := "devin --config " + terminal.Quote(terminal.ShellPOSIX, l.DevinConfig); spec.Command != want {
		t.Fatalf("devin command:\n got %s\nwant %s", spec.Command, want)
	}
	devinCfg, err := os.ReadFile(l.DevinConfig)
	if err != nil || !strings.Contains(string(devinCfg), `\"/opt/torre/bin/torre\" hook devin-stop`) || strings.Contains(string(devinCfg), `"mcpServers"`) ||
		!strings.Contains(string(devinCfg), `hook devin-prompt`) || strings.Contains(string(devinCfg), "StopFailure") {
		t.Fatalf("devin config: %v\n%s", err, devinCfg)
	}
	// On a floor, Devin doesn't stop at its workspace trust prompt.
	spec = terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
	_, _ = l.Prepare(&store.Node{Floor: "f1"}, &spec)
	if !strings.HasSuffix(spec.Command, " --respect-workspace-trust false") {
		t.Fatalf("devin on a floor: %s", spec.Command)
	}
	// Nor in a session's copy.
	l.InCopy = func() bool { return true }
	spec = terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
	_, _ = l.Prepare(&store.Node{}, &spec)
	if !strings.HasSuffix(spec.Command, " --respect-workspace-trust false") {
		t.Fatalf("devin in a session's copy: %s", spec.Command)
	}
	l.InCopy = nil
	spec = terminal.Spec{Agent: "shell"}
	_, _ = l.Prepare(&store.Node{}, &spec)
	if spec.Command != "" {
		t.Fatalf("shell command: %q", spec.Command)
	}
	if !HasHooks("claude") || !HasHooks("devin") || HasHooks("gemini") || HasHooks("whatever") {
		t.Fatal("HasHooks")
	}
}

func TestRoleDelivery(t *testing.T) {
	t.Setenv("TORRE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	l, err := NewLauncher("/opt/torre/bin/torre")
	if err != nil {
		t.Fatal(err)
	}
	const text = "# Papel na Torre: Testador\n\nUse `torre ask \"Nome\"`; 100% $HOME! 'ok' ção \U0001F600\n"
	asked := 0
	l.Role = func(n store.Node) (string, bool) {
		asked++
		return text, n.Terminal != nil && n.Terminal.Role != ""
	}
	l.HasFlag = func(string, string) bool { return false } // a Gemini without --session-id
	node := func(id, role string) *store.Node {
		return &store.Node{ID: id, Terminal: &store.TerminalData{Name: "Ana", Role: role}}
	}

	// Claude Code: a file per terminal, appended to the system prompt.
	spec := terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	if _, err := l.Prepare(node("t1", "test"), &spec); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(l.RoleDir, "t1.md")
	if !strings.Contains(spec.Command, " --append-system-prompt-file "+terminal.Quote(terminal.ShellPOSIX, file)+" ") {
		t.Fatalf("claude: %s", spec.Command)
	}
	if data, _ := os.ReadFile(file); string(data) != text {
		t.Fatalf("role file: %q", data)
	}
	spec = terminal.Spec{Agent: "claude", Shell: "/bin/sh"}
	_, _ = l.Prepare(node("t1", ""), &spec)
	if strings.Contains(spec.Command, "append-system-prompt") {
		t.Fatalf("no role, no flag: %s", spec.Command)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("a role taken away removes the file")
	}

	// Codex: developer_instructions as an ASCII TOML string, safe in any shell.
	spec = terminal.Spec{Agent: "codex", Shell: "/bin/sh"}
	_, _ = l.Prepare(node("t2", "test"), &spec)
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
	spec = terminal.Spec{Agent: "devin", Shell: "/bin/sh"}
	_, _ = l.Prepare(node("t4", "test"), &spec)
	if cfg, _ := os.ReadFile(l.DevinConfig); !strings.Contains(string(cfg), `"SessionStart"`) || !strings.Contains(string(cfg), "hook devin-session-start") {
		t.Fatalf("devin config: %s", cfg)
	}

	// Gemini: the role is the first prompt, from the environment.
	spec = terminal.Spec{Agent: "gemini", Shell: "/bin/sh"}
	_, _ = l.Prepare(node("t3", "test"), &spec)
	if spec.Command != `gemini -i "$TORRE_ROLE"` || spec.Env["TORRE_ROLE"] != text {
		t.Fatalf("gemini: %s %q", spec.Command, spec.Env["TORRE_ROLE"])
	}
	if envRef(terminal.ShellPowerShell, "X") != "$env:X" || envRef(terminal.ShellCmd, "X") != `"%X%"` {
		t.Fatal("envRef")
	}

	// Shells and custom commands never get it: typing it there could run it.
	asked = 0
	for _, agent := range []string{"shell", "custom"} {
		spec = terminal.Spec{Agent: agent, Command: "aider", Shell: "/bin/sh"}
		_, _ = l.Prepare(node("t4", "test"), &spec)
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
	var reported []string
	h := HookHandler(HookOptions{
		CallerOf: func(r *http.Request) string { return r.Header.Get("X-Terminal-ID") },
		Role:     func(id string) (string, bool) { return "Seja o Testador.", id == "with-role" },
		Conversation: func(id, conv, source string) {
			reported = append(reported, id+" "+conv+" "+source)
		},
	})
	callHook := func(hook, id, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/hooks/"+hook, strings.NewReader(body))
		r.SetPathValue("agent", hook)
		r.Header.Set("X-Terminal-ID", id)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	call := func(id string) *httptest.ResponseRecorder {
		return callHook("devin-session-start", id, `{"source":"startup","session_id":"d-1"}`)
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
	// A continued conversation already has its role; Claude Code gets it at
	// launch. Both report their conversation.
	if w := callHook("devin-session-start", "with-role", `{"source":"resume","session_id":"d-1"}`); w.Code != http.StatusNoContent {
		t.Fatalf("devin resume: %d %s", w.Code, w.Body)
	}
	if w := callHook("claude-session-start", "with-role", `{"source":"startup","session_id":"c-1"}`); w.Code != http.StatusNoContent {
		t.Fatalf("claude session start: %d %s", w.Code, w.Body)
	}
	if got := strings.Join(reported, "|"); got != "with-role d-1 startup|no-role d-1 startup|with-role d-1 resume|with-role c-1 startup" {
		t.Fatalf("conversations: %s", got)
	}
}

// Each agent reports its conversation id where its hooks carry it.
func TestHookConversation(t *testing.T) {
	for hook, body := range map[string]string{
		"claude-prompt":        `{"prompt":"oi","session_id":"s1"}`,
		"claude-stop":          `{"last_assistant_message":"ok","session_id":"s1"}`,
		"devin-prompt":         `{"prompt":"oi","session_id":"s1"}`,
		"devin-stop":           `{"last_assistant_message":"ok","session_id":"s1"}`,
		"codex-notify":         `{"type":"agent-turn-complete","thread-id":"s1","last-assistant-message":"ok"}`,
		"claude-session-start": `{"source":"clear","session_id":"s1"}`,
	} {
		if ev, ok := ParseHook(hook, []byte(body)); !ok || ev.Conversation != "s1" {
			t.Errorf("%s: %+v", hook, ev)
		}
	}
	for id, ok := range map[string]bool{
		"0b9c7e1a-77f4-4e0b-9a3e-2b1f0c9d8e7f": true, "ses_01HXYZ": true, "a.b_c-1": true,
		"": false, "-r": false, "a b": false, "a;rm": false, "$(x)": false, strings.Repeat("a", 129): false,
	} {
		if ValidConversation(id) != ok {
			t.Errorf("ValidConversation(%q) != %v", id, ok)
		}
	}
}

// A terminal starts a new conversation, or continues the one it remembers;
// when the agent can't find it, the launch line itself starts a new one.
func TestConversationLines(t *testing.T) {
	t.Setenv("TORRE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	l, err := NewLauncher("/opt/torre/bin/torre")
	if err != nil {
		t.Fatal(err)
	}
	gemini := true
	l.HasFlag = func(prog, flag string) bool { return gemini && prog == "gemini" && flag == "--session-id" }
	l.Role = func(store.Node) (string, bool) { return "papel", true }
	prepare := func(agent, cmd, conv string) (string, string, bool) {
		t.Helper()
		n := &store.Node{ID: "t1", Terminal: &store.TerminalData{Name: "Ana", Agent: agent, Conversation: conv}}
		spec := terminal.Spec{ID: "t1", Agent: agent, Command: cmd, Shell: "/bin/sh"}
		resumed, err := l.Prepare(n, &spec)
		if err != nil {
			t.Fatal(err)
		}
		return spec.Command, n.Terminal.Conversation, resumed
	}

	// Claude: Torre picks the id, so it's known before the first prompt.
	line, conv, resumed := prepare("claude", "", "")
	if !ValidConversation(conv) || len(conv) != 36 || conv != strings.ToLower(conv) || resumed || !strings.HasSuffix(line, " --session-id "+conv) {
		t.Fatalf("claude, new: %s (%s, %v)", line, conv, resumed)
	}
	base := strings.TrimSuffix(line, " --session-id "+conv)
	line, conv2, resumed := prepare("claude", "", conv)
	if want := base + " --resume " + conv + " || " + base + " --session-id " + conv; line != want || conv2 != conv || !resumed {
		t.Fatalf("claude, continued:\n got %s\nwant %s", line, want)
	}
	// An id that isn't one (it came from a hook) is never put on the line.
	if line, conv, _ := prepare("claude", "", "-r; rm -rf ~"); strings.Contains(line, "rm") || !ValidConversation(conv) {
		t.Fatalf("claude, bad id: %s %s", line, conv)
	}
	// The user's own line picks the conversation: Torre leaves it alone.
	if line, conv, resumed := prepare("claude", "claude --continue", "c-1"); strings.Contains(line, "--session-id") || strings.Contains(line, "--resume") || conv != "c-1" || resumed {
		t.Fatalf("claude --continue: %s %s", line, conv)
	}

	// Devin picks its own id (the hooks report it).
	line, conv, resumed = prepare("devin", "", "")
	devin := "devin --config " + terminal.Quote(terminal.ShellPOSIX, l.DevinConfig)
	if line != devin || conv != "" || resumed {
		t.Fatalf("devin, new: %s %q", line, conv)
	}
	if line, conv, resumed := prepare("devin", "", "d-1"); line != devin+" --resume d-1 || "+devin || conv != "d-1" || !resumed {
		t.Fatalf("devin, continued: %s", line)
	}

	// Codex: the resume subcommand goes right after the program.
	line, _, _ = prepare("codex", "codex --model x", "")
	if line, conv, resumed := prepare("codex", "codex --model x", "x-1"); !strings.HasPrefix(line, "codex resume x-1 --model x -c ") ||
		!strings.HasSuffix(line, " || "+strings.TrimSpace(strings.SplitN(line, " || ", 2)[1])) || !strings.Contains(line, " || codex --model x -c ") || conv != "x-1" || !resumed {
		t.Fatalf("codex, continued: %s", line)
	}
	if line, conv, resumed := prepare("codex", "npx codex", "x-1"); strings.Contains(line, "resume") || conv != "x-1" || resumed {
		t.Fatalf("codex through npx: %s %s", line, conv)
	}

	// Gemini: like Claude when it has --session-id; the role is the first
	// prompt of a new conversation only.
	line, conv, _ = prepare("gemini", "", "")
	if line != "gemini --session-id "+conv+` -i "$TORRE_ROLE"` {
		t.Fatalf("gemini, new: %s", line)
	}
	if line, _, resumed := prepare("gemini", "", "g-1"); line != `gemini --resume g-1 || gemini --session-id g-1 -i "$TORRE_ROLE"` || !resumed {
		t.Fatalf("gemini, continued: %s", line)
	}
	gemini = false // an older Gemini CLI
	if line, conv, resumed := prepare("gemini", "", "g-1"); line != `gemini -i "$TORRE_ROLE"` || conv != "" || resumed {
		t.Fatalf("older gemini: %s %q", line, conv)
	}
}

// Claude Code 2.1.302's question and plan, as its permission hook sends them:
// the question's text (the screen gives the rest), the plan whole.
func TestParseHookQuestionAndPlan(t *testing.T) {
	ev, ok := ParseHook("claude-permission", []byte(`{"hook_event_name":"PermissionRequest","tool_name":"AskUserQuestion",
		"tool_input":{"questions":[{"question":"Qual cor?","header":"Cor","options":[{"label":"Azul","description":"Blue color"}],"multiSelect":false}]}}`))
	if a := ev.Attention; !ok || a == nil || a.Kind != attention.Question || a.Text != "Qual cor?" || a.Detail != "" {
		t.Fatalf("question: %+v %v", ev.Attention, ok)
	}
	plan := "# Plan: create hello.txt\n\n## Steps\n1. Create it.\n"
	b, _ := json.Marshal(map[string]any{"tool_name": "ExitPlanMode", "tool_input": map[string]any{"plan": plan, "planFilePath": "/root/.claude/plans/x.md"}})
	ev, ok = ParseHook("claude-permission", b)
	if a := ev.Attention; !ok || a == nil || a.Kind != attention.Plan || a.PlanText != plan || a.Detail != "Plan: create hello.txt" {
		t.Fatalf("plan: %+v %v", ev.Attention, ok)
	}
	long := strings.Repeat("ç", 20<<10)
	b, _ = json.Marshal(map[string]any{"tool_name": "ExitPlanMode", "tool_input": map[string]any{"plan": long}})
	if ev, _ := ParseHook("claude-permission", b); len(ev.Attention.PlanText) > 33<<10 || !utf8.ValidString(ev.Attention.PlanText) {
		t.Fatalf("long plan: %d bytes", len(ev.Attention.PlanText))
	}
}

package agents

import (
	"os"
	"path/filepath"
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
	spec = terminal.Spec{Agent: "shell"}
	_ = l.Prepare(store.Node{}, &spec)
	if spec.Command != "" {
		t.Fatalf("shell command: %q", spec.Command)
	}
	if !HasHooks("claude") || HasHooks("gemini") || HasHooks("whatever") {
		t.Fatal("HasHooks")
	}
}

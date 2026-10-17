package attention

import (
	"reflect"
	"testing"
)

// Claude Code 2.1.302, a Bash command in the default permission mode.
const claudeBash = `
● Creating empty file made.txt
  ⎿  $ touch /tmp/ask/made.txt

────────────────────────────────────────────────────────────────────────────────
 Bash command
 Tip: auto mode handles these prompts for you — choose "switch to auto mode" below
 Create empty file made.txt
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 │ touch /tmp/ask/made.txt
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and always allow access to
      /tmp/ask from this
      project
   3. Yes, and switch to auto mode · auto mode handles these prompts for you
   4. No

 Esc to cancel · Tab to amend
`

// An edit, where option 2 switches to accepting every edit.
const claudeEdit = `
 Edit file
 web/src/App.tsx
 Do you want to make this edit to App.tsx?
 ❯ 1. Yes
   2. Yes, allow all edits during this session (shift+tab)
   3. No, and tell Claude what to do differently (esc)
`

// An MCP tool.
const claudeMCP = `
 Tool use
   torre - keys(name: "Shell", keys: "ls\n") (MCP)
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for torre - keys commands in /home/user/squad
   3. No, and tell Claude what to do differently (esc)
`

// The agent's answer with a numbered list: no cursor, not a prompt.
const prose = `
● Duas opções:

  Quer que eu siga com qual?
  1. Yes, refatorar agora
  2. No, deixar para depois
`

func TestParseClaude(t *testing.T) {
	p, ok := Parse(claudeBash)
	if !ok {
		t.Fatal("no prompt")
	}
	if p.Question != "Do you want to proceed?" || len(p.Options) != 4 {
		t.Fatalf("prompt: %+v", p)
	}
	if got := p.Options[1].Label; got != "Yes, and always allow access to /tmp/ask from this project" {
		t.Fatalf("wrapped label: %q", got)
	}
	want := []Choice{Allow, Always, "", Deny}
	for i, o := range p.Options {
		if o.Choice != want[i] || o.Key != string(rune('1'+i)) {
			t.Fatalf("option %d: %+v", i, o)
		}
	}
	if k, ok := Keys("claude", p, Always); !ok || k != "2" {
		t.Fatalf("always: %q %v", k, ok)
	}
	if k, ok := Keys("claude", p, Deny); !ok || k != "4" {
		t.Fatalf("deny: %q %v", k, ok)
	}
	if !reflect.DeepEqual(p.Choices(), []Choice{Allow, Always, Deny}) {
		t.Fatalf("choices: %v", p.Choices())
	}

	e, ok := Parse(claudeEdit)
	if !ok || e.Question != "Do you want to make this edit to App.tsx?" {
		t.Fatalf("edit: %+v %v", e, ok)
	}
	// Accepting all edits switches the mode: never offered as "always".
	if !reflect.DeepEqual(e.Choices(), []Choice{Allow, Deny}) {
		t.Fatalf("edit choices: %+v", e.Options)
	}

	m, ok := Parse(claudeMCP)
	if !ok || !reflect.DeepEqual(m.Choices(), []Choice{Allow, Always, Deny}) || m.Options[1].Choice != Always {
		t.Fatalf("mcp: %+v %v", m, ok)
	}
}

func TestParseNotAPrompt(t *testing.T) {
	if p, ok := Parse(prose); ok {
		t.Fatalf("prose read as a prompt: %+v", p)
	}
	if _, ok := Parse("$ ls\nfile.txt\n$ "); ok {
		t.Fatal("shell read as a prompt")
	}
	// The agent left, and the shell's prompt is under its last screen.
	if _, ok := Parse(claudeBash + "$ "); ok {
		t.Fatal("a prompt above the shell read as live")
	}
}

func TestKeysOnlyForVerifiedAgents(t *testing.T) {
	p, _ := Parse(claudeBash)
	for _, agent := range []string{"devin", "codex", "gemini", "shell", "custom"} {
		if _, ok := Keys(agent, p, Allow); ok {
			t.Fatalf("%s got keys", agent)
		}
		if c := Answerable(agent, &p); c != nil {
			t.Fatalf("%s answerable: %v", agent, c)
		}
	}
	if c := Answerable("claude", &p); len(c) != 3 {
		t.Fatalf("claude answerable: %v", c)
	}
	if Answerable("claude", nil) != nil {
		t.Fatal("no prompt yet, still answerable")
	}
}

func TestSame(t *testing.T) {
	a, _ := Parse(claudeBash)
	b, _ := Parse(claudeBash)
	c, _ := Parse(claudeMCP)
	if !a.Same(b) || a.Same(c) {
		t.Fatal("Same")
	}
}

// Devin CLI's labels (3000.11.92), read from its binary: classified, but
// never answered from the interface until its keys are checked.
func TestClassifyDevinLabels(t *testing.T) {
	for label, want := range map[string]Choice{
		"Yes, allow `npm test`":                      Allow,
		"Yes, always allow `npm test`":               Always,
		"Yes, always allow all web fetches":          Always,
		"Yes, allow calling all tools on the torre":  "",
		"Yes, switch to bypass mode":                 "",
		"Yes, implement plan and bypass permissions": "",
		"No, deny": Deny,
		"Reject":   Deny,
		"Allow":    Allow,
		"Yes, and switch to auto mode · auto mode …": "",
	} {
		if got := classify(label); got != want {
			t.Errorf("%q: %q, want %q", label, got, want)
		}
	}
}

// Gemini CLI frames its prompt in a box; Codex shows the command between
// the question and the options. Neither is verified against the real CLI,
// so they only raise the state.
func TestParseBoxedAndCodex(t *testing.T) {
	gemini := `
╭──────────────────────────────────────────────╮
│ ? Shell  ls -la                              │
│                                              │
│ Allow execution of: 'ls'?                    │
│                                              │
│ ● 1. Yes, allow once                         │
│   2. Yes, allow always ...                   │
│   3. No, suggest changes (esc)               │
╰──────────────────────────────────────────────╯
`
	p, ok := Parse(gemini)
	if !ok || p.Question != "Allow execution of: 'ls'?" || len(p.Options) != 3 || p.Options[0].Choice != Allow || p.Options[2].Choice != Deny {
		t.Fatalf("gemini: %+v %v", p, ok)
	}
	codex := `
Would you like to run the following command?

  $ npm run build
  && npm test

› 1. Yes, proceed (y)
  2. Yes, and don't ask again for this command (a)
  3. No, and tell Codex what to do differently (esc)

  Press enter to confirm or esc to cancel
`
	p, ok = Parse(codex)
	if !ok || p.Question != "Would you like to run the following command?" || !reflect.DeepEqual(p.Choices(), []Choice{Allow, Always, Deny}) {
		t.Fatalf("codex: %+v %v", p, ok)
	}
}

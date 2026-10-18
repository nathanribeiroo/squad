package attention

import (
	"reflect"
	"strings"
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

// A command that changes something, where "always" is a pattern (Claude
// Code writes "don’t" with a typographic apostrophe here).
const claudePattern = `
 Bash command
 git tag teste-torre
 This command requires approval
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don’t ask again for: git tag *
   3. Yes, and switch to auto mode · auto mode handles these prompts for you
   4. No
 Esc to cancel · Tab to amend
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

	g, ok := Parse(claudePattern)
	if !ok || !reflect.DeepEqual(Answerable("claude", &g), []Choice{Allow, Always, Deny}) || g.Options[1].Choice != Always {
		t.Fatalf("pattern: %+v %v", g, ok)
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
	for _, agent := range []string{"codex", "gemini", "shell", "custom"} {
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

// Devin CLI 3000.11.92, from a real session: a command, an edit, a web
// fetch and a torre MCP tool, each with Devin's tool header for a question.
const devinExec = `
❭ Run exactly this shell command and nothing else, then stop: touch criado.txt

 ⏺ Running command
 └ $ touch criado.txt

❭ 1 Yes  (Approve once)
  2 Yes, allow ` + "`touch`" + ` commands
  3 Yes, always allow ` + "`touch`" + ` commands in ` + "`proj`" + `
  4 Yes, always allow ` + "`touch`" + ` commands in all projects
  5 Yes, switch to bypass mode
  6 Edit command
  7 Describe change to command
  8 No
↑↓ select · ↵ confirm · esc cancel
`

const devinEdit = `
 ⏺ Read ./README.md
 └ 1 lines

 ⏺ Editing ./README.md
 │ 1 -  # proj
 └ 1 +  # projeto

❭ 1 Yes  (Approve once)
  2 Yes, switch to accept edits mode
  3 Yes, switch to bypass mode
  4 No
↑↓ select · ↵ confirm · esc cancel
`

const devinFetch = `
 ⏺ Fetching https://example.com

❭ 1 Yes  (Approve once)
  2 Yes, allow fetches to example.com/*
  3 Yes, always allow fetches to example.com/*
  4 Yes, always allow all web fetches
  5 Yes, switch to bypass mode
  6 No
↑↓ select · ↵ confirm · esc cancel
`

const devinMCP = `
 ⏺ mcp__torre__keys(name="Sh", keys="x")

❭ 1 Yes  (Approve once)
  2 Yes, allow calling keys on the torre MCP server
  3 Yes, always allow calling keys on the torre MCP server
  4 Yes, allow calling all tools on the torre MCP server
  5 Yes, always allow calling tools on the torre MCP server
  6 Yes, switch to bypass mode
  7 No
↑↓ select · ↵ confirm · esc cancel
`

// Its question tool (no permission hook for it) and the folder trust.
const devinQuestion = `
── Cor ─────────────────────────────────────────────────────────────────────
  Qual cor?
  ❭ 1 Azul
      Blue color
    2 Verde
      Green color
      Other (type your own)
────────────────────────────────────────────────────────────────────────────
↑↓ navigate · ↵ select · ? help me out · esc cancel
? Not ready to answer, help me out!
`

const devinTrust = `
 ✱ Do you trust the authors of this directory?
   For security, devin should not be run in directories with untrusted content.

 /tmp/x/proj

 ❭ 1 Yes, trust
   2 No, exit

 ↓↑ to select · ↵ to choose · esc to quit
`

func TestParseDevin(t *testing.T) {
	for _, c := range []struct {
		name, screen, question string
		choices                []Choice
		keys                   string // allow, always, deny
	}{
		// "Always" is this command in this project, never every project
		// nor this session only.
		{"exec", devinExec, "Running command", []Choice{Allow, Always, Deny}, "138"},
		{"edit", devinEdit, "Editing ./README.md", []Choice{Allow, Deny}, "1-4"},
		// This address, never every fetch; this tool, never all of them.
		{"fetch", devinFetch, "Fetching https://example.com", []Choice{Allow, Always, Deny}, "136"},
		{"mcp", devinMCP, `mcp__torre__keys(name="Sh", keys="x")`, []Choice{Allow, Always, Deny}, "137"},
	} {
		p, ok := Parse(c.screen)
		if !ok || p.Question != c.question {
			t.Fatalf("%s: %+v %v", c.name, p, ok)
		}
		if got := Answerable("devin", &p); !reflect.DeepEqual(got, c.choices) {
			t.Fatalf("%s choices: %v (%+v)", c.name, got, p.Options)
		}
		for i, ch := range []Choice{Allow, Always, Deny} {
			k, ok := Keys("devin", p, ch)
			if want := c.keys[i : i+1]; (want == "-") == ok || (ok && k != want) {
				t.Fatalf("%s %s: %q %v, want %q", c.name, ch, k, ok, want)
			}
		}
	}

	// The prompt the user typed may end in "?": the tool's header, closer
	// to the options, is the question.
	asked := strings.Replace(devinExec, "then stop: touch criado.txt", "can you touch criado.txt?", 1)
	if p, ok := Parse(asked); !ok || p.Question != "Running command" {
		t.Fatalf("header: %+v %v", p, ok)
	}

	// A question: read (the flight asks you), nothing to answer from here.
	q, ok := Parse(devinQuestion)
	if !ok || q.Question != "Qual cor?" || len(q.Options) != 2 || Answerable("devin", &q) != nil {
		t.Fatalf("question: %+v %v", q, ok)
	}
	// Trusting the folder: "No, exit" alone is no permission to deny.
	tr, ok := Parse(devinTrust)
	if !ok || tr.Options[1].Choice != Deny || Answerable("devin", &tr) != nil {
		t.Fatalf("trust: %+v %v", tr, ok)
	}

	// Devin's prompt line with something typed in it is no prompt.
	typed := "──────────\n❭ 2 bugs to fix in the parser?\n──────────\nSWE-1.6 Slow           Context: 13k / 200k tokens (6%)\n"
	if p, ok := Parse(typed); ok {
		t.Fatalf("typed prompt read: %+v", p)
	}
}

// The choices sent to the interface carry the agent's words, and a tool
// that asks at every use (keys, new_shell…) is never answered "always".
func TestSetPrompt(t *testing.T) {
	p, _ := Parse(devinExec)
	a := &Attention{Kind: Permission}
	a.SetPrompt("devin", &p)
	if !reflect.DeepEqual(a.Choices, []Choice{Allow, Always, Deny}) || a.Labels[Always] != "Yes, always allow `touch` commands in `proj`" || a.Labels[Allow] != "Yes  (Approve once)" {
		t.Fatalf("%v %v", a.Choices, a.Labels)
	}
	m, _ := Parse(devinMCP)
	a = &Attention{Kind: Permission, Tool: "mcp__torre__keys", AskEveryUse: true}
	a.SetPrompt("devin", &m)
	if !reflect.DeepEqual(a.Choices, []Choice{Allow, Deny}) || a.Labels[Always] != "" {
		t.Fatalf("every use: %v %v", a.Choices, a.Labels)
	}
	a.SetPrompt("gemini", &m)
	if a.Choices != nil || a.Labels != nil {
		t.Fatalf("unverified: %v %v", a.Choices, a.Labels)
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

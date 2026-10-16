package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"
)

// runHook forwards an agent hook to the server: `squad hook <name> [json]`.
// The payload is the last argument (Codex notify) or stdin (Claude Code,
// Devin CLI).
// It never fails loudly: a hook error must not disturb the agent.
func runHook(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: squad hook <name> [payload]")
		return 0
	}
	name := args[1]
	var payload []byte
	if len(args) >= 3 {
		payload = []byte(args[len(args)-1])
	} else if !term.IsTerminal(int(os.Stdin.Fd())) {
		payload, _ = io.ReadAll(io.LimitReader(os.Stdin, 16<<20))
	}
	env, err := readClientEnv()
	if err != nil {
		debugf(stderr, "%v", err)
		return 0
	}
	req, err := http.NewRequest("POST", strings.TrimRight(env.url, "/")+"/hooks/"+name, bytes.NewReader(payload))
	if err != nil {
		debugf(stderr, "%v", err)
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Terminal-ID", env.id)
	req.Header.Set("X-Squad-Token", env.token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		debugf(stderr, "%v", err)
		return 0
	}
	defer resp.Body.Close()
	// A hook's answer is for the agent (a session start's context).
	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(stdout, io.LimitReader(resp.Body, 1<<20))
	}
	return 0
}

func debugf(w io.Writer, format string, a ...any) {
	if os.Getenv("SQUAD_DEBUG") != "" {
		fmt.Fprintf(w, "squad hook: "+format+"\n", a...)
	}
}

// skillMD teaches agents the Squad CLI (written for Squad; Claude Code loads
// skills from ~/.claude/skills/<name>/SKILL.md, Devin CLI from
// ~/.config/devin/skills/<name>/SKILL.md, same format).
const skillMD = `---
name: squad
description: Talk to other agents and terminals connected to you on the Squad canvas, read or write connected notes, and use connected portals (a live browser page). Use when a message starts with [de: <Name> · pedido <id>], when the user asks you to ask, tell, delegate to or check on another agent, to read/update a note, or to look at, click or test something in the portal / browser.
---

# Squad

You are running inside a Squad terminal. Squad is a canvas where agents and
shells are connected by wires; you can only talk to what is connected to you.
The ` + "`squad`" + ` command is on your PATH (if not, use "$SQUAD_CLI").

## Commands

- ` + "`squad list`" + `: who you are and what is connected to you, with each agent's role (Planejador, Implementador, Revisor, Testador…). Run it first, use names exactly as printed, and give work to the agent whose role fits it. Your own role's instructions came with your session.
- ` + "`squad ask \"Name\" \"message\"`" + `: send a message and wait for the reply. The reply is the other agent's final answer for that turn (or its screen when it has no hook). Set your shell tool timeout generously (minutes for real tasks).
- ` + "`squad ask \"Name\" --raw \"keys\"`" + `: type keys into a terminal (escapes: \n Enter, \t Tab, \e ESC, \xNN byte, e.g. \x03 Ctrl-C) and get its screen back. Use for menus or shells.
- ` + "`squad ask --batch '{\"A\": \"m1\", \"B\": \"m2\"}'`" + `: ask several at once; returns a JSON array of {name, output|error}.
- ` + "`squad check \"Name\"`" + `: read a terminal's current screen without sending anything. Text still in someone's input box is not addressed to you.
- ` + "`squad reply <id> \"message\"`" + `: answer request <id>.
- ` + "`squad note …`" + `: read and write connected notes (` + "`squad note help`" + `).
- ` + "`squad portal …`" + `: use a connected portal, a browser page the user also sees live on the canvas (` + "`squad portal help`" + `).

For long messages use a heredoc: ` + "`squad ask \"Name\" <<'EOF'`" + ` … ` + "`EOF`" + `.

## Portals

A portal is a real browser tab on the canvas; the user watches what you do
in it (your clicks show with your name). Typical loop:

1. ` + "`squad portal snapshot`" + `: the page as a list like ` + "`@e3 button \"Enviar\" [x,y wxh]`" + `.
2. Act on refs: ` + "`squad portal click @e3`" + `, ` + "`squad portal fill @e2 \"text\"`" + `, ` + "`squad portal key Enter`" + `.
3. Snapshot again after anything that changes the page; refs expire then.

Also: ` + "`goto URL`" + `, ` + "`back`" + `, ` + "`reload`" + `, ` + "`scroll down`" + `, ` + "`screenshot`" + ` (prints
a PNG path you can open), ` + "`eval \"js\"`" + ` and ` + "`console`" + ` (errors and logs).
With several portals connected, name one first: ` + "`squad portal \"Docs\" snapshot`" + `.
Use it to check your own work in the running app (dev server) after changes.

## Receiving requests

A message that starts with ` + "`[de: Planejador · pedido 7f3a]`" + ` comes from the agent
Planejador, and Planejador is waiting for your answer. Do the work, then
either finish your turn with a clear final answer (Squad sends it back) or
run ` + "`squad reply 7f3a \"…\"`" + `. If you need something from the sender while it
waits, ` + "`squad ask \"Planejador\" \"…\"`" + ` delivers your message as the answer.

## Rules

- If an ask times out, don't resend it: run ` + "`squad check \"Name\"`" + ` and wait again.
- Don't edit files another agent is changing right now.
- Conversations can be paused by the user, and chains are limited in depth;
  when a command says so, stop and tell the user.
`

// runInstallSkill writes the Squad skill for Claude Code (default) or Devin
// CLI, which doesn't read ~/.claude/skills.
func runInstallSkill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install-skill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "claude", "agent that loads the skill: claude or devin")
	dir := fs.String("dir", "", "skills directory (default: the agent's global skills directory)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *dir == "" {
		d, err := skillsDir(*agent)
		if err != nil {
			fmt.Fprintln(stderr, "squad:", err)
			return 2
		}
		*dir = d
	}
	target := filepath.Join(*dir, "squad")
	if err := os.MkdirAll(target, 0o755); err != nil {
		fmt.Fprintln(stderr, "squad:", err)
		return 1
	}
	path := filepath.Join(target, "SKILL.md")
	if err := os.WriteFile(path, []byte(skillMD), 0o644); err != nil {
		fmt.Fprintln(stderr, "squad:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Skill do Squad instalada em", path)
	return 0
}

// skillsDir is where an agent loads global skills from: ~/.claude/skills for
// Claude Code; for Devin CLI, devin/skills under $XDG_CONFIG_HOME (default
// ~/.config, also on macOS) or %APPDATA% on Windows.
func skillsDir(agent string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch agent {
	case "claude":
		return filepath.Join(home, ".claude", "skills"), nil
	case "devin":
		base := os.Getenv("XDG_CONFIG_HOME")
		if runtime.GOOS == "windows" {
			base = os.Getenv("APPDATA")
		}
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "devin", "skills"), nil
	}
	return "", fmt.Errorf("unknown agent %q (use claude or devin)", agent)
}

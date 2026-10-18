package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/nathanribeiroo/squad/internal/agents"
	"github.com/nathanribeiroo/squad/internal/config"
)

// runHook forwards an agent hook to the server: `torre hook <name> [json]`.
// The payload is the last argument (Codex notify) or stdin (Claude Code,
// Devin CLI).
// It never fails loudly: a hook error must not disturb the agent.
func runHook(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: torre hook <name> [payload]")
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
	req.Header.Set("X-Torre-Token", env.token)
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
	if config.Getenv("DEBUG") != "" {
		fmt.Fprintf(w, "torre hook: "+format+"\n", a...)
	}
}

// skillMD teaches agents the Torre CLI (written for Torre; Claude Code loads
// skills from ~/.claude/skills/<name>/SKILL.md, Devin CLI from
// ~/.config/devin/skills/<name>/SKILL.md, same format).
const skillMD = `---
name: torre
description: Talk to other agents and terminals connected to you on the Torre canvas, read or write connected notes, use or open portals (a live browser page) and, as a manager, create your team of agents and floors (isolated copies of the repository). Use when a message starts with [de: <Name> · pedido <id>], when the user asks you to ask, tell, delegate to or check on another agent, to plan work for several agents, to read/update a note, or to look at, click or test something in the portal / browser.
---

# Torre

You are running inside a Torre terminal. Torre is a canvas where agents and
shells are connected by wires; you can only talk to what is connected to you.
The ` + "`torre`" + ` command is on your PATH (if not, use "$TORRE_CLI").

If your session has the torre MCP tools (` + "`mcp__torre__list`" + `, ` + "`ask`" + `, ` + "`note`" + `,
` + "`portal`" + `, ` + "`new`" + `…), prefer them: same commands with typed arguments, and most
need no approval. ` + "`keys`" + `, ` + "`new_shell`" + ` and ` + "`portal_eval`" + ` can run commands, and
` + "`floor_land`" + ` merges branches, so the user approves each call. The CLI below
works everywhere.

## Commands

- ` + "`torre list`" + `: who you are and what is connected to you, with each agent's role (Planejador, Implementador, Revisor, Testador…). Run it first, use names (or the short callsigns, like IMP) exactly as printed, and give work to the agent whose role fits it. Your own role's instructions came with your session.
- ` + "`torre ask \"Name\" \"message\"`" + `: send a message and wait for the reply. The reply is the other agent's final answer for that turn (or its screen when it has no hook). An agent in the middle of a turn gets your message when that turn ends (requests to it go in one at a time). Set your shell tool timeout generously (minutes for real tasks).
- ` + "`torre ask \"Name\" --raw \"keys\"`" + `: type keys into a terminal (escapes: \n Enter, \t Tab, \e ESC, \xNN byte, e.g. \x03 Ctrl-C) and get its screen back. Use for menus or shells.
- ` + "`torre ask --batch '{\"A\": \"m1\", \"B\": \"m2\"}'`" + `: ask several at once; returns a JSON array of {name, output|error}.
- ` + "`torre check \"Name\"`" + `: read a terminal's current screen without sending anything. Text still in someone's input box is not addressed to you.
- ` + "`torre wait \"Name\"`" + `: wait for an agent's current turn to end and get its final answer, without sending anything (as with ask, set a generous shell tool timeout). Use it when an ask timed out while the agent was still working. If the agent isn't in a turn, you get its last answer at once. A message it sends you meanwhile ends the wait: answer with ` + "`torre reply`" + `, then wait again.
- ` + "`torre reply <id> \"message\"`" + `: answer request <id>.
- ` + "`torre note …`" + `: read and write connected notes (` + "`torre note help`" + `).
- ` + "`torre portal …`" + `: use a connected portal, a browser page the user also sees live on the canvas (` + "`torre portal help`" + `).
- ` + "`torre new`" + `, ` + "`torre connect`" + `, ` + "`torre close`" + `: managers only, see below.
- ` + "`torre floor list`" + `: the floors of the workspace; ` + "`create`" + ` and ` + "`land`" + ` are for managers, see below.

For long messages use a heredoc: ` + "`torre ask \"Name\" <<'EOF'`" + ` … ` + "`EOF`" + `.

## Roles

` + "`torre list`" + ` shows each agent's role; ` + "`torre roles`" + ` lists all of them with what
they do. The Arquiteto designs (a note "Arquitetura: …" with modules, contracts,
risks and parts that can run in parallel) and doesn't write product code. If
an architecture note is connected to you, read it before you implement or
review, and follow it; ask its author before departing from it.

## Notes

A note's name is unique in the workspace and permanent. When you create one
(` + "`torre note create \"Name\"`" + `), give it a clear name; don't create a second
note for the same thing, use ` + "`torre note list`" + ` to see the ones you have.
Names ignore case. The user may rename a note they made: its former names
keep working, and the reply tells you the current one; use that from then on.
The user can stack notes: a wire to a stack reaches every note in it, and
` + "`torre list`" + ` / ` + "`torre note list`" + ` show them with ` + "`stack: \"Name\"`" + `. Read and
write each one by its own name.

## Portals

A portal is a real browser tab on the canvas; the user watches what you do
in it (your clicks show with your name). Typical loop:

1. ` + "`torre portal snapshot`" + `: the page as a list like ` + "`@e3 button \"Enviar\" [x,y wxh]`" + `.
2. Act on refs: ` + "`torre portal click @e3`" + `, ` + "`torre portal fill @e2 \"text\"`" + `, ` + "`torre portal key Enter`" + `.
3. Snapshot again after anything that changes the page; refs expire then.

Also: ` + "`goto URL`" + `, ` + "`back`" + `, ` + "`reload`" + `, ` + "`scroll down`" + `, ` + "`screenshot`" + ` (prints
a PNG path you can open), ` + "`eval \"js\"`" + ` and ` + "`console`" + ` (errors and logs).
With several portals connected, name one first: ` + "`torre portal \"Docs\" snapshot`" + `.
Use it to check your own work in the running app (dev server) after changes.

No portal connected? Open one: ` + "`torre portal create localhost:5173 \"App\"`" + ` creates it
next to you, connected to you (a few per agent). ` + "`torre portal close \"App\"`" + `
removes one you created.

## Building a team (managers)

` + "`torre list`" + ` says "You are a manager" when your terminal may create others
(the Planejador role does). Then:

1. Plan the work and decide who you need: one agent per independent task,
   with the role that fits (` + "`torre roles`" + ` lists them: arch, impl, rev, test and the
   user's own). Call an Arquiteto (` + "`--role arch`" + `) when the change touches the
   structure (a new module, an API, a data format, a dependency) or will be
   split into parallel fronts; skip it for small, local tasks.
2. ` + "`torre new \"Bia\" --role impl`" + ` creates a terminal next to you, connected to
   you, with that role and your agent, and returns once it is ready.
   Options: ` + "`--agent codex|devin|gemini|claude|shell`" + `, ` + "`--cwd folder`" + `,
   ` + "`--command \"npm run dev\"`" + ` (with ` + "`--agent shell`" + `, for a dev server).
3. Delegate with ` + "`torre ask`" + ` (` + "`--batch`" + ` for parallel work) and check the results:
   have the Revisor review and the Testador test before telling the user it's done.
   The Arquiteto writes its design in a note ("Arquitetura: …"): connect that
   note to whoever implements and reviews, and use the parts it proposes to
   decide the floors.
4. ` + "`torre connect \"A\" \"B\"`" + ` wires two nodes (e.g. the Testador to a portal or
   the Implementador), ` + "`torre close \"Name\"`" + ` removes a terminal you created that
   is no longer needed. Leave the rest for the user to see.

Agents you create can't create others, and there is a limit per manager.
When one of them stops at a permission prompt (Claude Code's "Do you want to
proceed?", Devin's approval menu, Codex's approval request), don't answer it
for them with keys: tell the user, who sees that terminal on the canvas (on
its floor) and decides.

## Floors (managers)

A floor is an isolated copy of the repository: a git worktree on its own
branch, outside the project folder, with its own canvas. Use one per front
of work that would otherwise touch the same files at the same time.

1. ` + "`torre floor create \"Login\"`" + ` makes it from the last commit (uncommitted
   changes stay behind), on branch ` + "`login`" + `, and runs the workspace's setup command.
2. ` + "`torre new \"Bia\" --role impl --floor \"Login\"`" + ` creates an agent on that floor,
   connected to you; it works in the floor's folder. Tell it to commit there.
3. When the floor's work is done and reviewed: ` + "`torre floor land \"Login\" --merge`" + `
   merges its branch into the branch it started from (` + "`--into main`" + ` for another),
   closes its terminals and removes its folder; the branch is kept. Without
   ` + "`--merge`" + ` nothing is merged. Landing is refused while the floor has uncommitted
   changes, and a merge with conflicts changes nothing: have an agent on the
   floor merge the base branch into the floor's branch, resolve, commit, and
   land again.

You can only land floors you created; the user can land or discard any floor.

## Receiving requests

A message that starts with ` + "`[de: Planejador · pedido 7f3a]`" + ` comes from the agent
Planejador, and Planejador is waiting for your answer. Do the work, then
either finish your turn with a clear final answer (Torre sends it back) or
run ` + "`torre reply 7f3a \"…\"`" + `. If you need something from the sender while it
waits, ` + "`torre ask \"Planejador\" \"…\"`" + ` delivers your message as the answer.

## Rules

- If an ask times out while the agent is still in its turn, don't resend it: ` + "`torre wait \"Name\"`" + ` returns its answer when it finishes.
- If Torre restarted while you waited (a torre command failed to reach it, or
  you were told your work was interrupted), agents continue their
  conversations: use ` + "`torre wait \"Name\"`" + ` or ` + "`torre check`" + ` before resending a request.
- When you were asked something, your final answer is what goes back: don't
  end your turn with progress ("still waiting for X") while agents you called
  are working. Wait for them; stop early only when something needs the user
  (a question or a permission prompt on someone's screen), and say what.
- Don't edit files another agent is changing right now.
- Conversations can be paused by the user, and chains are limited in depth;
  when a command says so, stop and tell the user.
`

// runInstallSkill writes the Torre skill for Claude Code (default) or Devin
// CLI, which doesn't read ~/.claude/skills.
func runInstallSkill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install-skill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "claude, devin or all (default: each of them found on this machine)")
	dir := fs.String("dir", "", "skills directory (default: each agent's global skills directory)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	var targets, missing []string
	switch *agent {
	case "":
		targets, missing = installedAgents()
		if *dir != "" {
			targets, missing = []string{"claude"}, nil
		}
	case "all":
		targets = []string{"claude", "devin"}
	case "claude", "devin":
		targets = []string{*agent}
	default:
		fmt.Fprintf(stderr, "torre: unknown agent %q (use claude, devin or all)\n", *agent)
		return 2
	}
	if *dir != "" && len(targets) > 1 {
		fmt.Fprintln(stderr, "torre: -dir takes one agent (-agent claude or -agent devin)")
		return 2
	}
	fmt.Fprintln(stdout, "torre", versionString())
	for _, a := range targets {
		d := *dir
		if d == "" {
			var err error
			if d, err = skillsDir(a); err != nil {
				fmt.Fprintln(stderr, "torre:", err)
				return 1
			}
		}
		path := filepath.Join(d, "torre", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(stderr, "torre:", err)
			return 1
		}
		if err := os.WriteFile(path, []byte(skillMD), 0o644); err != nil {
			fmt.Fprintln(stderr, "torre:", err)
			return 1
		}
		fmt.Fprintf(stdout, "Skill da Torre instalada para o %s em %s\n", agentLabel[a], path)
		if old, ok := removeSquadSkill(d); ok {
			fmt.Fprintf(stdout, "Skill antiga do Squad removida (%s)\n", old)
		}
		if a == "devin" {
			if code := registerDevinMCP(stdout, stderr); code != 0 {
				return code
			}
		}
	}
	for _, a := range missing {
		fmt.Fprintf(stdout, "%s não encontrado nesta máquina (%s). Para instalar nele mesmo assim: torre install-skill -agent %s\n", agentLabel[a], agentHint(a), a)
	}
	return 0
}

var agentLabel = map[string]string{"claude": "Claude Code", "devin": "Devin"}

// installedAgents splits the agents with a skill format into those found on
// this machine (their CLI on the PATH, or their configuration folder) and the
// others; Claude Code counts as found when none is.
func installedAgents() (found, missing []string) {
	home, _ := os.UserHomeDir()
	d, _ := agents.DevinConfigDir()
	for _, a := range []string{"claude", "devin"} {
		dir := filepath.Join(home, ".claude")
		if a == "devin" {
			dir = d
		}
		if _, err := exec.LookPath(a); err == nil || (dir != "" && isDir(dir)) {
			found = append(found, a)
		} else {
			missing = append(missing, a)
		}
	}
	if len(found) == 0 {
		found, missing = []string{"claude"}, []string{"devin"}
	}
	return found, missing
}

// agentHint says where install-skill looked for an agent.
func agentHint(a string) string {
	if a == "devin" {
		d, _ := agents.DevinConfigDir()
		return "nem devin no PATH nem a pasta " + d
	}
	return "nem claude no PATH nem a pasta ~/.claude"
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// registerDevinMCP adds the torre MCP server to Devin's mcp_config.json:
// Devin reads MCP servers only from there, not from the config file Torre
// passes it. Outside a Torre terminal, torre mcp serves no tools.
func registerDevinMCP(stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
	}
	if err != nil || strings.Contains(exe, "go-build") { // go run: a temporary binary
		if p, err := exec.LookPath("torre"); err == nil {
			exe, _ = filepath.Abs(p)
		}
	}
	d, err := agents.DevinConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, "torre:", err)
		return 1
	}
	path := filepath.Join(d, "mcp_config.json")
	changed, err := agents.AddDevinMCP(path, exe)
	if err != nil {
		fmt.Fprintln(stderr, "torre: register the MCP server for Devin:", err)
		return 1
	}
	if changed {
		fmt.Fprintf(stdout, "Servidor MCP torre registrado no Devin em %s\n", path)
	} else {
		fmt.Fprintf(stdout, "Servidor MCP torre já estava registrado no Devin (%s)\n", path)
	}
	return 0
}

// removeSquadSkill removes the skill `squad install-skill` wrote in skills
// directory d before Squad became Torre (only that file, and its folder
// when nothing else is in it).
func removeSquadSkill(d string) (string, bool) {
	dir := filepath.Join(d, "squad")
	path := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "---\nname: squad\n") {
		return "", false
	}
	if os.Remove(path) != nil {
		return "", false
	}
	_ = os.Remove(dir) // fails, and stays, when something else is in it
	return path, true
}

// skillsDir is where an agent loads global skills from: ~/.claude/skills for
// Claude Code; for Devin CLI, skills in its configuration folder.
func skillsDir(agent string) (string, error) {
	switch agent {
	case "claude":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude", "skills"), nil
	case "devin":
		d, err := agents.DevinConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "skills"), nil
	}
	return "", fmt.Errorf("unknown agent %q (use claude or devin)", agent)
}

package agents

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nathanribeiroo/squad/internal/ids"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Conversations: each agent CLI keeps its own conversations. Squad remembers
// the id of the one a terminal is in (store.TerminalData.Conversation) and
// continues it when the terminal starts again:
//
//	agent   new conversation              continue            id learned from
//	claude  --session-id <id> (Squad's)    --resume <id>       Squad; hooks after /clear or /resume
//	devin   its own id                     --resume <id>       hooks (SessionStart, prompt, stop)
//	codex   its own id                     codex resume <id>   notify (thread-id, at turn ends)
//	gemini  --session-id <id> (Squad's)    --resume <id>       Squad (versions with --session-id)
//
// Continuing falls back on the launch line itself (terminal.OrElse): an agent
// that can't find the conversation exits with an error, and a new one starts,
// with the same id where Squad picks it, so the id it remembers stays valid.

var conversationID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidConversation reports whether id can be an agent's conversation id on a
// launch line: hooks report it, and it must never become an option or a
// shell word.
func ValidConversation(id string) bool { return conversationID.MatchString(id) }

func newConversationID() string { return strings.ToLower(ids.New()) }

// userPicksConversation reports whether the user's own launch line already
// picks the conversation; Squad then leaves it alone.
func userPicksConversation(agent, cmd string) bool {
	for _, f := range strings.Fields(cmd) {
		name, _, _ := strings.Cut(f, "=")
		switch {
		case name == "--resume" || name == "--continue" || name == "--session-id":
			return true
		case agent == "codex" && (f == "resume" || f == "fork"):
			return true
		case agent != "codex" && (f == "-r" || f == "-c"):
			return true
		}
	}
	return false
}

// codexResume puts the resume subcommand after the program, where Codex
// expects it: codex resume <id> [options]. ok is false when the line doesn't
// start with the codex program.
func codexResume(cmd, arg string) (string, bool) {
	prog, rest, _ := strings.Cut(cmd, " ")
	base := strings.ToLower(filepath.Base(strings.Trim(prog, `"'`)))
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".exe"), ".cmd")
	if base != "codex" {
		return cmd, false
	}
	line := prog + " resume " + arg
	if rest != "" {
		line += " " + rest
	}
	return line, true
}

// program is the first word of a launch line.
func program(cmd string) string {
	prog, _, _ := strings.Cut(strings.TrimSpace(cmd), " ")
	return strings.Trim(prog, `"'`)
}

var helpCache sync.Map // "program flag" → bool

// helpMentions reports whether a program's --help mentions flag, asked once
// per program (Gemini CLI has --session-id only in newer versions).
func helpMentions(prog, flag string) bool {
	key := prog + " " + flag
	if v, ok := helpCache.Load(key); ok {
		return v.(bool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, prog, "--help").CombinedOutput()
	ok := bytes.Contains(out, []byte(flag))
	helpCache.Store(key, ok)
	return ok
}

// conversationLine returns the launch line that starts (or continues) the
// terminal's conversation. base is the agent's line without it; tail is
// appended to a line that starts a new conversation only (Gemini's first
// prompt with the role). conv is the remembered id, "" for none; it returns
// the id the terminal is now in ("" when the agent picks it) and whether the
// line continues a conversation.
func (l *Launcher) conversationLine(agent, base, tail, conv string, fam terminal.ShellFamily) (line, id string, resumed bool) {
	if !ValidConversation(conv) {
		conv = ""
	}
	q := func(s string) string { return terminal.Quote(fam, s) }
	switch agent {
	case "claude":
		if conv == "" {
			conv = newConversationID()
			return base + " --session-id " + q(conv) + tail, conv, false
		}
		return terminal.OrElse(fam, base+" --resume "+q(conv), base+" --session-id "+q(conv)+tail), conv, true
	case "gemini":
		has := l.HasFlag
		if has == nil {
			has = helpMentions
		}
		if !has(program(base), "--session-id") {
			return base + tail, "", false
		}
		if conv == "" {
			conv = newConversationID()
			return base + " --session-id " + q(conv) + tail, conv, false
		}
		return terminal.OrElse(fam, base+" --resume "+q(conv), base+" --session-id "+q(conv)+tail), conv, true
	case "devin":
		if conv == "" {
			return base + tail, "", false
		}
		return terminal.OrElse(fam, base+" --resume "+q(conv), base+tail), conv, true
	case "codex":
		if conv == "" {
			return base + tail, "", false
		}
		if line, ok := codexResume(base, q(conv)); ok {
			return terminal.OrElse(fam, line, base+tail), conv, true
		}
		return base + tail, conv, false // a custom line: keep the id for the hooks
	}
	return base + tail, "", false
}

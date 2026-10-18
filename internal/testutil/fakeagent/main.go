// Command fakeagent imitates an agent TUI for tests. It enables bracketed
// paste, reads input in raw mode and logs every chunk (hex, with timestamps)
// to $FAKEAGENT_LOG. Each submitted message (text followed by CR) triggers a
// "turn": progress lines for $FAKEAGENT_WORK seconds, an answer line, then
// "DONE". Special messages:
//
//	RUN:<shell command>   runs the command (e.g. `torre ask …`) and prints its output;
//	                      {id} and {from} are replaced from the [de: … · pedido …] envelope
//	SLEEP:<seconds>       stays silent for that long before answering
//	ASKPERM:<command>     asks permission to run the command like Claude Code
//	                      2.1.302 (the permission hook, then the numbered
//	                      prompt) and reads one key: 1, 2 or 3 go on, 4 or Esc
//	                      interrupt the turn without its end hook. With
//	                      $FAKEAGENT_HOOK=devin, like Devin CLI 3000.11.92:
//	                      its "exec" hook and prompt (1–7 go on, 8 or Esc
//	                      deny), or its MCP prompt for a command that is a
//	                      tool name (mcp__torre__keys: 1–6 go on, 7 denies)
//	ASKQ:<question>       asks a question with two options like Devin's
//	                      question tool (no hook) and reads one key
//	ASKCQ:<question>      asks a question like Claude Code's AskUserQuestion
//	                      (its permission hook, then the screen), with two
//	                      options ("question|one|two"; Azul, Verde): 1, 2 or 4
//	                      pick an option, 3 opens the text field (the text up
//	                      to CR is the answer), Esc declines
//	PLAN:<title>          asks to approve a plan like Claude Code's
//	                      ExitPlanMode: 1 or 2 approve, 3 then text and CR
//	                      sends feedback, Esc declines
//
// $FAKEAGENT_WIDTH sets the width of progress lines (default 60).
// $FAKEAGENT_HOOK=claude makes it behave like Claude Code with Torre's hooks:
// `torre hook claude-session-start` when it starts, `torre hook claude-prompt`
// when a turn starts and `torre hook claude-stop` (with last_assistant_message)
// when it ends, each with its session_id. $FAKEAGENT_HOOK=devin does the same
// with Devin CLI's hook names (devin-session-start, devin-prompt, devin-stop).
//
// Its conversation id comes from --session-id or --resume, like Claude Code,
// or else $FAKEAGENT_SESSION (Devin picks its own). With
// $FAKEAGENT_RESUME_FAIL set, --resume fails like an agent that can't find
// the conversation: it prints an error and exits 1.
//
// An empty CR (the reinforcement Enter) is ignored. Ctrl-C exits with 130.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

var logFile *os.File

func logf(format string, a ...any) {
	if logFile != nil {
		fmt.Fprintf(logFile, "%d "+format+"\n", append([]any{time.Now().UnixMilli()}, a...)...)
	}
}

func out(s string) { _, _ = os.Stdout.WriteString(strings.ReplaceAll(s, "\n", "\r\n")) }

func main() {
	if p := os.Getenv("FAKEAGENT_LOG"); p != "" {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			logFile = f
			defer f.Close()
		}
	}
	work := 1.0
	if w, err := strconv.ParseFloat(os.Getenv("FAKEAGENT_WORK"), 64); err == nil {
		work = w
	}
	if os.Getenv("FAKEAGENT_NOPASTE") == "" {
		out("\x1b[?2004h")
	}
	if old, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer term.Restore(int(os.Stdin.Fd()), old)
	}
	logf("START %q", os.Args[1:])
	session, source := os.Getenv("FAKEAGENT_SESSION"), "startup"
	for i, a := range os.Args[1:] {
		if (a == "--session-id" || a == "--resume") && i+2 < len(os.Args) {
			session = os.Args[i+2]
			if a == "--resume" {
				source = "resume"
			}
		}
	}
	if source == "resume" && os.Getenv("FAKEAGENT_RESUME_FAIL") != "" {
		out("No conversation found with session ID: " + session + "\n")
		logf("RESUME-FAIL")
		os.Exit(1)
	}
	sessionID = session
	start, _ := json.Marshal(map[string]any{"hook_event_name": "SessionStart", "source": source, "session_id": session})
	hook("session-start", string(start))
	out("fakeagent ready\n> ")

	var (
		buf     bytes.Buffer // current message
		inPaste bool
		pending []byte // bytes that may be the start of a paste marker
	)
	chunk := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(chunk)
		if n > 0 {
			logf("IN %x", chunk[:n])
			data := append(pending, chunk[:n]...)
			pending = nil
			for len(data) > 0 {
				switch {
				case bytes.HasPrefix(data, []byte(pasteStart)):
					inPaste = true
					data = data[len(pasteStart):]
				case bytes.HasPrefix(data, []byte(pasteEnd)):
					inPaste = false
					data = data[len(pasteEnd):]
				case data[0] == 0x1b && len(data) < len(pasteStart) && strings.HasPrefix(pasteStart, string(data)):
					pending = data // incomplete marker; wait for more input
					data = nil
				case data[0] == 0x03:
					logf("CTRL-C")
					out("^C\n")
					os.Exit(130)
				case data[0] == '\r' && !inPaste:
					msg := buf.String()
					buf.Reset()
					data = data[1:]
					if strings.TrimSpace(msg) == "" {
						logf("EMPTY-CR")
						continue
					}
					logf("SUBMIT %q", msg)
					turn(msg, work)
				default:
					buf.WriteByte(data[0])
					data = data[1:]
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// sessionID is the conversation id the hooks report.
var sessionID string

var envelopeRe = regexp.MustCompile(`^\[de: (.+?) · pedido ([0-9a-f]+)\]`)

// hook runs `torre hook <agent>-<event>` when $FAKEAGENT_HOOK names an agent
// with hooks (claude or devin).
func hook(event string, payload string) {
	agent := os.Getenv("FAKEAGENT_HOOK")
	if agent != "claude" && agent != "devin" {
		return
	}
	name := agent + "-" + event
	cmd := exec.Command("torre", "hook", name)
	cmd.Stdin = strings.NewReader(payload)
	o, err := cmd.CombinedOutput()
	logf("HOOK %s err=%v out=%q", name, err, o)
}

func turn(msg string, work float64) {
	prompt, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": msg, "session_id": sessionID})
	hook("prompt", string(prompt))
	first, _, _ := strings.Cut(msg, "\n")
	from, id := "", ""
	if m := envelopeRe.FindStringSubmatch(msg); m != nil {
		from, id = m[1], m[2]
	}
	out("\nreceived: " + first + "\n")
	if cmdline, ok := strings.CutPrefix(lastLine(msg), "ASKPERM:"); ok && !askPermission(cmdline) {
		return
	}
	if q, ok := strings.CutPrefix(lastLine(msg), "ASKQ:"); ok {
		out("── Pergunta ──────────────────────────────\n  " + q + "\n  ❭ 1 Azul\n      Blue color\n    2 Verde\n      Green color\n" +
			"──────────────────────────────────────────\n↑↓ navigate · ↵ select · ? help me out · esc cancel\n? Not ready to answer, help me out!\n")
		readChoice(0x1b, '2') // 1 or 2 answers, Esc cancels
	}
	if q, ok := strings.CutPrefix(lastLine(msg), "ASKCQ:"); ok {
		// "question|option one|option two" (Azul and Verde by default).
		parts := strings.Split(q, "|")
		q = parts[0]
		o1, o2 := "Azul", "Verde"
		if len(parts) == 3 {
			o1, o2 = parts[1], parts[2]
		}
		input, _ := json.Marshal(map[string]any{"questions": []any{map[string]any{"question": q, "header": "Cor", "multiSelect": false,
			"options": []any{map[string]any{"label": o1, "description": "The first"}, map[string]any{"label": o2, "description": "The second"}}}}})
		hook("permission", `{"hook_event_name":"PermissionRequest","session_id":"`+sessionID+`","tool_name":"AskUserQuestion","tool_input":`+string(input)+`}`)
		out("────────────────────────────────────────\n ☐ Cor\n\n" + q + "\n\n❯ 1. " + o1 + "\n     The first\n  2. " + o2 + "\n     The second\n  3. Type something.\n" +
			"────────────────────────────────────────\n  4. Chat about this\n\nEnter to select · ↑/↓ to navigate · Esc to cancel\n")
		readOption('4', '3')
	}
	if title, ok := strings.CutPrefix(lastLine(msg), "PLAN:"); ok {
		plan := "# " + title + "\n\n1. Create hello.txt\n2. Read it back\n"
		input, _ := json.Marshal(map[string]any{"plan": plan, "planFilePath": "/tmp/plan.md"})
		hook("permission", `{"hook_event_name":"PermissionRequest","session_id":"`+sessionID+`","tool_name":"ExitPlanMode","tool_input":`+string(input)+`}`)
		out("   Here is Claude's plan:\n   " + title + "\n  ────────────────────────────────────────\n" +
			"   Claude has written up a plan and is ready to execute. Would you like to proceed?\n\n" +
			"   ❯ 1. Yes, and use auto mode\n     2. Yes, manually approve edits\n     3. Tell Claude what to change\n" +
			"        shift+tab to approve with this feedback\n\n   ctrl+g to edit in Vim · ~/.claude/plans/plan.md\n")
		readOption('2', '3')
	}
	if rest, ok := strings.CutPrefix(lastLine(msg), "SLEEP:"); ok {
		if s, err := strconv.ParseFloat(rest, 64); err == nil {
			time.Sleep(time.Duration(s * float64(time.Second)))
		}
	}
	if cmdline, ok := strings.CutPrefix(lastLine(msg), "RUN:"); ok {
		cmdline = strings.NewReplacer("{id}", id, "{from}", from).Replace(cmdline)
		shell, flag := "/bin/sh", "-c"
		if runtime.GOOS == "windows" {
			shell, flag = "cmd.exe", "/c"
		}
		o, err := exec.Command(shell, flag, cmdline).CombinedOutput()
		logf("RUN %q err=%v out=%q", cmdline, err, o)
		out(string(o))
	}
	width := 60
	if w, err := strconv.Atoi(os.Getenv("FAKEAGENT_WIDTH")); err == nil && w > 0 {
		width = w
	}
	end := time.Now().Add(time.Duration(work * float64(time.Second)))
	for i := 1; time.Now().Before(end); i++ {
		out(fmt.Sprintf("working %d %s\n", i, strings.Repeat("#", width)))
		time.Sleep(200 * time.Millisecond)
	}
	answer := "answer: " + strings.ToUpper(first)
	out(answer + "\nDONE\n> ")
	logf("TURN-END")
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "Stop", "last_assistant_message": "final: " + first, "session_id": sessionID})
	hook("stop", string(payload))
}

// lastLine returns the last line of msg (after the [de: …] envelope line).
func lastLine(msg string) string {
	msg = strings.TrimRight(msg, "\n")
	if i := strings.LastIndex(msg, "\n"); i >= 0 {
		return msg[i+1:]
	}
	if i := strings.LastIndex(msg, "] "); i >= 0 && strings.HasPrefix(msg, "[") {
		return msg[i+2:]
	}
	return msg
}

// askPermission shows Claude Code's permission prompt for a Bash command (or
// Devin's, as Devin) and reads the answer; false when it was denied (the
// turn is interrupted).
func askPermission(cmdline string) bool {
	if os.Getenv("FAKEAGENT_HOOK") == "devin" {
		return askDevin(cmdline)
	}
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "PermissionRequest", "session_id": sessionID,
		"tool_name": "Bash", "tool_input": map[string]any{"command": cmdline, "description": "Run it"}})
	hook("permission", string(payload))
	out(" Bash command\n │ " + cmdline + "\n Do you want to proceed?\n ❯ 1. Yes\n   2. Yes, and always allow access to\n      /tmp from this project\n" +
		"   3. Yes, and switch to auto mode\n   4. No\n\n Esc to cancel · Tab to amend\n")
	return readChoice('4', '3')
}

// askDevin is Devin CLI's permission prompt: for a command, or for an MCP
// tool when cmdline is a tool name.
func askDevin(cmdline string) bool {
	tool, input := "exec", map[string]any{"command": cmdline}
	if strings.HasPrefix(cmdline, "mcp__") {
		tool, input = cmdline, map[string]any{"name": "Sh", "keys": "x"}
	}
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "PermissionRequest", "session_id": sessionID, "tool_name": tool, "tool_input": input})
	hook("permission", string(payload))
	if tool != "exec" {
		out(" ⏺ " + tool + "(name=\"Sh\", keys=\"x\")\n\n❭ 1 Yes  (Approve once)\n  2 Yes, allow calling keys on the torre MCP server\n" +
			"  3 Yes, always allow calling keys on the torre MCP server\n  4 Yes, allow calling all tools on the torre MCP server\n" +
			"  5 Yes, always allow calling tools on the torre MCP server\n  6 Yes, switch to bypass mode\n  7 No\n↑↓ select · ↵ confirm · esc cancel\n")
		return readChoice('7', '6')
	}
	word, _, _ := strings.Cut(cmdline, " ")
	out(" ⏺ Running command\n └ $ " + cmdline + "\n\n❭ 1 Yes  (Approve once)\n  2 Yes, allow `" + word + "` commands\n" +
		"  3 Yes, always allow `" + word + "` commands in `proj`\n  4 Yes, always allow `" + word + "` commands in all projects\n" +
		"  5 Yes, switch to bypass mode\n  6 Edit command\n  7 Describe change to command\n  8 No\n↑↓ select · ↵ confirm · esc cancel\n")
	return readChoice('8', '7')
}

// readOption reads the answer to a question or a plan: a key from 1 up to
// last picks an option, text opens the text field (its answer is what comes
// before CR), Esc declines. Ctrl-C exits.
func readOption(last, text byte) {
	key := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(key)
		if err != nil {
			return
		}
		logf("ASK-KEY %x", key[:n])
		switch c := key[0]; {
		case c == 0x03:
			logf("CTRL-C")
			os.Exit(130)
		case c == 0x1b:
			out("\x1b[2J\x1b[H")
			logf("ASK-DECLINED")
			return
		case c == text:
			var typed []byte
			for rest := key[1:n]; ; {
				if i := bytes.IndexByte(rest, '\r'); i >= 0 {
					typed = append(typed, rest[:i]...)
					break
				}
				typed = append(typed, rest...)
				m, err := os.Stdin.Read(key)
				if err != nil {
					return
				}
				logf("ASK-KEY %x", key[:m])
				rest = key[:m]
			}
			out("\x1b[2J\x1b[H")
			logf("ASK-TEXT %q", typed)
			out(fmt.Sprintf("answered %s\n", typed))
			return
		case c >= '1' && c <= last:
			out("\x1b[2J\x1b[H")
			logf("ASK-CHOSE %c", c)
			out(fmt.Sprintf("chose %c\n", c))
			return
		}
	}
}

// readChoice reads the key that answers a prompt: deny or Esc deny (false),
// 1 up to last go on; Ctrl-C exits.
func readChoice(deny, last byte) bool {
	key := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(key)
		if err != nil {
			return false
		}
		logf("ASK-KEY %x", key[:n])
		c := key[0]
		if c == 0x03 {
			logf("CTRL-C")
			os.Exit(130)
		}
		if c >= '1' && c <= last || c == deny || c == 0x1b {
			out("\x1b[2J\x1b[H")
			if c == deny || c == 0x1b {
				out("Interrupted · What should Claude do instead?\n> ")
				logf("ASK-DENIED")
				return false
			}
			out(fmt.Sprintf("chose %c\n", c))
			logf("ASK-CHOSE %c", c)
			return true
		}
	}
}

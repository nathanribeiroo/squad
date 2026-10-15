// Command fakeagent imitates an agent TUI for tests. It enables bracketed
// paste, reads input in raw mode and logs every chunk (hex, with timestamps)
// to $FAKEAGENT_LOG. Each submitted message (text followed by CR) triggers a
// "turn": progress lines for $FAKEAGENT_WORK seconds, an answer line, then
// "DONE". Special messages:
//
//	RUN:<shell command>   runs the command (e.g. `squad ask …`) and prints its output;
//	                      {id} and {from} are replaced from the [de: … · pedido …] envelope
//	SLEEP:<seconds>       stays silent for that long before answering
//
// $FAKEAGENT_WIDTH sets the width of progress lines (default 60).
// $FAKEAGENT_HOOK=claude makes it behave like Claude Code with Squad's hooks:
// `squad hook claude-prompt` when a turn starts and `squad hook claude-stop`
// (with last_assistant_message) when it ends.
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
	out("fakeagent ready\n> ")
	logf("START")

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

var envelopeRe = regexp.MustCompile(`^\[de: (.+?) · pedido ([0-9a-f]+)\]`)

func hook(name string, payload string) {
	if os.Getenv("FAKEAGENT_HOOK") != "claude" {
		return
	}
	cmd := exec.Command("squad", "hook", name)
	cmd.Stdin = strings.NewReader(payload)
	o, err := cmd.CombinedOutput()
	logf("HOOK %s err=%v out=%q", name, err, o)
}

func turn(msg string, work float64) {
	hook("claude-prompt", `{"hook_event_name":"UserPromptSubmit"}`)
	first, _, _ := strings.Cut(msg, "\n")
	from, id := "", ""
	if m := envelopeRe.FindStringSubmatch(msg); m != nil {
		from, id = m[1], m[2]
	}
	out("\nreceived: " + first + "\n")
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
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "Stop", "last_assistant_message": "final: " + first})
	hook("claude-stop", string(payload))
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

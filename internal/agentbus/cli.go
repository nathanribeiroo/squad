package agentbus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/nathanribeiroo/squad/internal/store"
)

// CLIRequest is the body of POST /cli.
type CLIRequest struct {
	Args []string `json:"args"`
	Cwd  string   `json:"cwd"`
	// Safe marks calls from tools the agent may use without asking the user
	// (squad mcp): nothing they do may run a command. See Safe.
	Safe bool `json:"safe,omitempty"`
}

type safeKey struct{}

// WithSafe marks ctx as a call that must not run commands.
func WithSafe(ctx context.Context) context.Context { return context.WithValue(ctx, safeKey{}, true) }

// Safe reports whether a call must not run commands: no keys typed into a
// terminal, no message to a shell (it would run as a command line), no new
// terminal with a command line, no JavaScript in a portal.
func Safe(ctx context.Context) bool { v, _ := ctx.Value(safeKey{}).(bool); return v }

// IsAgent reports whether an agent type is an AI agent, which reads messages
// as prompts (shells and custom commands run them).
func IsAgent(agent string) bool {
	switch agent {
	case "claude", "codex", "devin", "gemini":
		return true
	}
	return false
}

// Extension handles extra top-level commands (e.g. "note" from the notes
// stage). It returns ok=false when the command isn't its own.
type Extension func(ctx context.Context, caller string, args []string) (r Result, ok bool)

// Handler returns the http.Handler for POST /cli. callerOf extracts the
// authenticated terminal id from the request.
func (b *Bus) Handler(callerOf func(*http.Request) string, exts ...Extension) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CLIRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil {
			writeResult(w, Result{Code: 400, Body: "squad: invalid request body"})
			return
		}
		caller := callerOf(r)
		ctx := r.Context()
		if req.Safe {
			ctx = WithSafe(ctx)
		}
		res := b.Dispatch(ctx, caller, req.Args, exts...)
		writeResult(w, res)
	})
}

func writeResult(w http.ResponseWriter, res Result) {
	if res.Code == 0 {
		res.Code = 200
	}
	if res.Code == 499 { // client is gone; nothing to write
		res.Code = 409
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if res.Source != "" {
		w.Header().Set("X-Squad-Source", res.Source)
	}
	w.WriteHeader(res.Code)
	body := res.Body
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	_, _ = io.WriteString(w, body)
}

// Usage is printed by `squad help` inside a terminal.
const Usage = `Usage (inside a Squad terminal):
  squad list                         who you are and what is connected to you
  squad ask "Name" "message"         send a message and wait for the reply
  squad ask "Name" --raw "keys"      type keys (escapes: \n Enter, \t, \e ESC, \xNN) and return the screen
  squad ask --batch '{"A":"m1","B":"m2"}'   ask several terminals in parallel (JSON array result)
  squad check "Name"                 read a connected terminal's screen without sending anything
  squad reply <id> "message"         answer request <id> (shown as [de: … · pedido <id>])
  squad note …                       read and write connected notes (squad note help)
  squad portal …                     use connected portals, or create one (squad portal help)
  squad roles                        the roles agents can have (Planejador, Arquiteto, Implementador…)
  squad new "Name" --role impl       managers: create a terminal next to you (squad new --help)
  squad connect / close              managers: connect nodes, remove what you created
  squad floor …                      floors, isolated copies of the repository (squad floor help)

Messages you receive start with [de: <Name> · pedido <id>]. Reply with
squad reply <id> "…", or just finish your turn: your final answer is returned
to the sender. If an ask times out, don't resend it: use squad check.`

// Dispatch runs one CLI command for caller.
func (b *Bus) Dispatch(ctx context.Context, caller string, args []string, exts ...Extension) Result {
	if len(args) == 0 {
		return Result{Code: 400, Body: Usage}
	}
	switch args[0] {
	case "help", "-h", "--help":
		return Result{Code: 200, Body: Usage}
	case "list":
		return b.List(caller)
	case "check":
		if len(args) != 2 {
			return Result{Code: 400, Body: `Usage: squad check "Name"`}
		}
		return b.Check(caller, args[1])
	case "reply":
		if len(args) < 3 {
			return Result{Code: 400, Body: `Usage: squad reply <id> "message"`}
		}
		return b.Reply(caller, args[1], strings.Join(args[2:], " "))
	case "ask":
		rest := args[1:]
		if len(rest) == 2 && rest[0] == "--batch" {
			return b.Batch(ctx, caller, rest[1])
		}
		if len(rest) == 3 && rest[1] == "--raw" {
			if Safe(ctx) {
				return Result{Code: 403, Body: "squad: typing keys into a terminal needs the user's approval: use the keys tool."}
			}
			return b.Raw(caller, rest[0], rest[2])
		}
		if len(rest) < 2 || strings.HasPrefix(rest[0], "--") {
			return Result{Code: 400, Body: "Usage: squad ask \"Name\" \"message\"  |  squad ask \"Name\" --raw \"keys\"  |  squad ask --batch '{json}'"}
		}
		return b.Ask(ctx, caller, rest[0], strings.Join(rest[1:], " "))
	}
	for _, ext := range exts {
		if r, ok := ext(ctx, caller, args); ok {
			return r
		}
	}
	return Result{Code: 400, Body: fmt.Sprintf("squad: unknown command %q\n\n%s", args[0], Usage)}
}

// floorLabel names a floor for squad list ("ground" for the ground floor).
func (b *Bus) floorLabel(id string) string {
	if f, ok := b.store.Floor(id); ok && id != "" {
		return f.Name
	}
	return "ground"
}

// List describes the caller and what is connected to it.
func (b *Bus) List(caller string) Result {
	me, ok := b.store.Node(caller)
	if !ok || me.Terminal == nil {
		return Result{Code: 400, Body: "squad: unknown calling terminal"}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "You:\n  - name: %s, agent: %s", quote(me.Terminal.Name), me.Terminal.Agent)
	if me.Terminal.Role != "" {
		fmt.Fprintf(&sb, ", role: %s", quote(b.roleLabel(me.Terminal.Role)))
	}
	if f, ok := b.store.Floor(me.Floor); ok && me.Floor != "" {
		fmt.Fprintf(&sb, ", floor: %s (branch %s; commit your work there)", quote(f.Name), f.Branch)
	}
	sb.WriteString("\n")
	if b.selfNote != nil {
		if note := b.selfNote(caller); note != "" {
			sb.WriteString("  " + note + "\n")
		}
	}

	neighbors := b.store.Neighbors(caller)
	sort.Slice(neighbors, func(i, j int) bool { return neighbors[i].Label() < neighbors[j].Label() })
	var agents, notes, portals []store.Node
	for _, n := range neighbors {
		switch n.Kind {
		case store.KindTerminal:
			agents = append(agents, n)
		case store.KindNote:
			notes = append(notes, n)
		case store.KindPortal:
			portals = append(portals, n)
		}
	}
	if len(agents)+len(notes)+len(portals) == 0 {
		sb.WriteString("\nNothing is connected to you yet. Ask the user to connect you to other terminals or notes on the canvas")
		if b.selfNote != nil && b.selfNote(caller) != "" {
			sb.WriteString(", or create the terminals you need")
		}
		sb.WriteString(".\n")
		return Result{Code: 200, Body: sb.String()}
	}
	if len(agents) > 0 {
		sb.WriteString("\nConnected terminals (use squad ask / check):\n")
		for _, n := range agents {
			fmt.Fprintf(&sb, "  - name: %s, agent: %s", quote(n.Terminal.Name), n.Terminal.Agent)
			if n.Terminal.Role != "" {
				fmt.Fprintf(&sb, ", role: %s", quote(b.roleLabel(n.Terminal.Role)))
			}
			if n.Floor != me.Floor {
				fmt.Fprintf(&sb, ", floor: %s", quote(b.floorLabel(n.Floor)))
			}
			if b.mon != nil {
				st, _ := b.mon.State(n.ID)
				fmt.Fprintf(&sb, ", state: %s", st)
			}
			sb.WriteString("\n")
		}
	}
	if len(notes) > 0 {
		sb.WriteString("\nConnected notes (use squad note read/write/edit):\n")
		for _, n := range notes {
			fmt.Fprintf(&sb, "  - name: %s", quote(n.Note.Title))
			if n.Note.File != "" {
				fmt.Fprintf(&sb, ", file: %s", n.Note.File)
			}
			if len(n.Note.Aliases) > 0 {
				fmt.Fprintf(&sb, ", formerly: %s", quote(strings.Join(n.Note.Aliases, `", "`)))
			}
			if n.Note.Locked {
				sb.WriteString(", locked: true")
			}
			sb.WriteString("\n")
		}
	}
	if len(portals) > 0 {
		sb.WriteString("\nConnected portals (use squad portal snapshot / click / fill…; squad portal help):\n")
		for _, n := range portals {
			fmt.Fprintf(&sb, "  - name: %s, url: %s\n", quote(n.Label()), n.Portal.URL)
		}
	}
	return Result{Code: 200, Body: sb.String()}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanribeiroo/squad/internal/agents"
	"github.com/nathanribeiroo/squad/internal/version"
)

const mcpInstructions = `Squad: a canvas where agents and terminals are connected by wires; you can only reach what is connected to you.
Start with list. ask sends a message to a connected agent and waits for its reply (its final answer; it can take minutes).
note reads and writes connected notes; portal drives a connected browser page the user watches live.
Managers (list says so) create their team with new and wire nodes with connect.
keys, portal_eval and new_shell can run commands, so they ask the user first.`

// runMCP serves Squad's commands as MCP tools over stdio (squad mcp). The
// agent starts it inside a Squad terminal; each call goes to the Squad server
// as that terminal, like the CLI.
func runMCP(_ []string, _, stderr io.Writer) int {
	env, err := readClientEnv()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "squad", Version: version.Version}, &mcp.ServerOptions{Instructions: mcpInstructions})
	addMCPTools(s, env)
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(stderr, "squad mcp:", err)
		return 1
	}
	return 0
}

// addTool registers a tool that runs the CLI command build returns. A
// blocking tool waits as long as the call takes (ask). Calls of the tools in
// agents.SafeMCPTools are marked safe: the server refuses anything in them
// that would run a command.
func addTool[In any](s *mcp.Server, env clientEnv, name, desc string, blocking bool, build func(In) ([]string, error)) {
	safe := slices.Contains(agents.SafeMCPTools, name)
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		args, err := build(in)
		if err != nil {
			return textResult(err.Error(), true), nil, nil
		}
		timeout := 60 * time.Second
		if blocking {
			timeout = 0
		}
		code, out, err := callServer(ctx, env, args, safe, timeout)
		if err != nil {
			return textResult("can't reach the Squad server: "+err.Error(), true), nil, nil
		}
		return textResult(strings.TrimRight(string(out), "\n"), code >= 300), nil, nil
	})
}

func textResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: isError}
}

// name checks a name-like argument: required, and never an option ("--…"),
// so a value can't change what the command does.
func name(field, v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return "", fmt.Errorf("%s is required", field)
	case strings.HasPrefix(v, "-"):
		return "", fmt.Errorf("%s can't start with -", field)
	}
	return v, nil
}

// optional adds --flag value when v isn't empty.
func optional(args []string, flag, field, v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return args, nil
	}
	v, err := name(field, v)
	if err != nil {
		return nil, err
	}
	return append(args, flag, v), nil
}

type (
	noInput   struct{}
	nameInput struct {
		Name string `json:"name" jsonschema:"terminal name, exactly as list prints it"`
	}
	askInput struct {
		Name    string `json:"name" jsonschema:"agent name, exactly as list prints it"`
		Message string `json:"message" jsonschema:"what to ask or tell it"`
	}
	batchInput struct {
		Requests map[string]string `json:"requests" jsonschema:"agent name → message, all sent at once"`
	}
	replyInput struct {
		ID      string `json:"id" jsonschema:"request id, from [de: Name · pedido <id>]"`
		Message string `json:"message" jsonschema:"your answer"`
	}
	noteInput struct {
		Action  string `json:"action" jsonschema:"list | read | write | edit | create"`
		Name    string `json:"name,omitempty" jsonschema:"note title (all but list)"`
		Content string `json:"content,omitempty" jsonschema:"write: the whole new content; create: the initial content"`
		Old     string `json:"old,omitempty" jsonschema:"edit: text that appears exactly once"`
		New     string `json:"new,omitempty" jsonschema:"edit: its replacement"`
		From    int    `json:"from,omitempty" jsonschema:"read: first line (1-based)"`
		Count   int    `json:"count,omitempty" jsonschema:"read: number of lines"`
	}
	portalInput struct {
		Action    string   `json:"action" jsonschema:"list | snapshot | click | fill | type | key | scroll | goto | back | forward | reload | screenshot | console | create | close"`
		Portal    string   `json:"portal,omitempty" jsonschema:"portal name, when several are connected to you"`
		Ref       string   `json:"ref,omitempty" jsonschema:"click/fill: element ref from the last snapshot, like @e3"`
		X         *float64 `json:"x,omitempty" jsonschema:"click: x in page px, instead of ref"`
		Y         *float64 `json:"y,omitempty" jsonschema:"click: y in page px, instead of ref"`
		Text      string   `json:"text,omitempty" jsonschema:"fill/type: the text"`
		Key       string   `json:"key,omitempty" jsonschema:"key: Enter, Escape, Tab, Control+a…"`
		URL       string   `json:"url,omitempty" jsonschema:"goto/create: the address"`
		Direction string   `json:"direction,omitempty" jsonschema:"scroll: down (default) or up"`
		Pixels    int      `json:"pixels,omitempty" jsonschema:"scroll: how far (default 600)"`
		Count     int      `json:"count,omitempty" jsonschema:"console: how many messages"`
		Name      string   `json:"name,omitempty" jsonschema:"create: the new portal's name; close: the portal to close"`
	}
	newInput struct {
		Name  string `json:"name" jsonschema:"the new terminal's name"`
		Role  string `json:"role,omitempty" jsonschema:"role id or name: plan, impl, rev, test or another role"`
		Agent string `json:"agent,omitempty" jsonschema:"claude, codex, devin or gemini (default: yours)"`
		Cwd   string `json:"cwd,omitempty" jsonschema:"folder, relative to the workspace (default: yours)"`
	}
	connectInput struct {
		A string `json:"a" jsonschema:"a node name (terminal, portal or note)"`
		B string `json:"b,omitempty" jsonschema:"another node name; without it, a is connected to you"`
	}
	keysInput struct {
		Name string `json:"name" jsonschema:"terminal name"`
		Keys string `json:"keys" jsonschema:"keys to type; escapes: \\n Enter, \\t Tab, \\e ESC, \\xNN a byte (\\x03 is Ctrl-C)"`
	}
	evalInput struct {
		Portal string `json:"portal,omitempty" jsonschema:"portal name, when several are connected to you"`
		JS     string `json:"js" jsonschema:"JavaScript expression; its value is returned (10 s limit)"`
	}
	newShellInput struct {
		Name    string `json:"name" jsonschema:"the new terminal's name"`
		Command string `json:"command" jsonschema:"command line to run in it, e.g. npm run dev"`
		Agent   string `json:"agent,omitempty" jsonschema:"shell (default) or custom (a program that is not an AI agent)"`
		Cwd     string `json:"cwd,omitempty" jsonschema:"folder, relative to the workspace (default: yours)"`
	}
)

func addMCPTools(s *mcp.Server, env clientEnv) {
	addTool(s, env, "list", "Who you are and what is connected to you: agents (role, state), notes and portals. Run it first and use names exactly as printed.",
		false, func(noInput) ([]string, error) { return []string{"list"}, nil })

	addTool(s, env, "ask", "Send a message to a connected agent and wait for its reply (its final answer for that turn; it can take minutes). Agents only: for a shell, use keys. If it times out, don't resend: use check.",
		true, func(in askInput) ([]string, error) {
			n, err := name("name", in.Name)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(in.Message) == "" {
				return nil, errors.New("message is required")
			}
			return []string{"ask", n, in.Message}, nil
		})

	addTool(s, env, "ask_batch", "Ask several connected agents at once and wait for all replies (JSON array of {name, output|error}).",
		true, func(in batchInput) ([]string, error) {
			if len(in.Requests) == 0 {
				return nil, errors.New("requests is required")
			}
			for n := range in.Requests {
				if _, err := name("agent name", n); err != nil {
					return nil, err
				}
			}
			b, _ := json.Marshal(in.Requests)
			return []string{"ask", "--batch", string(b)}, nil
		})

	addTool(s, env, "check", "Read a connected terminal's current screen without sending anything.",
		false, func(in nameInput) ([]string, error) {
			n, err := name("name", in.Name)
			return []string{"check", n}, err
		})

	addTool(s, env, "reply", "Answer a request you received ([de: Name · pedido <id>]) before your turn ends. Finishing your turn with a clear answer also replies.",
		false, func(in replyInput) ([]string, error) {
			id, err := name("id", in.ID)
			if err != nil {
				return nil, err
			}
			return []string{"reply", id, in.Message}, nil
		})

	addTool(s, env, "note", "Read and write notes connected to you (Markdown): list, read (with line numbers), write (replace all), edit (replace text that appears once), create (a new note connected to you).",
		false, func(in noteInput) ([]string, error) {
			if in.Action == "list" {
				return []string{"note", "list"}, nil
			}
			n, err := name("name", in.Name)
			if err != nil {
				return nil, err
			}
			switch in.Action {
			case "read":
				args := []string{"note", "read", n}
				if in.From > 0 || in.Count > 0 {
					args = append(args, strconv.Itoa(max(in.From, 1)))
				}
				if in.Count > 0 {
					args = append(args, strconv.Itoa(in.Count))
				}
				return args, nil
			case "write":
				return []string{"note", "write", n, in.Content}, nil
			case "edit":
				return []string{"note", "edit", n, in.Old, in.New}, nil
			case "create":
				if in.Content == "" {
					return []string{"note", "create", n}, nil
				}
				return []string{"note", "create", n, in.Content}, nil
			}
			return nil, fmt.Errorf("unknown action %q", in.Action)
		})

	addTool(s, env, "portal", "Drive a browser page connected to you, which the user watches live: snapshot (elements with refs like @e3), click/fill/type/key/scroll, goto/back/forward/reload, screenshot (a PNG path), console (errors and logs), list; create opens a new portal connected to you, close removes one you created. Take a new snapshot after the page changes.",
		false, buildPortal)

	addTool(s, env, "new", "Managers: create an agent terminal next to you, connected to you, with a role; returns once the agent is ready for your first ask.",
		false, func(in newInput) ([]string, error) {
			n, err := name("name", in.Name)
			if err != nil {
				return nil, err
			}
			args := []string{"new", n}
			for _, f := range [][3]string{{"--role", "role", in.Role}, {"--agent", "agent", in.Agent}, {"--cwd", "cwd", in.Cwd}} {
				if args, err = optional(args, f[0], f[1], f[2]); err != nil {
					return nil, err
				}
			}
			return args, nil
		})

	addTool(s, env, "connect", "Managers: connect two nodes of the canvas (terminals, portals, notes), or one to you.",
		false, func(in connectInput) ([]string, error) {
			a, err := name("a", in.A)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(in.B) == "" {
				return []string{"connect", a}, nil
			}
			b, err := name("b", in.B)
			return []string{"connect", a, b}, err
		})

	addTool(s, env, "close", "Managers: stop and remove a terminal (or portal) you created.",
		false, func(in nameInput) ([]string, error) {
			n, err := name("name", in.Name)
			return []string{"close", n}, err
		})

	// These can run commands: the user approves each call.
	addTool(s, env, "keys", "Type keys into a connected terminal (a shell, a menu) and get its screen back. It can run commands, so the user approves it.",
		false, func(in keysInput) ([]string, error) {
			n, err := name("name", in.Name)
			if err != nil {
				return nil, err
			}
			if in.Keys == "" {
				return nil, errors.New("keys is required")
			}
			return []string{"ask", n, "--raw", in.Keys}, nil
		})

	addTool(s, env, "portal_eval", "Run JavaScript in a connected portal's page and return its value. The user approves it.",
		false, func(in evalInput) ([]string, error) {
			args := []string{"portal"}
			if strings.TrimSpace(in.Portal) != "" {
				p, err := name("portal", in.Portal)
				if err != nil {
					return nil, err
				}
				args = append(args, p)
			}
			if strings.TrimSpace(in.JS) == "" {
				return nil, errors.New("js is required")
			}
			return append(args, "eval", in.JS), nil
		})

	addTool(s, env, "new_shell", "Managers: create a terminal that runs a command line (a dev server, a watcher) next to you, connected to you. The user approves it.",
		false, func(in newShellInput) ([]string, error) {
			n, err := name("name", in.Name)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(in.Command) == "" {
				return nil, errors.New("command is required")
			}
			agent := in.Agent
			if agent == "" {
				agent = "shell"
			}
			if agent != "shell" && agent != "custom" {
				return nil, errors.New("agent is shell or custom (use new for AI agents)")
			}
			args := []string{"new", n, "--agent", agent, "--command", in.Command}
			return optional(args, "--cwd", "cwd", in.Cwd)
		})
}

// buildPortal turns the portal tool's input into squad portal arguments.
func buildPortal(in portalInput) ([]string, error) {
	switch in.Action {
	case "list":
		return []string{"portal", "list"}, nil
	case "create":
		url, err := name("url", in.URL)
		if err != nil {
			return nil, err
		}
		args := []string{"portal", "create", url}
		if strings.TrimSpace(in.Name) != "" {
			n, err := name("name", in.Name)
			if err != nil {
				return nil, err
			}
			args = append(args, n)
		}
		return args, nil
	case "close":
		if strings.TrimSpace(in.Name) == "" {
			return []string{"portal", "close"}, nil
		}
		n, err := name("name", in.Name)
		return []string{"portal", "close", n}, err
	}
	args := []string{"portal"}
	if strings.TrimSpace(in.Portal) != "" {
		p, err := name("portal", in.Portal)
		if err != nil {
			return nil, err
		}
		args = append(args, p)
	}
	args = append(args, in.Action)
	need := func(field, v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s needs %s", in.Action, field)
		}
		return nil
	}
	switch in.Action {
	case "snapshot", "screenshot", "back", "forward", "reload":
	case "click":
		if in.Ref == "" && in.X != nil && in.Y != nil {
			return append(args, strconv.FormatFloat(*in.X, 'f', -1, 64), strconv.FormatFloat(*in.Y, 'f', -1, 64)), nil
		}
		ref, err := name("ref", in.Ref)
		return append(args, ref), err
	case "fill":
		ref, err := name("ref", in.Ref)
		if err != nil {
			return nil, err
		}
		return append(args, ref, in.Text), nil
	case "type":
		return append(args, in.Text), need("text", in.Text)
	case "key":
		k, err := name("key", in.Key)
		return append(args, k), err
	case "scroll":
		if in.Direction == "up" {
			args = append(args, "up")
		}
		if in.Pixels > 0 {
			args = append(args, strconv.Itoa(in.Pixels))
		}
	case "goto":
		u, err := name("url", in.URL)
		return append(args, u), err
	case "console":
		if in.Count > 0 {
			args = append(args, strconv.Itoa(in.Count))
		}
	default:
		return nil, fmt.Errorf("unknown action %q (portal_eval runs JavaScript)", in.Action)
	}
	return args, nil
}

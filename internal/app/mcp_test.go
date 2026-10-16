//go:build !windows

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nathanribeiroo/squad/internal/store"
)

// mcpSession starts `squad mcp` as terminal n's agent would.
func (e *env) mcpSession(n store.Node) *mcp.ClientSession {
	e.t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "squad"), "mcp")
	cmd.Env = append(os.Environ(), "SQUAD_URL="+e.app.Server.URL(), "SQUAD_TERMINAL_ID="+n.ID, "SQUAD_TOKEN="+e.app.Server.TerminalToken(n.ID))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = s.Close() })
	return s
}

// call runs a tool and returns its text and whether it failed.
func call(t *testing.T, s *mcp.ClientSession, tool string, args map[string]any) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	var sb strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), r.IsError
}

func TestMCPTools(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agentOfType("Plano", "gemini") // an AI agent type, so it may be asked
	e.setRole(plan, "plan")
	bia := e.agentOfType("Bia", "gemini")
	sh := e.agentOfType("Shell", "shell")
	e.connect(plan, bia)
	e.connect(plan, sh)
	s := e.mcpSession(plan)

	tools, err := s.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 15 {
		t.Fatalf("tools: %v %d", err, len(tools.Tools))
	}

	if out, bad := call(t, s, "list", nil); bad || !strings.Contains(out, `name: "Bia"`) || !strings.Contains(out, "You are a manager") {
		t.Fatalf("list: %v %s", bad, out)
	}
	// ask waits for the agent's reply.
	if out, bad := call(t, s, "ask", map[string]any{"name": "Bia", "message": "implemente o CSV"}); bad || !strings.Contains(out, "implemente o CSV") {
		t.Fatalf("ask: %v %s", bad, out)
	}
	if out, bad := call(t, s, "ask_batch", map[string]any{"requests": map[string]string{"Bia": "e o teste?"}}); bad || !strings.Contains(out, `"output"`) {
		t.Fatalf("ask_batch: %v %s", bad, out)
	}

	// The safe tools never run a command: a message to a shell, keys,
	// a terminal with a command line and JavaScript are refused…
	for _, c := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"ask", map[string]any{"name": "Shell", "message": "rm -rf ~"}, "not an agent"},
		{"ask_batch", map[string]any{"requests": map[string]string{"Shell": "rm -rf ~"}}, "not an agent"},
		{"ask", map[string]any{"name": "--raw", "message": "x"}, "can't start with -"},
		{"new", map[string]any{"name": "X", "agent": "shell"}, "new_shell"},
		{"new", map[string]any{"name": "--command=rm", "agent": "claude"}, "can't start with -"},
		{"portal", map[string]any{"action": "eval"}, "unknown action"},
		{"floors", map[string]any{"action": "land", "name": "X"}, "floor_land"},
	} {
		if out, bad := call(t, s, c.tool, c.args); !strings.Contains(out, c.want) || (c.tool != "ask_batch" && !bad) {
			t.Fatalf("%s %v: %v %s", c.tool, c.args, bad, out)
		}
	}
	// …and the shell still got nothing.
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(e.app.Server.Terminals().Get(sh.ID).Snapshot(), "rm -rf") {
		t.Fatal("the shell received the message")
	}

	// The tools the user approves can.
	if out, bad := call(t, s, "keys", map[string]any{"name": "Shell", "keys": `echo via-keys\n`}); bad || !strings.Contains(out, "via-keys") {
		t.Fatalf("keys: %v %s", bad, out)
	}
	if out, bad := call(t, s, "new_shell", map[string]any{"name": "Dev", "command": "echo servidor"}); bad || !strings.Contains(out, `Created "Dev" (shell)`) {
		t.Fatalf("new_shell: %v %s", bad, out)
	}
	if out, bad := call(t, s, "portal_eval", map[string]any{"js": "1+1"}); !bad || strings.Contains(out, "approval") {
		t.Fatalf("portal_eval is not refused as unsafe (no portal here): %v %s", bad, out)
	}

	// Notes and the team tools.
	if out, bad := call(t, s, "note", map[string]any{"action": "create", "name": "Plano de testes", "content": "- [ ] CSV"}); bad {
		t.Fatalf("note create: %s", out)
	}
	if out, bad := call(t, s, "note", map[string]any{"action": "read", "name": "plano de TESTES"}); bad || !strings.Contains(out, "CSV") {
		t.Fatalf("note read (any case): %v %s", bad, out)
	}
	if out, bad := call(t, s, "note", map[string]any{"action": "create", "name": "Plano de Testes"}); !bad || !strings.Contains(out, "already a note named") {
		t.Fatalf("a second note with the same name: %v %s", bad, out)
	}
	if out, bad := call(t, s, "connect", map[string]any{"a": "Bia", "b": "Plano de testes"}); bad {
		t.Fatalf("connect: %s", out)
	}
	if out, bad := call(t, s, "close", map[string]any{"name": "Dev"}); bad || !strings.Contains(out, `Closed "Dev"`) {
		t.Fatalf("close: %v %s", bad, out)
	}

	// Floors (the workspace folder here isn't a repository).
	if out, bad := call(t, s, "floors", map[string]any{"action": "list"}); bad || !strings.Contains(out, "No floors yet") {
		t.Fatalf("floors list: %v %s", bad, out)
	}
	if out, bad := call(t, s, "floors", map[string]any{"action": "create", "name": "Login"}); !bad || !strings.Contains(out, "not in a git repository") {
		t.Fatalf("floors create: %v %s", bad, out)
	}
	if out, bad := call(t, s, "floor_land", map[string]any{"name": "Login", "merge": true}); !bad || !strings.Contains(out, `no floor named "Login"`) {
		t.Fatalf("floor_land is refused as unsafe: %v %s", bad, out)
	}
}

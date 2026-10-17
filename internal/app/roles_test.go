//go:build !windows

package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanribeiroo/squad/internal/store"
)

func (e *env) setRole(n store.Node, role string) {
	e.t.Helper()
	if _, err := e.app.Server.Store().UpdateNode(n.ID, func(x *store.Node) error { x.Terminal.Role = role; return nil }); err != nil {
		e.t.Fatal(err)
	}
}

// hook runs `torre hook <name>` as a terminal's agent would.
func (e *env) hook(n store.Node, name, payload string) string {
	e.t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "torre"), "hook", name, payload)
	cmd.Env = append(os.Environ(), "SQUAD_URL="+e.app.Server.URL(), "SQUAD_TERMINAL_ID="+n.ID, "SQUAD_TOKEN="+e.app.Server.TerminalToken(n.ID))
	out, err := cmd.Output()
	if err != nil {
		e.t.Fatalf("torre hook %s: %v", name, err)
	}
	return string(out)
}

func TestRolesReachAgents(t *testing.T) {
	e := newEnv(t, fastLimits)
	ana := e.agent("Ana")                // a custom command: no instructions
	bia := e.agentOfType("Bia", "devin") // Devin asks for them at session start
	e.setRole(ana, "test")
	e.setRole(bia, "rev")
	e.connect(ana, bia)

	// torre list names the roles.
	r := e.run(ana, "list")
	if !strings.Contains(r.Body, `name: "Ana", agent: custom, role: "Testador"`) || !strings.Contains(r.Body, `name: "Bia", agent: devin, role: "Revisor"`) {
		t.Fatalf("list:\n%s", r.Body)
	}

	// Devin's SessionStart hook prints the role as context.
	var out struct {
		HookSpecificOutput struct{ HookEventName, AdditionalContext string }
	}
	got := e.hook(bia, "devin-session-start", `{"hook_event_name":"SessionStart","source":"startup"}`)
	if json.Unmarshal([]byte(got), &out) != nil || out.HookSpecificOutput.HookEventName != "SessionStart" ||
		!strings.Contains(out.HookSpecificOutput.AdditionalContext, `terminal "Bia" da Torre, com o papel "Revisor"`) ||
		!strings.Contains(out.HookSpecificOutput.AdditionalContext, "Você revisa mudanças de código") {
		t.Fatalf("devin session start:\n%s", got)
	}

	// An edited role reaches the next session.
	rev, _ := e.roles.Get("rev")
	rev.Prompt = "Revise só a segurança."
	if _, err := e.roles.Update("rev", rev); err != nil {
		t.Fatal(err)
	}
	if got := e.hook(bia, "devin-session-start", `{}`); !strings.Contains(got, "Revise só a segurança.") {
		t.Fatalf("edited role:\n%s", got)
	}

	// Custom commands and terminals without a role get nothing.
	if got := e.hook(ana, "devin-session-start", `{}`); got != "" {
		t.Fatalf("custom agent: %q", got)
	}
	e.setRole(bia, "")
	if got := e.hook(bia, "devin-session-start", `{}`); got != "" {
		t.Fatalf("no role: %q", got)
	}
}

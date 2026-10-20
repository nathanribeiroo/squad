//go:build !windows

package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nathanribeiroo/squad/internal/store"
)

// fakeCommand is a launch line for the fake agent, for torre new --command.
func (e *env) fakeCommand(name string) string {
	return "FAKEAGENT_LOG=" + filepath.Join(e.dir, name+".log") + " " + filepath.Join(binDir, "fakeagent")
}

func (e *env) node(name string) (store.Node, bool) {
	for _, n := range e.app.Server.Store().Snapshot().Nodes {
		if n.Label() == name {
			return n, true
		}
	}
	return store.Node{}, false
}

func TestManagerBuildsTeam(t *testing.T) {
	limits := fastLimits
	limits.MaxTeam = 2
	e := newEnv(t, limits)
	plan := e.agent("Plano")
	e.setRole(plan, "plan") // the Planejador is a manager
	user := e.agent("Usuário")

	if r := e.run(plan, "list"); !strings.Contains(r.Body, "You are a manager") || !strings.Contains(r.Body, "0 of 2 created") {
		t.Fatalf("list:\n%s", r.Body)
	}

	// torre new: next to the manager, connected, with the role, started and
	// ready (it waits for the agent's screen to settle).
	start := time.Now()
	r := e.run(plan, "new", "Bia", "--role", "Implementador", "--agent=gemini", "--command", e.fakeCommand("Bia"))
	if r.Code != 200 || !strings.Contains(r.Body, `Created "Bia" (gemini, role "Implementador"), connected to you and ready`) {
		t.Fatalf("new: %d %s", r.Code, r.Body)
	}
	t.Logf("ready after %v", time.Since(start))
	bia, ok := e.node("Bia")
	if !ok || bia.CreatedBy != plan.ID || bia.Terminal.Role != "impl" || bia.Terminal.Manager == nil || *bia.Terminal.Manager {
		t.Fatalf("Bia: %+v %+v", bia, bia.Terminal)
	}
	if !e.app.Server.Store().Connected(plan.ID, bia.ID) {
		t.Fatal("Bia is not connected to the manager")
	}
	me, _ := e.app.Server.Store().Node(plan.ID)
	if bia.X < me.X+me.W || bia.Y != me.Y {
		t.Fatalf("Bia at %v,%v; manager at %v,%v %vx%v", bia.X, bia.Y, me.X, me.Y, me.W, me.H)
	}
	if st, _ := e.app.Server.Terminals().Get(bia.ID).State(); st != "running" {
		t.Fatalf("Bia not running: %s", st)
	}
	// The first message reaches the agent, not the shell it was typed in.
	if r := e.run(plan, "ask", "Bia", "olá"); r.Code != 200 || !strings.Contains(r.Body, "olá") {
		t.Fatalf("ask Bia: %d %s", r.Code, r.Body)
	}

	// Mistakes and limits.
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"new", "Bia", "--agent", "custom", "--command", "true"}, 409, "already a terminal named"},
		{[]string{"new", "X", "--role", "nope"}, 400, "unknown role"},
		{[]string{"new", "X", "--agent", "vim"}, 400, "unknown agent"},
		{[]string{"new", "X", "--agent", "custom"}, 400, "needs --command"},
		{[]string{"new", "--role", "impl"}, 400, "a name is needed"},
	} {
		if r := e.run(plan, c.args...); r.Code != c.code || !strings.Contains(r.Body, c.want) {
			t.Fatalf("%v: %d %s", c.args, r.Code, r.Body)
		}
	}
	if r := e.run(plan, "new", "Caio", "--agent", "shell", "--command", "echo servidor"); r.Code != 200 || !strings.Contains(r.Body, `Created "Caio" (shell)`) {
		t.Fatalf("shell: %d %s", r.Code, r.Body)
	}
	if r := e.run(plan, "new", "Dani", "--agent", "custom", "--command", "true"); r.Code != 409 || !strings.Contains(r.Body, "the limit") {
		t.Fatalf("limit: %d %s", r.Code, r.Body)
	}

	// Agents it created are not managers.
	if r := e.run(bia, "new", "Eva"); r.Code != 403 || !strings.Contains(r.Body, "manager mode") {
		t.Fatalf("Bia creating: %d %s", r.Code, r.Body)
	}
	// Anyone sees the roles; the manager's help lists them too.
	r = e.run(bia, "roles")
	if r.Code != 200 || !strings.Contains(r.Body, "- plan: Planejador (manager) — Você planeja") || !strings.Contains(r.Body, "- spec: Especificador — Você transforma") ||
		!strings.Contains(r.Body, "\n  call it: Feature que precisa de spec técnica") {
		t.Fatalf("roles: %d %s", r.Code, r.Body)
	}
	if r := e.run(plan, "new", "--help"); !strings.Contains(r.Body, "Roles: an id or a name") || !strings.Contains(r.Body, "- docs: Documentador") {
		t.Fatalf("new --help: %s", r.Body)
	}

	// connect and close.
	if r := e.run(plan, "connect", "Bia", "Caio"); r.Code != 200 {
		t.Fatalf("connect: %d %s", r.Code, r.Body)
	}
	caio, _ := e.node("Caio")
	if !e.app.Server.Store().Connected(bia.ID, caio.ID) {
		t.Fatal("Bia and Caio not connected")
	}
	if r := e.run(plan, "connect", "bia", "Caio"); r.Code != 200 || !strings.Contains(r.Body, "already connected") {
		t.Fatalf("connect again: %d %s", r.Code, r.Body)
	}
	if r := e.run(plan, "close", "Usuário"); r.Code != 403 {
		t.Fatalf("close the user's terminal: %d %s", r.Code, r.Body)
	}
	if r := e.run(plan, "close", "Caio"); r.Code != 200 {
		t.Fatalf("close: %d %s", r.Code, r.Body)
	}
	if _, ok := e.node("Caio"); ok || e.app.Server.Terminals().Get(caio.ID) != nil {
		t.Fatal("Caio still there")
	}

	// Manager mode can be turned off on the terminal.
	off := false
	if _, err := e.app.Server.Store().UpdateNode(plan.ID, func(n *store.Node) error { n.Terminal.Manager = &off; return nil }); err != nil {
		t.Fatal(err)
	}
	if r := e.run(plan, "new", "Fabi"); r.Code != 403 {
		t.Fatalf("manager mode off: %d %s", r.Code, r.Body)
	}
	_ = user
}

// Roles hand their work over in notes: notes from templates, a stack of
// tasks an agent makes, and the notes of a dismissed flight staying with its
// manager, for the flight that comes back.
func TestSpecNotesAndDismissal(t *testing.T) {
	e := newEnv(t, fastLimits)
	plan := e.agent("Plano")
	e.setRole(plan, "plan")
	if r := e.run(plan, "new", "Esp", "--role", "spec", "--agent=gemini", "--command", e.fakeCommand("Esp")); r.Code != 200 {
		t.Fatalf("new: %d %s", r.Code, r.Body)
	}
	esp, _ := e.node("Esp")
	read := func(caller store.Node, name string) string {
		t.Helper()
		r := e.run(caller, "note", "read", name)
		if r.Code != 200 {
			t.Fatalf("read %s: %d %s", name, r.Code, r.Body)
		}
		return r.Body
	}

	// Notes from templates; an ADR is dated today.
	if r := e.run(esp, "note", "create", "Spec: Filtro", "--template", "spec"); r.Code != 200 || !strings.Contains(read(esp, "Spec: Filtro"), "## Ordem de construção") {
		t.Fatalf("spec: %d %s", r.Code, r.Body)
	}
	if r := e.run(esp, "note", "create", "ADR-001: Filtro no cliente", "--template=adr"); r.Code != 200 || !strings.Contains(read(esp, "ADR-001: Filtro no cliente"), time.Now().Format("2006-01-02")) {
		t.Fatalf("adr: %d %s", r.Code, r.Body)
	}
	for _, args := range [][]string{
		{"X", "conteúdo", "--template", "prd"}, // content or a template
		{"X", "--template", "nope"},
		{"X", "--template"},
		{"X", "--outra", "y"},
	} {
		if r := e.run(esp, append([]string{"note", "create"}, args...)...); r.Code != 400 {
			t.Fatalf("%v: %d %s", args, r.Code, r.Body)
		}
	}
	if r := e.run(esp, "note", "create", "X", "--template", "nope"); !strings.Contains(r.Body, "prd, spec, adr, tasks, task, design, memory") {
		t.Fatalf("the templates: %s", r.Body)
	}

	// A stack of tasks: any of its notes, in any case, names it.
	if r := e.run(esp, "note", "create", "Tarefas: Filtro", "--template", "tasks"); r.Code != 200 {
		t.Fatalf("tasks: %d %s", r.Code, r.Body)
	}
	if r := e.run(esp, "note", "create", "Tarefa 01: Campo", "--template", "task", "--stack", "Tarefas: Filtro"); r.Code != 200 || !strings.Contains(r.Body, `on top of the stack "Tarefas: Filtro"`) {
		t.Fatalf("task 1: %d %s", r.Code, r.Body)
	}
	if r := e.run(esp, "note", "create", "Tarefa 02: Lista", "--template", "task", "--stack=tarefa 01: campo"); r.Code != 200 || !strings.Contains(r.Body, `on top of the stack "Tarefas: Filtro"`) {
		t.Fatalf("task 2: %d %s", r.Code, r.Body)
	}
	base, _ := e.node("Tarefas: Filtro")
	var order []string
	for _, n := range e.app.Server.Store().StackMembers(base.ID) {
		order = append(order, n.Note.Title)
	}
	if strings.Join(order, " | ") != "Tarefas: Filtro | Tarefa 01: Campo | Tarefa 02: Lista" {
		t.Fatalf("stack: %v", order)
	}
	if task, _ := e.node("Tarefa 02: Lista"); e.app.Server.Store().Connected(esp.ID, task.ID) || !e.app.Server.Store().Connected(esp.ID, base.ID) {
		t.Fatal("a stack's wires are its base's")
	}
	if r := e.run(esp, "note", "list"); strings.Count(r.Body, `stack: "Tarefas: Filtro"`) != 3 {
		t.Fatalf("note list:\n%s", r.Body)
	}
	// Not onto a note that isn't connected: and nothing is left behind.
	if _, err := e.app.Notes.Create(plan.ID, "Plano geral", "", "Plano"); err != nil {
		t.Fatal(err)
	}
	if r := e.run(esp, "note", "create", "Y", "--stack", "Plano geral"); r.Code != 404 {
		t.Fatalf("stack onto a note not connected: %d %s", r.Code, r.Body)
	}
	if _, ok := e.node("Y"); ok {
		t.Fatal("a refused stack left a note")
	}

	// Dismissed: its notes stay, connected to the manager; the stack by its base.
	r := e.run(plan, "close", "Esp")
	if r.Code != 200 || !strings.Contains(r.Body, `"Spec: Filtro"`) || !strings.Contains(r.Body, `"Tarefa 02: Lista"`) || strings.Contains(r.Body, "Plano geral") {
		t.Fatalf("close: %d %s", r.Code, r.Body)
	}
	spec, _ := e.node("Spec: Filtro")
	if !e.app.Server.Store().Connected(plan.ID, spec.ID) || !e.app.Server.Store().Connected(plan.ID, base.ID) {
		t.Fatal("the dismissed flight's notes are not connected to the manager")
	}
	if !strings.Contains(read(plan, "Tarefa 01: Campo"), "## Subtarefas") {
		t.Fatal("the manager can't read the tasks")
	}

	// Called back with the same name: the manager connects its notes again.
	if r := e.run(plan, "new", "Esp", "--role", "spec", "--agent=gemini", "--command", e.fakeCommand("Esp2")); r.Code != 200 {
		t.Fatalf("new again: %d %s", r.Code, r.Body)
	}
	if r := e.run(plan, "connect", "Esp", "Spec: Filtro"); r.Code != 200 {
		t.Fatalf("connect: %d %s", r.Code, r.Body)
	}
	esp2, _ := e.node("Esp")
	if esp2.ID == esp.ID || !strings.Contains(read(esp2, "spec: filtro"), "## Resumo") {
		t.Fatal("the flight called back doesn't read its spec")
	}
}

func TestAgentsCreatePortals(t *testing.T) {
	e := newEnv(t, fastLimits)
	ana := e.agent("Ana") // any agent, not only managers
	bia := e.agent("Bia")

	r := e.run(ana, "portal", "create", "localhost:5173", "App")
	if r.Code != 200 || !strings.Contains(r.Body, `Created portal "App" at http://localhost:5173`) {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	app, ok := e.node("App")
	if !ok || app.CreatedBy != ana.ID || !e.app.Server.Store().Connected(ana.ID, app.ID) {
		t.Fatalf("portal: %+v", app)
	}
	if r := e.run(ana, "portal", "list"); !strings.Contains(r.Body, `name: "App"`) {
		t.Fatalf("list: %s", r.Body)
	}
	if r := e.run(bia, "portal", "create", "example.com", "app"); r.Code != 409 {
		t.Fatalf("same name: %d %s", r.Code, r.Body)
	}
	// Unnamed portals get a free name; three per agent at most.
	for i := 0; i < 2; i++ {
		if r := e.run(ana, "portal", "create", "example.com"); r.Code != 200 || !strings.Contains(r.Body, `"Portal`) {
			t.Fatalf("create %d: %d %s", i, r.Code, r.Body)
		}
	}
	if r := e.run(ana, "portal", "create", "example.com"); r.Code != 409 {
		t.Fatalf("limit: %d %s", r.Code, r.Body)
	}

	// Only the creator closes a portal.
	if r := e.run(bia, "portal", "close", "App"); r.Code != 404 {
		t.Fatalf("close someone else's: %d %s", r.Code, r.Body)
	}
	if r := e.run(ana, "portal", "close"); r.Code != 400 || !strings.Contains(r.Body, "name one") {
		t.Fatalf("close which: %d %s", r.Code, r.Body)
	}
	if r := e.run(ana, "portal", "close", "app"); r.Code != 200 {
		t.Fatalf("close: %d %s", r.Code, r.Body)
	}
	if _, ok := e.node("App"); ok {
		t.Fatal("App still there")
	}
}

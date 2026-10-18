// Package team lets a manager agent build its team on the canvas (docs/PLAN.md
// R2): torre new creates a terminal next to it, connected and started, and
// returns once the new agent is ready; torre connect wires two nodes; torre
// close removes what it created. Only managers may: terminals whose role is a
// manager one (the Planejador), unless the terminal turns it off. Terminals a
// manager creates are not managers themselves, so a team can't grow by itself.
package team

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nathanribeiroo/squad/internal/activity"
	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/roles"
	"github.com/nathanribeiroo/squad/internal/store"
	"github.com/nathanribeiroo/squad/internal/terminal"
)

// Canvas changes the canvas the way the user does, with its events.
type Canvas interface {
	Store() *store.Store
	Terminals() *terminal.Manager
	AddNode(n store.Node) (store.Node, error)
	AddEdge(a, b string, rope [][2]float64) (store.Edge, error)
	RemoveNode(id string) error
}

// Monitor tells when a new agent is ready for its first message.
type Monitor interface {
	State(id string) (activity.State, time.Time)
	Working(id string) bool
	StableFor(id string) time.Duration
}

// Options configure the service; zero values take the defaults.
type Options struct {
	Canvas  Canvas
	Monitor Monitor
	Roles   *roles.Library // nil: no role makes a manager
	MaxTeam int            // terminals a manager may create (6)
	// ReadyTimeout bounds the wait for a new agent (45 s: within the CLI's
	// 60 s); Settle is how long its screen stays still to count as ready (2 s).
	ReadyTimeout time.Duration
	Settle       time.Duration
}

// Service runs the manager commands of one workspace.
type Service struct {
	o       Options
	maxTeam atomic.Int64 // Options.MaxTeam, changed by SetMaxTeam
}

// SetMaxTeam changes how many terminals a manager may create.
func (s *Service) SetMaxTeam(n int) { s.maxTeam.Store(int64(n)) }

// MaxTeam is how many terminals a manager may create.
func (s *Service) MaxTeam() int { return int(s.maxTeam.Load()) }

// New returns the service.
func New(o Options) *Service {
	if o.MaxTeam == 0 {
		o.MaxTeam = 6
	}
	if o.ReadyTimeout == 0 {
		o.ReadyTimeout = 45 * time.Second
	}
	if o.Settle == 0 {
		o.Settle = 2 * time.Second
	}
	s := &Service{o: o}
	s.maxTeam.Store(int64(o.MaxTeam))
	return s
}

// Agents a manager may create (the CLI's agent types).
var agentTypes = []string{"claude", "codex", "devin", "gemini", "shell", "custom"}

// Usage of the manager commands.
const Usage = `Manager commands (terminals in manager mode, e.g. the Planejador):
  torre new "Name" [--role impl] [--agent claude] [--cwd dir] [--floor F] [--command "…"]
        create a terminal next to you, connected to you and started; returns
        once its agent is ready. Defaults: your agent, your folder, your floor,
        no role. Roles: plan, arch, impl, rev, test or any role name (listed
        below, and by torre roles). Agents: claude,
        codex, devin, gemini, shell (a plain shell; --command runs in it),
        custom. --floor puts it on a floor (torre floor list), working in the
        floor's copy of the repository; --floor ground is the ground floor.
  torre connect "A" ["B"]   connect A and B (or you and A): terminals, notes, portals
  torre close "Name"        stop and remove a terminal (or portal) you created`

// Extension handles torre new, connect and close.
func (s *Service) Extension(ctx context.Context, caller string, args []string) (agentbus.Result, bool) {
	if len(args) == 0 {
		return agentbus.Result{}, false
	}
	switch args[0] {
	case "roles": // anyone: it only reads the library
		return res(200, s.RolesText()), true
	case "new", "connect", "close":
	default:
		return agentbus.Result{}, false
	}
	if len(args) > 1 && (args[1] == "help" || args[1] == "--help" || args[1] == "-h") {
		return res(200, Usage+"\n\n"+s.RolesText()), true
	}
	me, ok := s.o.Canvas.Store().Node(caller)
	if !ok || me.Terminal == nil {
		return res(400, "torre: unknown calling terminal"), true
	}
	if !s.IsManager(me) {
		return res(403, "torre: only terminals in manager mode can create or connect terminals (the Planejador is one).\nAsk the user to turn on manager mode (terminal menu → Modo gerente), or to connect the agents you need."), true
	}
	switch args[0] {
	case "new":
		return s.create(ctx, me, args[1:]), true
	case "connect":
		return s.connect(me, args[1:]), true
	default:
		return s.close(me, args[1:]), true
	}
}

// RolesText lists the roles of the library for agents: id, name, whether
// it's a manager and what it does (torre roles, torre new --help).
func (s *Service) RolesText() string {
	if s.o.Roles == nil {
		return "No role library."
	}
	var b strings.Builder
	b.WriteString(`Roles (a manager gives one with torre new "Name" --role <id>; the id or the name works):`)
	for _, r := range s.o.Roles.List() {
		fmt.Fprintf(&b, "\n- %s: %s", r.ID, r.Name)
		if r.Manager {
			b.WriteString(" (manager)")
		}
		if sum := roles.Summary(r); sum != "" {
			b.WriteString(" — " + sum)
		}
	}
	return b.String()
}

// IsManager reports whether a terminal may create and connect terminals.
func (s *Service) IsManager(n store.Node) bool {
	if n.Terminal == nil {
		return false
	}
	if n.Terminal.Manager != nil {
		return *n.Terminal.Manager
	}
	if s.o.Roles == nil || n.Terminal.Role == "" {
		return false
	}
	r, ok := s.o.Roles.Get(n.Terminal.Role)
	return ok && r.Manager
}

// Note describes the caller's manager mode for torre list ("" if it isn't one).
func (s *Service) Note(caller string) string {
	me, ok := s.o.Canvas.Store().Node(caller)
	if !ok || !s.IsManager(me) {
		return ""
	}
	return fmt.Sprintf("You are a manager: create the terminals you need with torre new (%d of %d created; torre new --help). For parallel work on the same files, give each front its own floor (torre floor --help).", s.created(caller, store.KindTerminal), s.MaxTeam())
}

func (s *Service) created(caller, kind string) int {
	count := 0
	for _, n := range s.o.Canvas.Store().Snapshot().Nodes {
		if n.CreatedBy == caller && n.Kind == kind {
			count++
		}
	}
	return count
}

type newArgs struct {
	name, agent, role, cwd, command, floor string
}

func parseNew(args []string) (newArgs, error) {
	var a newArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		key, value, hasValue := strings.Cut(arg, "=")
		var dst *string
		switch key {
		case "--agent":
			dst = &a.agent
		case "--role":
			dst = &a.role
		case "--cwd":
			dst = &a.cwd
		case "--command":
			dst = &a.command
		case "--floor":
			dst = &a.floor
		default:
			if strings.HasPrefix(arg, "--") {
				return a, fmt.Errorf("unknown option %s", arg)
			}
			if a.name != "" {
				return a, fmt.Errorf("one name only (got %q and %q); quote names with spaces", a.name, arg)
			}
			a.name = strings.TrimSpace(arg)
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return a, fmt.Errorf("%s needs a value", key)
			}
			i++
			value = args[i]
		}
		*dst = strings.TrimSpace(value)
	}
	if a.name == "" {
		return a, errors.New("a name is needed")
	}
	return a, nil
}

func (s *Service) create(ctx context.Context, me store.Node, args []string) agentbus.Result {
	a, err := parseNew(args)
	if err != nil {
		return res(400, "torre: "+err.Error()+"\n\n"+Usage)
	}
	if n := s.created(me.ID, store.KindTerminal); n >= s.MaxTeam() {
		return res(409, fmt.Sprintf("torre: you already created %d terminals, the limit. Close one you no longer need (torre close \"Name\") or ask the user.", n))
	}
	if a.agent == "" {
		a.agent = me.Terminal.Agent
		if a.agent == "shell" || a.agent == "custom" {
			a.agent = "claude"
		}
	}
	if !contains(agentTypes, a.agent) {
		return res(400, fmt.Sprintf("torre: unknown agent %q (one of: %s)", a.agent, strings.Join(agentTypes, ", ")))
	}
	if a.agent == "custom" && a.command == "" {
		return res(400, "torre: --agent custom needs --command")
	}
	if agentbus.Safe(ctx) && (a.command != "" || !agentbus.IsAgent(a.agent)) {
		return res(403, "torre: a shell or a command line needs the user's approval: use the new_shell tool.")
	}
	role := ""
	if a.role != "" && a.agent != "shell" {
		r, ok := s.findRole(a.role)
		if !ok {
			return res(400, fmt.Sprintf("torre: unknown role %q (roles: %s)", a.role, s.roleNames()))
		}
		role = r.ID
	}
	cwd := a.cwd
	if cwd == "" {
		cwd = me.Terminal.Cwd
	}
	st := s.o.Canvas.Store()
	floor, floorName := me.Floor, ""
	if a.floor != "" && !store.IsGround(a.floor) {
		f, ok := st.FindFloor(a.floor)
		if !ok {
			return res(404, fmt.Sprintf("torre: no floor named %q (torre floor list)", a.floor))
		}
		floor = f.ID
	} else if a.floor != "" {
		floor = ""
	}
	if f, ok := st.FindFloor(floor); ok {
		floorName = f.Name
	}
	w, h := 560.0, 440.0 // a new terminal's size on the canvas
	x, y := store.FreeSpot(store.OnFloor(st.Snapshot().Nodes, floor), me, w, h)
	off := false
	n, err := s.o.Canvas.AddNode(store.Node{
		Kind: store.KindTerminal, X: x, Y: y, W: w, H: h, CreatedBy: me.ID, Floor: floor,
		Terminal: &store.TerminalData{Name: a.name, Agent: a.agent, Command: a.command, Shell: me.Terminal.Shell, Cwd: cwd, Role: role, Manager: &off},
	})
	switch {
	case errors.Is(err, store.ErrDuplicate):
		return res(409, fmt.Sprintf("torre: there is already a terminal named %q; pick another name", a.name))
	case err != nil:
		return res(400, "torre: "+err.Error())
	}
	if _, err := s.o.Canvas.AddEdge(me.ID, n.ID, nil); err != nil {
		return res(500, "torre: created "+strconv.Quote(a.name)+" but could not connect it: "+err.Error())
	}

	what := a.agent
	if role != "" {
		r, _ := s.o.Roles.Get(role)
		what += ", role " + strconv.Quote(r.Name)
	}
	if floorName != "" {
		what += ", on floor " + strconv.Quote(floorName)
	}
	if a.agent == "shell" || a.agent == "custom" {
		return res(200, fmt.Sprintf("Created %q (%s), connected to you. Read its screen with torre check %q.", a.name, what, a.name))
	}
	if !s.waitReady(ctx, n.ID) {
		screen := ""
		if t := s.o.Canvas.Terminals().Get(n.ID); t != nil {
			screen = tail(t.Snapshot(), 12)
		}
		return res(200, fmt.Sprintf("Created %q (%s), connected to you, but it is not ready yet (%s). Check it with torre check %q before asking it anything. Its screen:\n%s",
			a.name, what, s.o.ReadyTimeout, a.name, screen))
	}
	return res(200, fmt.Sprintf("Created %q (%s), connected to you and ready. Give it work with torre ask %q \"…\".", a.name, what, a.name))
}

// waitReady waits until a new agent has started and its screen settled, so a
// first message reaches the agent and not the shell it is launched from.
func (s *Service) waitReady(ctx context.Context, id string) bool {
	if s.o.Monitor == nil {
		return true
	}
	deadline := time.Now().Add(s.o.ReadyTimeout)
	for time.Now().Before(deadline) {
		st, _ := s.o.Monitor.State(id)
		switch {
		case st == activity.Err || st == activity.Off:
			return false
		case st != activity.Boot && !s.o.Monitor.Working(id) && s.o.Monitor.StableFor(id) >= s.o.Settle:
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(200 * time.Millisecond):
		}
	}
	return false
}

func (s *Service) connect(me store.Node, args []string) agentbus.Result {
	if len(args) < 1 || len(args) > 2 {
		return res(400, `Usage: torre connect "A" ["B"]   (one name connects it to you)`)
	}
	a := me
	if len(args) == 2 {
		var err error
		if a, err = s.byName(args[0]); err != nil {
			return res(404, "torre: "+err.Error())
		}
	}
	b, err := s.byName(args[len(args)-1])
	if err != nil {
		return res(404, "torre: "+err.Error())
	}
	_, err = s.o.Canvas.AddEdge(a.ID, b.ID, nil)
	switch {
	case errors.Is(err, store.ErrDuplicate):
		return res(200, fmt.Sprintf("%q and %q are already connected.", a.Label(), b.Label()))
	case err != nil:
		return res(400, "torre: "+err.Error())
	}
	return res(200, fmt.Sprintf("Connected %q and %q.", a.Label(), b.Label()))
}

func (s *Service) close(me store.Node, args []string) agentbus.Result {
	if len(args) != 1 {
		return res(400, `Usage: torre close "Name"`)
	}
	n, err := s.byName(args[0])
	if err != nil {
		return res(404, "torre: "+err.Error())
	}
	if n.CreatedBy != me.ID {
		return res(403, fmt.Sprintf("torre: you can only close what you created; %q was not. Ask the user.", n.Label()))
	}
	if err := s.o.Canvas.RemoveNode(n.ID); err != nil {
		return res(500, "torre: "+err.Error())
	}
	return res(200, fmt.Sprintf("Closed %q.", n.Label()))
}

// byName finds a node of the workspace by its label (terminal name, portal
// name or note title), in any case.
func (s *Service) byName(name string) (store.Node, error) {
	var found []store.Node
	for _, n := range s.o.Canvas.Store().Snapshot().Nodes {
		if n.Visual() { // the user's own frames and drawings
			continue
		}
		// A note the user renamed still answers to its former names.
		if strings.EqualFold(n.Label(), name) || n.Note != nil && slices.ContainsFunc(n.Note.Aliases, func(a string) bool { return strings.EqualFold(a, name) }) ||
			n.Terminal != nil && n.Terminal.Callsign != "" && strings.EqualFold(n.Terminal.Callsign, name) {
			found = append(found, n)
		}
	}
	switch len(found) {
	case 0:
		return store.Node{}, fmt.Errorf("nothing named %q on the canvas", name)
	case 1:
		return found[0], nil
	}
	return store.Node{}, fmt.Errorf("%d nodes are named %q; rename one on the canvas", len(found), name)
}

func (s *Service) findRole(q string) (roles.Role, bool) {
	if s.o.Roles == nil {
		return roles.Role{}, false
	}
	for _, r := range s.o.Roles.List() {
		if r.ID == q || strings.EqualFold(r.Name, q) {
			return r, true
		}
	}
	return roles.Role{}, false
}

func (s *Service) roleNames() string {
	if s.o.Roles == nil {
		return "none"
	}
	var names []string
	for _, r := range s.o.Roles.List() {
		names = append(names, fmt.Sprintf("%s (%s)", r.ID, r.Name))
	}
	return strings.Join(names, ", ")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// tail returns the last non-empty lines of a screen.
func tail(screen string, n int) string {
	lines := strings.Split(strings.TrimRight(screen, " \n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func res(code int, body string) agentbus.Result { return agentbus.Result{Code: code, Body: body} }

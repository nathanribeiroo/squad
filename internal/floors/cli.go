package floors

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nathanribeiroo/squad/internal/agentbus"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Usage of torre floor.
const Usage = `Floors: isolated copies of the repository (a git worktree on its own branch),
each with its own canvas, for work in parallel that touches the same files.
  torre floor list                  the floors: branch, status, folder
  torre floor create "Name" [--branch B] [--existing]
        managers: a new floor from the last commit (uncommitted changes stay
        behind), with its own branch (default: the name). Then create its
        agents with torre new "Name" --floor "Floor": they work in its folder.
  torre floor land "Name" [--merge | --into BRANCH]
        managers, floors you created: close the floor's terminals and remove
        its folder, keeping its branch. --merge first merges the branch into
        the one the floor started from (--into: another branch). Refused while
        the floor has uncommitted changes: its agents commit first.`

// Extension handles torre floor.
func (s *Service) Extension(ctx context.Context, caller string, args []string) (agentbus.Result, bool) {
	if len(args) == 0 || args[0] != "floor" {
		return agentbus.Result{}, false
	}
	args = args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return res(200, Usage), true
	}
	me, ok := s.o.Canvas.Store().Node(caller)
	if !ok || me.Terminal == nil {
		return res(400, "torre: unknown calling terminal"), true
	}
	switch args[0] {
	case "list":
		return s.cliList(me), true
	case "create", "land":
	default:
		return res(400, fmt.Sprintf("torre: unknown floor command %q\n\n%s", args[0], Usage)), true
	}
	if !s.o.IsManager(me) {
		return res(403, "torre: only terminals in manager mode can create and land floors (the Planejador is one). Ask the user, who can also do it in the Torre window."), true
	}
	if args[0] == "create" {
		return s.cliCreate(ctx, me, args[1:]), true
	}
	if agentbus.Safe(ctx) {
		return res(403, "torre: landing a floor needs the user's approval: use the floor_land tool."), true
	}
	return s.cliLand(ctx, me, args[1:]), true
}

func (s *Service) cliList(me store.Node) agentbus.Result {
	st := s.State()
	var b strings.Builder
	where := "the ground floor"
	if f, ok := s.o.Canvas.Store().Floor(me.Floor); ok {
		where = fmt.Sprintf("floor %q", f.Name)
	}
	fmt.Fprintf(&b, "You are on %s.\n", where)
	if len(st.Floors) == 0 {
		b.WriteString("No floors yet. Managers create one with torre floor create \"Name\".\n")
		return res(200, b.String())
	}
	b.WriteString("Floors:\n")
	for _, f := range st.Floors {
		from := f.Base
		if from == "" {
			from = shortHash(f.BaseCommit)
		}
		state := f.Status
		switch {
		case f.Missing:
			state = "folder missing"
		case f.Status == store.FloorFailed:
			state = "setup failed: " + firstLine(f.Error)
		case f.Status == store.FloorPreparing:
			state = "preparing (setup running)"
		}
		fmt.Fprintf(&b, "- %q: branch %s (from %s), %s, %d terminal(s)", f.Name, f.Branch, from, state, f.Terminals)
		if f.CreatedBy == me.ID {
			b.WriteString(", created by you")
		}
		fmt.Fprintf(&b, "\n  folder: %s\n", f.Cwd)
	}
	return res(200, b.String())
}

func (s *Service) cliCreate(ctx context.Context, me store.Node, args []string) agentbus.Result {
	req := CreateRequest{By: me.ID}
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, value, hasValue := strings.Cut(a, "=")
		switch key {
		case "--existing":
			req.Existing = true
		case "--branch":
			if !hasValue {
				if i+1 >= len(args) {
					return res(400, "torre: --branch needs a value")
				}
				i++
				value = args[i]
			}
			req.Branch = strings.TrimSpace(value)
		default:
			if strings.HasPrefix(a, "-") {
				return res(400, fmt.Sprintf("torre: unknown option %s\n\n%s", a, Usage))
			}
			if req.Name != "" {
				return res(400, fmt.Sprintf("torre: one name only (got %q and %q); quote names with spaces", req.Name, a))
			}
			req.Name = a
		}
	}
	if strings.TrimSpace(req.Name) == "" {
		return res(400, `Usage: torre floor create "Name" [--branch B] [--existing]`)
	}
	f, err := s.Create(context.WithoutCancel(ctx), req)
	if err != nil {
		return errResult(err)
	}
	from := f.Base
	if from == "" {
		from = "commit " + shortHash(f.BaseCommit)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Created floor %q: branch %s, from the last commit of %s.\nFolder: %s\n", f.Name, f.Branch, from, f.Cwd)
	if len(f.Copied) > 0 {
		fmt.Fprintf(&b, "Copied from the ground floor (not to be committed): %s\n", strings.Join(f.Copied, ", "))
	}
	if f.Error != "" {
		fmt.Fprintf(&b, "Warning: %s\n", f.Error)
	}
	if f.Status == store.FloorPreparing {
		setup := s.o.Canvas.Store().FloorConfig().Setup
		if s.WaitReady(ctx, f.ID, s.o.ReadyWait) {
			f, _ = s.o.Canvas.Store().Floor(f.ID)
		}
		switch f.Status {
		case store.FloorPreparing:
			fmt.Fprintf(&b, "Setup (%s) is still running: torre floor list shows when it is done.\n", setup)
		case store.FloorFailed:
			fmt.Fprintf(&b, "Setup (%s) failed; the floor works, but may miss dependencies:\n%s\n", setup, f.Error)
		default:
			fmt.Fprintf(&b, "Setup (%s) done.\n", setup)
		}
	}
	fmt.Fprintf(&b, "Next: create its agents with torre new \"Name\" --role impl --floor %q (they work in that folder and should commit there), then land it with torre floor land %q --merge.", f.Name, f.Name)
	return res(200, b.String())
}

func (s *Service) cliLand(ctx context.Context, me store.Node, args []string) agentbus.Result {
	var name string
	req := LandRequest{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, value, hasValue := strings.Cut(a, "=")
		switch key {
		case "--merge":
			req.Merge = true
		case "--into":
			if !hasValue {
				if i+1 >= len(args) {
					return res(400, "torre: --into needs a branch")
				}
				i++
				value = args[i]
			}
			req.Into = strings.TrimSpace(value)
		default:
			if strings.HasPrefix(a, "-") {
				return res(400, fmt.Sprintf("torre: unknown option %s\n\n%s", a, Usage))
			}
			if name != "" {
				return res(400, fmt.Sprintf("torre: one floor only (got %q and %q)", name, a))
			}
			name = a
		}
	}
	if name == "" {
		return res(400, `Usage: torre floor land "Name" [--merge | --into BRANCH]`)
	}
	f, ok := s.o.Canvas.Store().FindFloor(name)
	if !ok {
		return res(404, fmt.Sprintf("torre: no floor named %q (torre floor list)", name))
	}
	if f.CreatedBy != me.ID {
		return res(403, fmt.Sprintf("torre: you can only land floors you created; %q was not. Ask the user, who can land it in the Torre window.", f.Name))
	}
	if me.Floor == f.ID {
		return res(403, "torre: you are on this floor, and landing it closes its terminals (you too). Ask whoever created it, or the user.")
	}
	r, err := s.Land(context.WithoutCancel(ctx), f.ID, req)
	if err != nil {
		return errResult(err)
	}
	switch {
	case r.UpToDate:
		return res(200, fmt.Sprintf("Landed %q: %s already had every commit of %s, so nothing was merged. Its terminals were closed and its folder removed; the branch %s is kept.", r.Floor, r.Into, r.Branch, r.Branch))
	case r.Merged:
		return res(200, fmt.Sprintf("Landed %q: merged %s into %s (commit %s). Its terminals were closed and its folder removed; the branch %s is kept.", r.Floor, r.Branch, r.Into, shortHash(r.Commit), r.Branch))
	}
	return res(200, fmt.Sprintf("Landed %q without merging: its terminals were closed and its folder removed. Its work stays on the branch %s.", r.Floor, r.Branch))
}

func errResult(err error) agentbus.Result {
	var dirtyErr *DirtyError
	var conflict *ConflictError
	switch {
	case errors.As(err, &dirtyErr), errors.As(err, &conflict):
		return res(409, "torre: "+err.Error())
	case errors.Is(err, ErrNotFound):
		return res(404, "torre: "+err.Error())
	case errors.Is(err, ErrInvalid), errors.Is(err, store.ErrInvalid):
		return res(400, "torre: "+err.Error())
	}
	return res(500, "torre: "+err.Error())
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func res(code int, body string) agentbus.Result { return agentbus.Result{Code: code, Body: body} }

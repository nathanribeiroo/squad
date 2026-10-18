package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/floors"
	"github.com/nathanribeiroo/squad/internal/plans"
	"github.com/nathanribeiroo/squad/internal/store"
)

// Flight plans on a workspace (UI only, never /cli or MCP: applying one
// starts processes):
//
//	POST /api/plans             {name, scope, nodes?, floor?, notesContent?} → the saved plan
//	POST /api/plans/{id}/apply  {floor?, commands} → what was made
//
// The plans themselves live in the hub (GET/DELETE /api/plans, import).

// planSave says what to keep: the selected nodes, a level or the session.
type planSave struct {
	Name         string   `json:"name"`
	Scope        string   `json:"scope"` // selection, level, session
	Nodes        []string `json:"nodes"`
	Floor        string   `json:"floor"`
	NotesContent bool     `json:"notesContent"`
}

// planApply puts a plan in the air on a level; Commands are the command
// lines the user saw before (every one of the plan's must be there).
type planApply struct {
	Floor    string   `json:"floor"`
	Commands []string `json:"commands"`
}

// PlanResult tells what applying a plan made, and what it couldn't.
type PlanResult struct {
	Nodes    []string `json:"nodes"`
	Floors   []string `json:"floors"`
	Warnings []string `json:"warnings"`
}

func (a *App) handlePlanSave(w http.ResponseWriter, r *http.Request) {
	var req planSave
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	p, err := a.buildPlan(req)
	if err == nil {
		p, err = a.plans.Save(p)
	}
	if err != nil {
		writePlanError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (a *App) handlePlanApply(w http.ResponseWriter, r *http.Request) {
	var req planApply
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	p, err := a.plans.Get(r.PathValue("id"))
	if err != nil {
		writePlanError(w, err)
		return
	}
	// Nothing runs that the user didn't see listed.
	for _, c := range p.Commands() {
		if !slices.Contains(req.Commands, c) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "review the plan's commands before it flies", "code": "plan.review", "commands": p.Commands()})
			return
		}
	}
	res, err := a.applyPlan(context.WithoutCancel(r.Context()), p, req.Floor)
	if err != nil {
		writePlanError(w, err)
		return
	}
	writeJSON(w, res)
}

func writePlanError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, plans.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, plans.ErrInvalid), errors.Is(err, store.ErrInvalid):
		code = http.StatusBadRequest
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(apierr.Body(err))
}

// buildPlan reads a plan off the canvas. Each level's items are placed
// from its top-left corner; the wires kept are the ones between kept items.
// Groups are kept open.
func (a *App) buildPlan(req planSave) (plans.Plan, error) {
	st := a.Server.Store()
	snap := st.Snapshot()
	// A selected group brings what is inside it.
	sel := map[string]bool{}
	for _, id := range req.Nodes {
		sel[id] = true
	}
	for _, n := range snap.Nodes {
		if n.Group != nil && sel[n.ID] && req.Scope == "selection" {
			for _, m := range store.Inside(snap.Nodes, n) {
				sel[m.ID] = true
			}
		}
	}
	var picked []store.Node
	for _, n := range snap.Nodes {
		switch req.Scope {
		case "selection": // a selected stack brings its notes
			if sel[n.ID] || n.Note != nil && n.Note.Stack != "" && sel[n.Note.Stack] {
				picked = append(picked, n)
			}
		case "level":
			if n.Floor == req.Floor {
				picked = append(picked, n)
			}
		case "session":
			picked = append(picked, n)
		default:
			return plans.Plan{}, apierr.With(fmt.Errorf("%w: scope is selection, level or session", plans.ErrInvalid), "plan.bad_scope")
		}
	}
	p := plans.Plan{Name: req.Name, Flights: []plans.Flight{}, Notes: []plans.Note{}, Portals: []plans.Portal{}, Links: [][2]string{}, Levels: []plans.Level{},
		Groups: []plans.Group{}, Drawings: []plans.Drawing{}}
	levelKey := map[string]string{"": ""}
	origin := map[string][2]float64{}
	for _, n := range picked {
		if _, ok := levelKey[n.Floor]; !ok {
			f, _ := st.Floor(n.Floor)
			key := fmt.Sprintf("l%d", len(p.Levels)+1)
			levelKey[n.Floor] = key
			p.Levels = append(p.Levels, plans.Level{Key: key, Name: f.Name, Branch: f.Branch})
		}
		o, ok := origin[n.Floor]
		if !ok {
			o = [2]float64{math.Inf(1), math.Inf(1)}
		}
		origin[n.Floor] = [2]float64{min(o[0], n.X), min(o[1], n.Y)}
	}
	box := func(n store.Node) plans.Box {
		o := origin[n.Floor]
		x, y, w, h := n.Box() // a collapsed group goes open
		return plans.Box{X: math.Round(x - o[0]), Y: math.Round(y - o[1]), W: w, H: h}
	}
	workdir := st.Workdir()
	keyOf := map[string]string{}
	stacked := map[int]store.Node{} // plan note → its note on the canvas, when stacked
	for _, n := range picked {
		lvl := levelKey[n.Floor]
		switch {
		case n.Terminal != nil:
			t := n.Terminal
			key := fmt.Sprintf("f%d", len(p.Flights)+1)
			f := plans.Flight{Key: key, Name: t.Name, Callsign: t.Callsign, Agent: t.Agent, Role: t.Role, Manager: t.Manager, Command: t.Command, Level: lvl, Box: box(n)}
			f.Cwd = portableDir(t.Cwd, workdir)
			p.Flights = append(p.Flights, f)
			keyOf[n.ID] = key
		case n.Note != nil:
			key := fmt.Sprintf("n%d", len(p.Notes)+1)
			note := plans.Note{Key: key, Title: n.Note.Title, Level: lvl, Box: box(n)}
			if req.NotesContent && n.Note.File != "" {
				if c, err := a.Notes.Read(n.Note.File); err == nil {
					note.Content = c
				}
			}
			if n.Note.Stack != "" {
				stacked[len(p.Notes)] = n
			}
			p.Notes = append(p.Notes, note)
			keyOf[n.ID] = key
		case n.Portal != nil:
			key := fmt.Sprintf("p%d", len(p.Portals)+1)
			name := n.Portal.Name
			if name == "" {
				name = "Portal"
			}
			p.Portals = append(p.Portals, plans.Portal{Key: key, Name: name, URL: n.Portal.URL, Level: lvl, Box: box(n)})
			keyOf[n.ID] = key
		case n.Group != nil:
			key := fmt.Sprintf("g%d", len(p.Groups)+1)
			p.Groups = append(p.Groups, plans.Group{Key: key, Name: n.Group.Name, Color: n.Group.Color, Level: lvl, Box: box(n)})
		case n.Drawing != nil:
			d := n.Drawing
			key := fmt.Sprintf("d%d", len(p.Drawings)+1)
			p.Drawings = append(p.Drawings, plans.Drawing{Key: key, Shape: d.Shape, Points: slices.Clone(d.Points), Color: d.Color, Text: d.Text, Level: lvl, Box: box(n)})
		}
	}
	for i, n := range stacked {
		if base := keyOf[n.Note.Stack]; base != "" {
			p.Notes[i].Stack, p.Notes[i].StackPos = base, n.Note.StackPos
		}
	}
	for _, e := range snap.Edges {
		if ka, kb := keyOf[e.A], keyOf[e.B]; ka != "" && kb != "" {
			p.Links = append(p.Links, [2]string{ka, kb})
		}
	}
	return p, p.Check()
}

// portableDir keeps a terminal's folder when it travels: relative to the
// workspace folder, or nothing when it is elsewhere.
func portableDir(dir, workdir string) string {
	if dir == "" || !filepath.IsAbs(dir) {
		return dir
	}
	rel, err := filepath.Rel(workdir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	if rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

// applyPlan makes the plan's items: its levels as new levels (named after
// them), the rest on floor beside what is there. Names already taken get a
// number; what can't be made is a warning.
func (a *App) applyPlan(ctx context.Context, p plans.Plan, floor string) (PlanResult, error) {
	st := a.Server.Store()
	if _, ok := st.Floor(floor); floor != "" && !ok {
		return PlanResult{}, apierr.With(fmt.Errorf("%w: no level %q", plans.ErrInvalid, floor), "plan.no_level")
	}
	res := PlanResult{Nodes: []string{}, Floors: []string{}, Warnings: []string{}}
	warn := func(format string, args ...any) { res.Warnings = append(res.Warnings, fmt.Sprintf(format, args...)) }

	// Levels first: their items go there.
	floorOf := map[string]string{"": floor}
	for _, l := range p.Levels {
		f, err := a.newLevel(ctx, l.Name)
		if err != nil {
			warn("level %s: %v; its items go to this level", l.Name, err)
			floorOf[l.Key] = floor
			continue
		}
		floorOf[l.Key] = f.ID
		res.Floors = append(res.Floors, f.ID)
	}
	// Beside what is already on each level.
	offsets := map[string][2]float64{}
	offset := func(fl string) [2]float64 {
		if o, ok := offsets[fl]; ok {
			return o
		}
		o := [2]float64{0, 0}
		var maxX, minY float64 = math.Inf(-1), math.Inf(1)
		for _, n := range store.OnFloor(st.Snapshot().Nodes, fl) {
			maxX, minY = max(maxX, n.X+n.W), min(minY, n.Y)
		}
		if !math.IsInf(maxX, -1) {
			o = [2]float64{math.Round(maxX + 160), math.Round(minY)}
		}
		offsets[fl] = o
		return o
	}
	place := func(level string, b plans.Box) (string, float64, float64) {
		fl := floorOf[level]
		o := offset(fl)
		return fl, o[0] + b.X, o[1] + b.Y
	}

	idOf := map[string]string{}
	for _, f := range p.Flights {
		fl, x, y := place(f.Level, f.Box)
		role := f.Role
		if role != "" && a.roles != nil {
			if _, ok := a.roles.Get(role); !ok {
				warn("%s: there is no role %q here; it flies without one", f.Name, role)
				role = ""
			}
		}
		t := store.TerminalData{Name: f.Name, Agent: f.Agent, Callsign: f.Callsign, Role: role, Manager: f.Manager, Command: f.Command, Cwd: f.Cwd}
		var n store.Node
		var err error
		for i := 1; i <= 20; i++ {
			if i > 1 {
				t.Name = fmt.Sprintf("%s %d", f.Name, i)
			}
			n, err = a.Server.AddNode(store.Node{Kind: store.KindTerminal, Floor: fl, X: x, Y: y, W: f.W, H: f.H, Terminal: &t})
			if err != nil && t.Cwd != "" && !errors.Is(err, store.ErrDuplicate) {
				warn("%s: the folder %s isn't here; it starts in the workspace folder", f.Name, t.Cwd)
				t.Cwd = ""
				n, err = a.Server.AddNode(store.Node{Kind: store.KindTerminal, Floor: fl, X: x, Y: y, W: f.W, H: f.H, Terminal: &t})
			}
			if !errors.Is(err, store.ErrDuplicate) {
				break
			}
		}
		if err != nil {
			warn("%s: %v", f.Name, err)
			continue
		}
		idOf[f.Key] = n.ID
		res.Nodes = append(res.Nodes, n.ID)
	}
	// Note names count their former names too (agents still use them).
	noteTaken := func(name string) bool {
		for _, n := range st.Snapshot().Nodes {
			if n.Note != nil && (strings.EqualFold(n.Note.Title, name) || slices.ContainsFunc(n.Note.Aliases, func(a string) bool { return strings.EqualFold(a, name) })) {
				return true
			}
		}
		return false
	}
	for _, nt := range p.Notes {
		fl, x, y := place(nt.Level, nt.Box)
		n, err := a.Server.AddNode(store.Node{Kind: store.KindNote, Floor: fl, X: x, Y: y, W: nt.W, H: nt.H, Note: &store.NoteData{Title: freeName(nt.Title, noteTaken)}})
		if err != nil {
			warn("NOTAM %s: %v", nt.Title, err)
			continue
		}
		if nt.Content != "" {
			if err := a.Notes.Write(n, nt.Content, "Plano de voo"); err != nil {
				warn("NOTAM %s: %v", nt.Title, err)
			}
		}
		idOf[nt.Key] = n.ID
		res.Nodes = append(res.Nodes, n.ID)
	}
	// The stacks again, in their order.
	order := slices.Clone(p.Notes)
	slices.SortStableFunc(order, func(x, y plans.Note) int { return x.StackPos - y.StackPos })
	for _, nt := range order {
		base, id := idOf[nt.Stack], idOf[nt.Key]
		if nt.Stack == "" || nt.Stack == nt.Key || base == "" || id == "" {
			continue
		}
		ch, err := st.Stack(id, base)
		if err != nil {
			warn("NOTAM %s: %v", nt.Title, err)
			continue
		}
		a.Server.PublishStack(ch)
	}
	portalTaken := func(name string) bool {
		for _, n := range st.Snapshot().Nodes {
			if n.Portal != nil && strings.EqualFold(n.Portal.Name, name) {
				return true
			}
		}
		return false
	}
	for _, o := range p.Portals {
		fl, x, y := place(o.Level, o.Box)
		n, err := a.Server.AddNode(store.Node{Kind: store.KindPortal, Floor: fl, X: x, Y: y, W: o.W, H: o.H, Portal: &store.PortalData{Name: freeName(o.Name, portalTaken), URL: o.URL}})
		if err != nil {
			warn("portal %s: %v", o.Name, err)
			continue
		}
		idOf[o.Key] = n.ID
		res.Nodes = append(res.Nodes, n.ID)
	}
	for _, g := range p.Groups {
		fl, x, y := place(g.Level, g.Box)
		n, err := a.Server.AddNode(store.Node{Kind: store.KindGroup, Floor: fl, X: x, Y: y, W: g.W, H: g.H, Group: &store.GroupData{Name: g.Name, Color: g.Color}})
		if err != nil {
			warn("group %s: %v", g.Name, err)
			continue
		}
		res.Nodes = append(res.Nodes, n.ID)
	}
	for _, d := range p.Drawings {
		fl, x, y := place(d.Level, d.Box)
		n, err := a.Server.AddNode(store.Node{Kind: store.KindDrawing, Floor: fl, X: x, Y: y, W: d.W, H: d.H,
			Drawing: &store.DrawingData{Shape: d.Shape, Points: d.Points, Color: d.Color, Text: d.Text}})
		if err != nil {
			warn("drawing %s: %v", d.Key, err)
			continue
		}
		res.Nodes = append(res.Nodes, n.ID)
	}
	for _, l := range p.Links {
		from, to := idOf[l[0]], idOf[l[1]]
		if from == "" || to == "" {
			continue
		}
		if _, err := a.Server.AddEdge(from, to, nil); err != nil {
			warn("frequency %s ↔ %s: %v", l[0], l[1], err)
		}
	}
	return res, nil
}

// freeName is name, or "name 2", "name 3"… when it is taken.
func freeName(name string, taken func(string) bool) string {
	try := name
	for i := 2; taken(try) && i <= 99; i++ {
		try = fmt.Sprintf("%s %d", name, i)
	}
	return try
}

// newLevel makes a level named name, or "name 2", "name 3"… when the name
// or its branch is taken.
func (a *App) newLevel(ctx context.Context, name string) (store.Floor, error) {
	var err error
	for i := 1; i <= 9; i++ {
		try := name
		if i > 1 {
			try = fmt.Sprintf("%s %d", name, i)
		}
		var f store.Floor
		f, err = a.Floors.Create(ctx, floors.CreateRequest{Name: try})
		if err == nil {
			return f, nil
		}
		if !takenLevel(err) {
			return store.Floor{}, err
		}
	}
	return store.Floor{}, err
}

// takenLevel reports whether a level couldn't be made because its name, its
// branch or its folder is already in use.
func takenLevel(err error) bool {
	var e *apierr.Error
	if !errors.As(err, &e) {
		return false
	}
	switch e.Code {
	case "level.name_taken", "level.folder_exists", "git.branch_exists":
		return true
	}
	return false
}

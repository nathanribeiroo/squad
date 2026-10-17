// Package plans keeps flight plans: a team saved from a canvas (flights,
// notes, portals, the wires between them and the levels they were on) that
// can be put in the air again in any workspace, with new conversations.
// They live in ~/.torre/plans, one JSON file each, and can be exported and
// imported as files.
package plans

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/ids"
)

// Plan is a saved team. Positions are relative to the plan's top-left on
// each level.
type Plan struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	// Imported: the plan came from a file. Its own command lines are listed
	// before it flies (they could run anything).
	Imported bool        `json:"imported,omitempty"`
	Flights  []Flight    `json:"flights"`
	Notes    []Note      `json:"notes"`
	Portals  []Portal    `json:"portals"`
	Links    [][2]string `json:"links"` // keys of linked items
	Levels   []Level     `json:"levels"`
}

// Box is where an item sits on its level, relative to the plan.
type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Flight is a terminal of the plan.
type Flight struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Callsign string `json:"callsign,omitempty"`
	Agent    string `json:"agent"`
	Role     string `json:"role,omitempty"`
	Manager  *bool  `json:"manager,omitempty"` // nil: as its role says
	Command  string `json:"command,omitempty"` // custom command line (agent "custom")
	Cwd      string `json:"cwd,omitempty"`     // relative to the workspace folder
	Level    string `json:"level,omitempty"`   // key of its level; "" the ground floor
	Box
}

// Note is a NOTAM of the plan, with its content when the user kept it.
type Note struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	Level   string `json:"level,omitempty"`
	Box
}

// Portal is a portal of the plan.
type Portal struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Level string `json:"level,omitempty"`
	Box
}

// Level is a flight level of the plan: a new one is made from it, named
// after it; Branch is the branch it had, as a reference.
type Level struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Branch string `json:"branch,omitempty"`
}

// Limits on what a plan may hold (an imported file is checked against them).
const (
	MaxName    = 60
	MaxItems   = 60
	MaxLevels  = 8
	MaxContent = 256 << 10 // per note
	MaxFile    = 4 << 20   // an imported plan
)

var (
	ErrNotFound = errors.New("plan not found")
	ErrInvalid  = errors.New("invalid plan")
)

// agentTypes are the agents a plan's flights may be.
var agentTypes = []string{"claude", "codex", "devin", "gemini", "shell", "custom"}

// Commands are the plan's own command lines: they run as written, so the
// user reviews them before the plan flies.
func (p Plan) Commands() []string {
	var out []string
	for _, f := range p.Flights {
		if c := strings.TrimSpace(f.Command); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func invalid(msg, code string, params ...string) error {
	return apierr.With(fmt.Errorf("%w: %s", ErrInvalid, msg), code, params...)
}

// Check validates a plan: a name, items with unique keys, links and levels
// that point at them, known agent types and sane sizes.
func (p *Plan) Check() error {
	p.Name = strings.TrimSpace(p.Name)
	switch {
	case p.Name == "":
		return invalid("a plan needs a name", "plan.name_required")
	case utf8.RuneCountInString(p.Name) > MaxName:
		return invalid(fmt.Sprintf("plan names have at most %d characters", MaxName), "plan.name_too_long", "max", fmt.Sprint(MaxName))
	case len(p.Flights)+len(p.Notes)+len(p.Portals) == 0:
		return invalid("the plan is empty", "plan.empty")
	case len(p.Flights)+len(p.Notes)+len(p.Portals) > MaxItems:
		return invalid(fmt.Sprintf("a plan holds at most %d items", MaxItems), "plan.too_big", "max", fmt.Sprint(MaxItems))
	case len(p.Levels) > MaxLevels:
		return invalid(fmt.Sprintf("a plan holds at most %d levels", MaxLevels), "plan.too_many_levels", "max", fmt.Sprint(MaxLevels))
	}
	levels := map[string]bool{"": true}
	for _, l := range p.Levels {
		if l.Key == "" || levels[l.Key] || strings.TrimSpace(l.Name) == "" {
			return invalid("levels need a unique key and a name", "plan.bad_item")
		}
		levels[l.Key] = true
	}
	keys := map[string]bool{}
	item := func(key, level, name string, b Box) error {
		if key == "" || keys[key] || strings.TrimSpace(name) == "" || !levels[level] {
			return invalid(fmt.Sprintf("item %q: a unique key, a name and a known level", key), "plan.bad_item")
		}
		if b.W < 0 || b.H < 0 || b.W > 5000 || b.H > 5000 || b.X < -1e6 || b.X > 1e6 || b.Y < -1e6 || b.Y > 1e6 {
			return invalid(fmt.Sprintf("item %q: a bad position or size", key), "plan.bad_item")
		}
		keys[key] = true
		return nil
	}
	for _, f := range p.Flights {
		if err := item(f.Key, f.Level, f.Name, f.Box); err != nil {
			return err
		}
		if !slices.Contains(agentTypes, f.Agent) {
			return invalid(fmt.Sprintf("flight %q: unknown agent %q", f.Name, f.Agent), "plan.bad_agent", "agent", f.Agent)
		}
		if f.Agent == "custom" && strings.TrimSpace(f.Command) == "" {
			return invalid(fmt.Sprintf("flight %q: a custom flight needs its command", f.Name), "plan.bad_item")
		}
	}
	for _, n := range p.Notes {
		if err := item(n.Key, n.Level, n.Title, n.Box); err != nil {
			return err
		}
		if len(n.Content) > MaxContent {
			return invalid(fmt.Sprintf("note %q is too long", n.Title), "plan.note_too_long", "note", n.Title)
		}
	}
	for _, o := range p.Portals {
		if err := item(o.Key, o.Level, o.Name, o.Box); err != nil {
			return err
		}
	}
	for _, l := range p.Links {
		if !keys[l[0]] || !keys[l[1]] || l[0] == l[1] {
			return invalid("links must join two items of the plan", "plan.bad_item")
		}
	}
	return nil
}

// Library is the folder of saved plans.
type Library struct {
	dir string
	mu  sync.Mutex
}

// Open uses dir (created when the first plan is saved).
func Open(dir string) *Library { return &Library{dir: dir} }

func (l *Library) path(id string) string { return filepath.Join(l.dir, id+".json") }

// List returns the plans, by name.
func (l *Library) List() ([]Plan, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := os.ReadDir(l.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Plan{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Plan{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() {
			continue
		}
		if p, err := l.read(id); err == nil {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// Get returns a plan.
func (l *Library) Get(id string) (Plan, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.read(id)
}

func (l *Library) read(id string) (Plan, error) {
	if id == "" || id != ids.Slug(id) {
		return Plan{}, ErrNotFound
	}
	b, err := os.ReadFile(l.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return Plan{}, fmt.Errorf("plan %s: %w", id, err)
	}
	p.ID = id
	return p.normal(), nil
}

// normal gives every list a value, so the JSON has [] and not null.
func (p Plan) normal() Plan {
	if p.Flights == nil {
		p.Flights = []Flight{}
	}
	if p.Notes == nil {
		p.Notes = []Note{}
	}
	if p.Portals == nil {
		p.Portals = []Portal{}
	}
	if p.Links == nil {
		p.Links = [][2]string{}
	}
	if p.Levels == nil {
		p.Levels = []Level{}
	}
	return p
}

// Save stores a new plan under a free id from its name, and returns it.
func (l *Library) Save(p Plan) (Plan, error) {
	if err := p.Check(); err != nil {
		return Plan{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	base := ids.Slug(p.Name)
	if base == "" {
		base = "plano"
	}
	p.ID = base
	for i := 2; ; i++ {
		if _, err := os.Stat(l.path(p.ID)); errors.Is(err, os.ErrNotExist) {
			break
		}
		p.ID = fmt.Sprintf("%s-%d", base, i)
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	b, err := json.MarshalIndent(p.normal(), "", "  ")
	if err != nil {
		return Plan{}, err
	}
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return Plan{}, err
	}
	tmp := l.path(p.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return Plan{}, err
	}
	return p.normal(), os.Rename(tmp, l.path(p.ID))
}

// Import stores a plan read from a file; it is marked imported.
func (l *Library) Import(data []byte) (Plan, error) {
	if len(data) > MaxFile {
		return Plan{}, invalid("the file is too big for a plan", "plan.bad_file")
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, invalid("not a flight plan: "+err.Error(), "plan.bad_file")
	}
	p.ID, p.CreatedAt, p.Imported = "", time.Time{}, true
	return l.Save(p)
}

// Delete removes a plan.
func (l *Library) Delete(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.read(id); err != nil {
		return err
	}
	return os.Remove(l.path(id))
}

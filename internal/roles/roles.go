// Package roles is the library of roles: reusable instructions that a
// terminal's agent receives when it starts. The built-in roles always
// exist; the user may edit them (and restore their default) or add new ones.
// The library is global, shared by every workspace (~/.torre/roles.json),
// which stores only new roles and edited built-in ones, so a built-in role
// nobody edited follows the defaults of newer Torre versions.
package roles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/apierr"
	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/ids"
)

// Role is one entry of the library. A terminal refers to it by ID.
type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Abbr: three letters (PLN, IMP…) that give its terminals their
	// callsigns; made from the name when not given.
	Abbr   string `json:"abbr,omitempty"`
	Prompt string `json:"prompt"` // the instruction the agent receives
	Color  string `json:"color"`  // one of Colors
	Icon   string `json:"icon"`   // one of Icons
	// Manager: its terminals may create and connect terminals (torre new),
	// unless a terminal turns it off.
	Manager bool `json:"manager,omitempty"`
	Builtin bool `json:"builtin,omitempty"`
	Edited  bool `json:"edited,omitempty"` // a built-in role that differs from its default
}

// Colors a role may take: the four of the built-in roles, then four more
// (CSS --r-<color> and --rb-<color> in the web UI).
var Colors = []string{"plan", "impl", "rev", "test", "teal", "indigo", "magenta", "orange"}

// Icons a role may take (Material Symbols; the web UI ships only these).
var Icons = []string{
	"account_tree", "code", "rate_review", "science", "bug_report", "palette",
	"description", "shield", "database", "rocket_launch", "psychology", "build",
	"architecture",
}

// Limits of a role's fields. The prompt limit keeps it within what every
// agent accepts (a Codex launch line, Devin's 10,000-character context).
const (
	MaxName   = 40
	MaxPrompt = 4000
)

var (
	ErrNotFound = errors.New("role not found")
	ErrInvalid  = errors.New("invalid role")
	ErrBuiltin  = errors.New("built-in roles can't be removed (restore the default instead)")
)

type invalidError string

func (e invalidError) Error() string        { return string(e) }
func (e invalidError) Is(target error) bool { return target == ErrInvalid }

// Library holds the roles and saves them.
type Library struct {
	path string

	mu    sync.Mutex
	roles []Role // built-in first, in the order of Defaults, then new ones
	subs  map[int]func([]Role)
	next  int
}

type file struct {
	SchemaVersion int    `json:"schemaVersion"`
	Roles         []Role `json:"roles"` // new roles and edited built-in ones
}

// Open loads the library from path; a missing file means the defaults.
func Open(path string) (*Library, error) {
	l := &Library{path: path, roles: Defaults(), subs: map[int]func([]Role){}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("roles: %s: %w", path, err)
	}
	for _, r := range f.Roles {
		if i := l.index(r.ID); i >= 0 && l.roles[i].Builtin {
			if r.Builtin { // an edited built-in role
				if r.Abbr == "" { // edited before roles had abbreviations
					r.Abbr = l.roles[i].Abbr
				}
				r.Edited = true
				l.roles[i] = r
				continue
			}
			r.ID = l.freeID(r.Name) // the user's role whose id a newer built-in took
		}
		if r.ID == "" || l.index(r.ID) >= 0 {
			continue
		}
		r.Builtin, r.Edited = false, false
		if r.Abbr == "" { // made before roles had abbreviations
			r.Abbr = l.freeAbbr(ids.Callsign(r.Name), r.ID)
		}
		// A built-in role added by a newer Torre may take the name of one the
		// user made (e.g. "Arquiteto"): theirs keeps its id and instructions
		// under another name, so --role and the role picker stay unambiguous.
		for l.nameTaken(r.Name) {
			r.Name = customName(r.Name)
		}
		l.roles = append(l.roles, r)
	}
	return l, nil
}

// nameTaken reports whether a role already has this name, in any case.
func (l *Library) nameTaken(name string) bool {
	for _, o := range l.roles {
		if strings.EqualFold(o.Name, name) {
			return true
		}
	}
	return false
}

// customName marks the name of a user's role that clashed with a built-in
// one, within MaxName.
func customName(name string) string {
	const suffix = " (personalizado)"
	if strings.HasSuffix(name, suffix) {
		return name + "*"
	}
	r := []rune(name)
	if max := MaxName - utf8.RuneCountInString(suffix); len(r) > max {
		r = r[:max]
	}
	return strings.TrimSpace(string(r)) + suffix
}

func (l *Library) index(id string) int {
	for i, r := range l.roles {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// List returns every role.
func (l *Library) List() []Role {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Role(nil), l.roles...)
}

// Get returns a role by id.
func (l *Library) Get(id string) (Role, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.index(id); i >= 0 {
		return l.roles[i], true
	}
	return Role{}, false
}

// Create adds a new role; its id comes from its name.
func (l *Library) Create(r Role) (Role, error) {
	return l.mutate(func() (Role, error) {
		r.Builtin, r.Edited = false, false
		if err := l.check(&r, ""); err != nil {
			return Role{}, err
		}
		r.ID = l.freeID(r.Name)
		l.roles = append(l.roles, r)
		return r, nil
	})
}

// Update changes a role's name, prompt, color and icon.
func (l *Library) Update(id string, r Role) (Role, error) {
	return l.mutate(func() (Role, error) {
		i := l.index(id)
		if i < 0 {
			return Role{}, ErrNotFound
		}
		if err := l.check(&r, id); err != nil {
			return Role{}, err
		}
		r.ID, r.Builtin, r.Edited = id, l.roles[i].Builtin, false
		if d, ok := defaultOf(id); ok && r.Builtin {
			r.Edited = r.Name != d.Name || r.Abbr != d.Abbr || r.Prompt != d.Prompt || r.Color != d.Color || r.Icon != d.Icon || r.Manager != d.Manager
		}
		l.roles[i] = r
		return r, nil
	})
}

// Reset restores a built-in role's default.
func (l *Library) Reset(id string) (Role, error) {
	return l.mutate(func() (Role, error) {
		i := l.index(id)
		if i < 0 {
			return Role{}, ErrNotFound
		}
		d, ok := defaultOf(id)
		if !ok {
			return Role{}, invalidError("only built-in roles have a default")
		}
		l.roles[i] = d
		return d, nil
	})
}

// Delete removes a role that isn't built in.
func (l *Library) Delete(id string) error {
	_, err := l.mutate(func() (Role, error) {
		i := l.index(id)
		if i < 0 {
			return Role{}, ErrNotFound
		}
		if l.roles[i].Builtin {
			return Role{}, ErrBuiltin
		}
		l.roles = append(l.roles[:i], l.roles[i+1:]...)
		return Role{}, nil
	})
	return err
}

// mutate runs fn under the lock, saves, and then tells the subscribers. A
// change that can't be saved is undone.
func (l *Library) mutate(fn func() (Role, error)) (Role, error) {
	l.mu.Lock()
	before := append([]Role(nil), l.roles...)
	r, err := fn()
	if err == nil {
		if err = l.saveLocked(); err != nil {
			l.roles = before
		}
	}
	var subs []func([]Role)
	list := append([]Role(nil), l.roles...)
	if err == nil {
		for _, s := range l.subs {
			subs = append(subs, s)
		}
	}
	l.mu.Unlock()
	for _, s := range subs {
		s(list)
	}
	return r, err
}

// Subscribe calls fn with the whole list after every change.
func (l *Library) Subscribe(fn func([]Role)) (cancel func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := l.next
	l.next++
	l.subs[id] = fn
	return func() {
		l.mu.Lock()
		delete(l.subs, id)
		l.mu.Unlock()
	}
}

// check normalizes and validates r; except is the id of the role being
// updated (its own name isn't a clash).
func (l *Library) check(r *Role, except string) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Prompt = strings.TrimSpace(strings.ReplaceAll(r.Prompt, "\r\n", "\n"))
	switch n := utf8.RuneCountInString(r.Name); {
	case n == 0:
		return apierr.With(invalidError("a role needs a name"), "role.name_required")
	case n > MaxName:
		return apierr.With(invalidError(fmt.Sprintf("role names have at most %d characters", MaxName)), "role.name_too_long", "max", fmt.Sprint(MaxName))
	}
	if utf8.RuneCountInString(r.Prompt) > MaxPrompt {
		return apierr.With(invalidError(fmt.Sprintf("role instructions have at most %d characters", MaxPrompt)), "role.prompt_too_long", "max", fmt.Sprint(MaxPrompt))
	}
	for _, o := range l.roles {
		if o.ID != except && strings.EqualFold(o.Name, r.Name) {
			return apierr.With(invalidError(fmt.Sprintf("there is already a role named %q", o.Name)), "role.name_taken", "name", o.Name)
		}
	}
	if r.Abbr = ids.CleanCallsign(r.Abbr); r.Abbr != "" {
		if len(r.Abbr) > 3 {
			return apierr.With(invalidError("a role's abbreviation has at most 3 letters"), "role.abbr_too_long")
		}
		if o := l.abbrOwner(r.Abbr, except); o != "" {
			return apierr.With(invalidError(fmt.Sprintf("the role %q already has the abbreviation %s", o, r.Abbr)), "role.abbr_taken", "role", o, "abbr", r.Abbr)
		}
	} else {
		r.Abbr = l.freeAbbr(ids.Callsign(r.Name), except)
	}
	if r.Color == "" {
		r.Color = Colors[len(l.roles)%len(Colors)]
	}
	if !contains(Colors, r.Color) {
		return invalidError("unknown role color " + r.Color)
	}
	if r.Icon == "" {
		r.Icon = "psychology"
	}
	if !contains(Icons, r.Icon) {
		return invalidError("unknown role icon " + r.Icon)
	}
	return nil
}

// abbrOwner is the name of the role (other than except) whose abbreviation
// is abbr, "" when none.
func (l *Library) abbrOwner(abbr, except string) string {
	for _, o := range l.roles {
		if o.ID != except && strings.EqualFold(o.Abbr, abbr) {
			return o.Name
		}
	}
	return ""
}

// freeAbbr is base, numbered when another role has it (RVS, RV2…).
func (l *Library) freeAbbr(base, except string) string {
	if base == "" {
		base = "PPL"
	}
	abbr := base
	for i := 2; i < 100 && l.abbrOwner(abbr, except) != ""; i++ {
		n := fmt.Sprint(i)
		abbr = base[:max(1, min(len(base), 3)-len(n))] + n
	}
	return abbr
}

// freeID turns a name into a readable id that no role uses yet.
func (l *Library) freeID(name string) string {
	base := ids.Slug(name)
	if base == "" {
		base = "role"
	}
	id := base
	for i := 2; l.index(id) >= 0; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

func (l *Library) saveLocked() error {
	f := file{SchemaVersion: 1, Roles: []Role{}}
	for _, r := range l.roles {
		if !r.Builtin || r.Edited {
			f.Roles = append(f.Roles, r)
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(l.path, data, 0o600)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Summary is what a role does in one line: the first line of its
// instructions (built-in roles start with it), shortened.
func Summary(r Role) string {
	line, _, _ := strings.Cut(strings.TrimSpace(r.Prompt), "\n")
	line = strings.TrimSpace(line)
	if rs := []rune(line); len(rs) > 160 {
		line = strings.TrimSpace(string(rs[:159])) + "…"
	}
	return line
}

// Instructions is what an agent receives for its role: a header that names
// the terminal and the role, then the role's prompt.
func Instructions(r Role, terminal string) string {
	return fmt.Sprintf("# Papel na Torre: %s\n\nVocê está no terminal %q da Torre, com o papel %q. Siga estas instruções do papel:\n\n%s\n",
		r.Name, terminal, r.Name, r.Prompt)
}

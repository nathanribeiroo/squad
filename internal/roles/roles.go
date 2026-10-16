// Package roles is the library of roles: reusable instructions that a
// terminal's agent receives when it starts. The four built-in roles always
// exist; the user may edit them (and restore their default) or add new ones.
// The library is global, shared by every workspace (~/.squad/roles.json),
// which stores only new roles and edited built-in ones, so a built-in role
// nobody edited follows the defaults of newer Squad versions.
package roles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/nathanribeiroo/squad/internal/config"
	"github.com/nathanribeiroo/squad/internal/ids"
)

// Role is one entry of the library. A terminal refers to it by ID.
type Role struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prompt string `json:"prompt"` // the instruction the agent receives
	Color  string `json:"color"`  // one of Colors
	Icon   string `json:"icon"`   // one of Icons
	// Manager: its terminals may create and connect terminals (squad new),
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
			r.Builtin, r.Edited = true, true
			l.roles[i] = r
			continue
		}
		if r.ID == "" || l.index(r.ID) >= 0 {
			continue
		}
		r.Builtin, r.Edited = false, false
		l.roles = append(l.roles, r)
	}
	return l, nil
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
			r.Edited = r.Name != d.Name || r.Prompt != d.Prompt || r.Color != d.Color || r.Icon != d.Icon || r.Manager != d.Manager
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
		return invalidError("a role needs a name")
	case n > MaxName:
		return invalidError(fmt.Sprintf("role names have at most %d characters", MaxName))
	}
	if utf8.RuneCountInString(r.Prompt) > MaxPrompt {
		return invalidError(fmt.Sprintf("role instructions have at most %d characters", MaxPrompt))
	}
	for _, o := range l.roles {
		if o.ID != except && strings.EqualFold(o.Name, r.Name) {
			return invalidError(fmt.Sprintf("there is already a role named %q", o.Name))
		}
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

// Instructions is what an agent receives for its role: a header that names
// the terminal and the role, then the role's prompt.
func Instructions(r Role, terminal string) string {
	return fmt.Sprintf("# Papel no Squad: %s\n\nVocê está no terminal %q do Squad, com o papel %q. Siga estas instruções do papel:\n\n%s\n",
		r.Name, terminal, r.Name, r.Prompt)
}

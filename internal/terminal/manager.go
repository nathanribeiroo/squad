package terminal

import (
	"errors"
	"sort"
	"sync"

	"github.com/nathanribeiroo/squad/internal/ids"
)

// Manager owns the running terminals.
type Manager struct {
	mu       sync.Mutex
	terms    map[string]*Terminal
	cliDir   string
	listener func(Event)
}

// NewManager creates a manager; cliDir is prepended to every terminal's PATH
// (normally the directory of the torre binary). listener may be nil.
func NewManager(cliDir string, listener func(Event)) *Manager {
	if listener == nil {
		listener = func(Event) {}
	}
	return &Manager{terms: map[string]*Terminal{}, cliDir: cliDir, listener: listener}
}

// Create registers and starts a terminal. If spec.ID is empty a new id is
// assigned. A terminal whose process fails to start is still registered (in
// StatusFailed) and the error is returned.
func (m *Manager) Create(spec Spec) (*Terminal, error) {
	if spec.ID == "" {
		spec.ID = ids.New()
	}
	m.mu.Lock()
	if _, exists := m.terms[spec.ID]; exists {
		m.mu.Unlock()
		return nil, errors.New("terminal id already exists")
	}
	t := &Terminal{
		spec:   spec,
		emit:   m.listener,
		subs:   map[int]*subscriber{},
		writeQ: make(chan writeJob, 64),
		quit:   make(chan struct{}),
	}
	m.terms[spec.ID] = t
	m.mu.Unlock()
	go t.writeLoop()
	return t, t.start(m.cliDir)
}

// Restart stops a terminal (if running) and starts it again with its spec.
func (m *Manager) Restart(id string) error {
	t := m.Get(id)
	if t == nil {
		return errors.New("terminal not found")
	}
	t.Stop()
	if old := t.Screen(); old != nil {
		_ = old.Close()
	}
	return t.start(m.cliDir)
}

// Get returns a terminal by id, or nil.
func (m *Manager) Get(id string) *Terminal {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.terms[id]
}

// List returns all terminals ordered by name.
func (m *Manager) List() []*Terminal {
	m.mu.Lock()
	out := make([]*Terminal, 0, len(m.terms))
	for _, t := range m.terms {
		out = append(out, t)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].spec.Name < out[j].spec.Name })
	return out
}

// Remove stops and forgets a terminal.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	t := m.terms[id]
	delete(m.terms, id)
	m.mu.Unlock()
	if t != nil {
		t.close()
	}
}

// Shutdown stops every terminal.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	all := make([]*Terminal, 0, len(m.terms))
	for _, t := range m.terms {
		all = append(all, t)
	}
	m.terms = map[string]*Terminal{}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, t := range all {
		wg.Add(1)
		go func() { defer wg.Done(); t.close() }()
	}
	wg.Wait()
}

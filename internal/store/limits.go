package store

// Limits are a workspace's conversation limits (Configurações → Limites). A
// zero field takes the server's default.
type Limits struct {
	MaxHops         int `json:"maxHops,omitempty"`         // chained requests before conversations pause
	PerMinute       int `json:"perMinute,omitempty"`       // requests per minute
	MaxTeam         int `json:"maxTeam,omitempty"`         // flights a leader may create
	PortalsPerAgent int `json:"portalsPerAgent,omitempty"` // portals an agent may have created
}

func cloneLimits(l *Limits) *Limits {
	if l == nil {
		return nil
	}
	c := *l
	return &c
}

// Limits returns the workspace's saved limits (zero fields: defaults).
func (s *Store) Limits() Limits {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ws.Limits == nil {
		return Limits{}
	}
	return *s.ws.Limits
}

// SetLimits saves the workspace's limits.
func (s *Store) SetLimits(l Limits) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.ws.Limits
	s.ws.Limits = &l
	if err := s.save(); err != nil {
		s.ws.Limits = old
		return err
	}
	return nil
}

package store

import (
	"fmt"
	"slices"
)

// Summary engines: the agent CLIs that can write a summary, or "auto" (the
// summarized terminal's own agent, else the first one installed) and "off".
var SummaryEngines = []string{"auto", "claude", "devin", "codex", "gemini", "off"}

// SummaryConfig is how a workspace summarizes its agents' turns.
type SummaryConfig struct {
	Cards  bool   `json:"cards"`  // a card at every turn end
	Engine string `json:"engine"` // one of SummaryEngines
}

// DefaultSummaryConfig shows cards and summarizes with the agents' own CLIs.
func DefaultSummaryConfig() SummaryConfig { return SummaryConfig{Cards: true, Engine: "auto"} }

// SummaryConfig returns how turns are summarized.
func (s *Store) SummaryConfig() SummaryConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ws.SummaryConfig == nil {
		return DefaultSummaryConfig()
	}
	return *s.ws.SummaryConfig
}

// SetSummaryConfig changes how turns are summarized.
func (s *Store) SetSummaryConfig(c SummaryConfig) (SummaryConfig, error) {
	if c.Engine == "" {
		c.Engine = "auto"
	}
	if !slices.Contains(SummaryEngines, c.Engine) {
		return SummaryConfig{}, fmt.Errorf("%w: unknown summary engine %q", ErrInvalid, c.Engine)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.ws.SummaryConfig
	s.ws.SummaryConfig = &c
	if err := s.save(); err != nil {
		s.ws.SummaryConfig = old
		return SummaryConfig{}, err
	}
	return c, nil
}

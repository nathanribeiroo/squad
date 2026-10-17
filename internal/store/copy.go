package store

// A session may work in a copy of the repository (docs/PLAN.md S4): a git
// worktree on a branch of its own, made like a floor. Its terminals start
// there and its floors are made from it, while the workspace folder (Meta,
// shared by every session) stays the project's.

// Copy returns the copy the canvas works in, if it has one.
func (s *Store) Copy() (Floor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ws.Copy == nil {
		return Floor{}, false
	}
	return cloneFloor(*s.ws.Copy), true
}

// SetCopy records the copy the canvas works in (nil: the workspace folder).
func (s *Store) SetCopy(f *Floor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.ws.Copy
	s.ws.Copy = nil
	if f != nil {
		c := cloneFloor(*f)
		s.ws.Copy = &c
	}
	if err := s.save(); err != nil {
		s.ws.Copy = old
		return err
	}
	return nil
}

// UpdateCopy applies fn to the copy and saves.
func (s *Store) UpdateCopy(fn func(*Floor)) (Floor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws.Copy == nil {
		return Floor{}, ErrNotFound
	}
	old := s.ws.Copy
	c := cloneFloor(*old)
	fn(&c)
	s.ws.Copy = &c
	if err := s.save(); err != nil {
		s.ws.Copy = old
		return Floor{}, err
	}
	return cloneFloor(c), nil
}

// Workdir is the folder the canvas works in: the workspace folder inside its
// copy, or the workspace folder.
func (s *Store) Workdir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ws.Copy != nil {
		return s.ws.Copy.Cwd
	}
	return s.ws.Cwd
}

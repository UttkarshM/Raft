package structs

// SetStateMachine allows setting a custom state machine after server creation
func (s *Server) SetStateMachine(sm StateMachine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateMachine = sm
}

// GetStateMachine returns the current state machine
func (s *Server) GetStateMachine() StateMachine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stateMachine
}

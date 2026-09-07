package transport

// Hooks for accepting/shutdown state machine tests in transport_test.

func HookAccepting(s *Server) bool {
	return s.accepting.Load()
}

func HookClosed(s *Server) bool {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	return closed
}

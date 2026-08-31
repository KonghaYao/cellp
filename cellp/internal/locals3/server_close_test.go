package locals3

import "testing"

func TestCloseNilServer(t *testing.T) {
	var s *Server
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStartInvalidAddress(t *testing.T) {
	_, err := Start("not-a-valid-listen-addr", t.TempDir()+"/x.bolt")
	if err == nil {
		t.Fatal("expected error")
	}
}

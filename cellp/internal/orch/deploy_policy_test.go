package orch

import (
	"testing"
)

func TestDeployFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		lenient string
		strict  string
		want    bool
	}{
		{"default unset", "", "", true},
		{"lenient off", "0", "", true},
		{"lenient on", "1", "", false},
		{"strict only deprecated noop", "", "1", true},
		{"lenient wins over strict", "1", "1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CELLP_LENIENT_DEPLOY", tt.lenient)
			t.Setenv("CELLP_STRICT_OFFSHOOT_FORK", tt.strict)
			if got := deployFailClosed(); got != tt.want {
				t.Fatalf("deployFailClosed() = %v, want %v", got, tt.want)
			}
		})
	}
}

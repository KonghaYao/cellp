package config

import "testing"

func TestEffectiveIngressTierB(t *testing.T) {
	host := IngressTierHost
	dedicated := IngressTierDedicatedPort
	cases := []struct {
		global   string
		project  *string
		want     string
	}{
		{"host", nil, "host"},
		{"dedicated_port", nil, "dedicated_port"},
		{"host", &dedicated, "dedicated_port"},
		{"dedicated_port", &host, "host"},
		{"host", strPtr(""), "host"},
	}
	for _, tc := range cases {
		got := EffectiveIngressTierB(tc.global, tc.project)
		if got != tc.want {
			t.Fatalf("global=%q project=%v got %q want %q", tc.global, tc.project, got, tc.want)
		}
	}
}

func TestValidateIngressTierB(t *testing.T) {
	if err := ValidateIngressTierB("host"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIngressTierB("no-such-tier"); err == nil {
		t.Fatal("expected error")
	}
}

func strPtr(s string) *string { return &s }

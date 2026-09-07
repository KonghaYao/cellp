package runtime

import (
	"testing"
)

func TestSeedPortCollision(t *testing.T) {
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")

	if err := m.SeedPort("demo", "v1", 8803); err != nil {
		t.Fatalf("SeedPort v1: %v", err)
	}
	if err := m.SeedPort("demo", "v1", 8803); err != nil {
		t.Fatalf("SeedPort v1 again: %v", err)
	}
	if err := m.SeedPort("demo", "v2", 8803); err == nil {
		t.Fatal("expected collision when seeding same port for different version")
	}
	if err := m.SeedPort("demo", "v1", 8804); err == nil {
		t.Fatal("expected error when changing seeded port for same version")
	}
}

func TestLoadReconcileConfigDefaults(t *testing.T) {
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "")
	cfg := LoadReconcileConfig()
	if !cfg.Background {
		t.Fatal("expected background enabled by default")
	}
	if cfg.Interval != defaultReconcileInterval {
		t.Fatalf("interval = %v, want %v", cfg.Interval, defaultReconcileInterval)
	}
}

func TestLoadReconcileConfigBootOnly(t *testing.T) {
	t.Setenv("CELLP_FLEET_RECONCILE_INTERVAL", "0")
	cfg := LoadReconcileConfig()
	if cfg.Background {
		t.Fatal("expected background disabled when interval is 0")
	}
}

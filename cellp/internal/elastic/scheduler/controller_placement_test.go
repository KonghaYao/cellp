package scheduler

import (
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestVersionStatusBlocksElasticPlacement(t *testing.T) {
	for _, status := range []string{
		registry.StatusFailed,
		registry.StatusArchived,
		registry.StatusDestroyed,
		registry.StatusDraining,
	} {
		if !versionStatusBlocksElasticPlacement(status) {
			t.Fatalf("expected block for %s", status)
		}
	}
	if versionStatusBlocksElasticPlacement(registry.StatusReady) {
		t.Fatal("ready should not block placement")
	}
}

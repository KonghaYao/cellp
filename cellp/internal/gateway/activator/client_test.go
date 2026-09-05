package activator_test

import (
	"context"
	"testing"

	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

func TestRegistryEnsureClientIdempotent(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/act.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, err = store.CreateProject(ctx, registry.CreateProjectInput{ID: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "ver", ProjectID: "proj"})
	if err != nil {
		t.Fatal(err)
	}

	client := &activator.RegistryEnsureClient{Store: store}
	if err := client.EnsureCapacity(ctx, "proj", "ver", 1); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetServingDesire(ctx, "proj", "ver")
	if err != nil || d == nil || d.DesiredReplicas != 1 {
		t.Fatalf("desire %+v err %v", d, err)
	}
	if err := client.EnsureCapacity(ctx, "proj", "ver", 1); err != nil {
		t.Fatal(err)
	}
	d2, _ := store.GetServingDesire(ctx, "proj", "ver")
	if d2.Generation != d.Generation {
		t.Fatalf("idempotent ensure should not bump generation: %d -> %d", d.Generation, d2.Generation)
	}
}

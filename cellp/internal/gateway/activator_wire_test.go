package gateway

import (
	"context"
	"sync"
	"testing"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

type activatorTestGuard struct{}

func (activatorTestGuard) HoldsWriteLock(context.Context) error { return nil }

func TestElasticActivatorInitOnce(t *testing.T) {
	t.Setenv(contract.EnvElasticRuntime, "1")
	store, err := registry.Open(t.TempDir() + "/gw.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	g := NewWithConfig(store, GatewayConfig{})
	client := &activator.RegistryEnsureClient{Store: store, Guard: activatorTestGuard{}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := g.ConfigureElasticActivator(client, activator.DefaultConfig()); err != nil {
				t.Errorf("configure: %v", err)
			}
		}()
	}
	wg.Wait()
	a := g.elasticActivator()
	b := g.elasticActivator()
	if a == nil || a != b {
		t.Fatalf("expected one shared activator, a=%p b=%p", a, b)
	}
	if err := g.ShutdownElasticActivator(context.Background()); err != nil {
		t.Fatal(err)
	}
}

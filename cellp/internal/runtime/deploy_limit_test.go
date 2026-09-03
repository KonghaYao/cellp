package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCelldDeployConcurrencyDefault(t *testing.T) {
	t.Setenv("CELLP_CELLD_DEPLOY_CONCURRENCY", "")
	if celldDeployConcurrency() != 1 {
		t.Fatalf("got %d", celldDeployConcurrency())
	}
}

func TestCelldDeployConcurrencyEnv(t *testing.T) {
	t.Setenv("CELLP_CELLD_DEPLOY_CONCURRENCY", "2")
	if celldDeployConcurrency() != 2 {
		t.Fatalf("got %d", celldDeployConcurrency())
	}
}

func TestWithCelldDeploySlotSerializes(t *testing.T) {
	t.Setenv("CELLP_CELLD_DEPLOY_CONCURRENCY", "1")
	celldDeploySemOnce = sync.Once{}
	celldDeploySem = nil

	var inFlight int32
	var peak int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = withCelldDeploySlot(context.Background(), func(context.Context) error {
				cur := atomic.AddInt32(&inFlight, 1)
				for {
					p := atomic.LoadInt32(&peak)
					if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return nil
			})
		}()
	}
	wg.Wait()
	if peak != 1 {
		t.Fatalf("peak in-flight=%d want 1", peak)
	}
}

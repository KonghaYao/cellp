package registry

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// TestOpenConcurrentWhileStoreActive verifies a second Open(migrate) succeeds while another
// store handle performs representative writes on the same database file.
func TestOpenConcurrentWhileStoreActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent-open.sqlite")
	primary, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := time.Now().UTC().Add(time.Hour)
	seed := contract.RuntimeNode{
		NodeID:        "writer-node",
		CapacityUnits: 1,
		Generation:    1,
		LeaseExpiry:   exp,
		AgentBaseURL:  "https://127.0.0.1:1",
		IdentityURI:   "spiffe://cellp/test/writer",
		Zone:          "z",
	}
	if err := primary.UpsertRuntimeNode(ctx, seed); err != nil {
		t.Fatal(err)
	}

	writerErr := make(chan error, 1)
	var writerReady atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		writerReady.Store(true)
		for ctx.Err() == nil {
			seed.LeaseExpiry = time.Now().UTC().Add(time.Hour)
			if err := primary.UpsertRuntimeNode(ctx, seed); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				if !isBusy(err) {
					writerErr <- fmt.Errorf("UpsertRuntimeNode: %w", err)
					return
				}
			}
			if _, err := primary.ListRuntimeNodes(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				if !isBusy(err) {
					writerErr <- fmt.Errorf("ListRuntimeNodes: %w", err)
					return
				}
			}
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !writerReady.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !writerReady.Load() {
		t.Fatal("writer goroutine did not start")
	}

	const rounds = 12
	const parallelOpens = 4
	for i := 0; i < rounds; i++ {
		select {
		case err := <-writerErr:
			t.Fatal(err)
		default:
		}

		var openWG sync.WaitGroup
		openErrs := make(chan error, parallelOpens)
		var openStart sync.WaitGroup
		openStart.Add(1)
		for j := 0; j < parallelOpens; j++ {
			openWG.Add(1)
			go func(round, idx int) {
				defer openWG.Done()
				openStart.Wait()
				secondary, err := Open(path)
				if err != nil {
					openErrs <- fmt.Errorf("round %d open %d: %w", round, idx, err)
					return
				}
				defer secondary.Close()
				node, err := secondary.GetRuntimeNode(context.Background(), seed.NodeID)
				if err != nil {
					openErrs <- fmt.Errorf("round %d open %d GetRuntimeNode: %w", round, idx, err)
					return
				}
				if node == nil || node.NodeID != seed.NodeID {
					openErrs <- fmt.Errorf("round %d open %d: missing runtime node", round, idx)
				}
			}(i, j)
		}
		openStart.Done()
		openWG.Wait()
		close(openErrs)
		for err := range openErrs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	cancel()
	wg.Wait()
	select {
	case err := <-writerErr:
		t.Fatal(err)
	default:
	}
}

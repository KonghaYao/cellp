package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// nextRuntimeNodeGeneration picks the activation generation for a node lease fence.
func nextRuntimeNodeGeneration(existing *contract.RuntimeNode, now time.Time) (int64, error) {
	if existing == nil {
		return 1, nil
	}
	if existing.LeaseExpiry.After(now) {
		return 0, fmt.Errorf("runtime node generation active")
	}
	return existing.Generation + 1, nil
}

func releaseRuntimeNodeLeaseCAS(ctx context.Context, store registry.ServingStore, nodeID string, generation int64) error {
	if store == nil || generation <= 0 {
		return nil
	}
	return store.ReleaseRuntimeNodeLease(ctx, nodeID, generation)
}

func waitBackground(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func shutdownHTTPServer(ctx context.Context, srv *http.Server, done <-chan struct{}) error {
	if srv == nil {
		return nil
	}
	var errs []error
	if err := srv.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := waitBackground(ctx, done); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func markQuiescence(err error, quiesced *bool) {
	if err != nil {
		*quiesced = false
	}
}

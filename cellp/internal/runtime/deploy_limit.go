package runtime

import (
	"context"
	"os"
	"strconv"
	"sync"
)

const defaultCelldDeployConcurrency = 1

var (
	celldDeploySem     chan struct{}
	celldDeploySemOnce sync.Once
)

func celldDeployConcurrency() int {
	if v := os.Getenv("CELLP_CELLD_DEPLOY_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultCelldDeployConcurrency
}

func celldDeploySemaphore() chan struct{} {
	celldDeploySemOnce.Do(func() {
		n := celldDeployConcurrency()
		celldDeploySem = make(chan struct{}, n)
	})
	return celldDeploySem
}

// withCelldDeploySlot runs fn while holding a process-wide deploy slot.
// Limits concurrent `celld deploy` subprocesses so AD-1 fleets (~N celld daemons)
// plus esbuild/upload peaks do not trigger macOS SIGKILL under memory pressure.
func withCelldDeploySlot(ctx context.Context, fn func(context.Context) error) error {
	sem := celldDeploySemaphore()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-sem }()
	return fn(ctx)
}

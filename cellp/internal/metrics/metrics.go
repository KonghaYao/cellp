package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

var (
	pendingJobs     atomic.Int64
	routesActive    atomic.Int64
	celldHealthy    atomic.Int64
	celldUnhealthy  atomic.Int64
)

// Collect refreshes gauge values from the registry store and runtime manager.
func Collect(ctx context.Context, store registry.Store, rm *runtime.Manager) error {
	pending, err := store.CountPendingJobs(ctx)
	if err != nil {
		return err
	}
	pendingJobs.Store(int64(pending))

	routes, err := store.ListAllActiveRoutes(ctx)
	if err != nil {
		return err
	}
	routesActive.Store(int64(len(routes)))

	var healthy, unhealthy int64
	for _, rh := range rm.RuntimeHealth(ctx, routes) {
		if rh.Healthy {
			healthy++
		} else {
			unhealthy++
		}
	}
	celldHealthy.Store(healthy)
	celldUnhealthy.Store(unhealthy)
	return nil
}

// Handler serves Prometheus text exposition format (stdlib only).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "# HELP cellp_pending_jobs Pending orchestrator jobs.\n")
		fmt.Fprintf(w, "# TYPE cellp_pending_jobs gauge\n")
		fmt.Fprintf(w, "cellp_pending_jobs %d\n", pendingJobs.Load())
		fmt.Fprintf(w, "# HELP cellp_routes_active Active gateway routes.\n")
		fmt.Fprintf(w, "# TYPE cellp_routes_active gauge\n")
		fmt.Fprintf(w, "cellp_routes_active %d\n", routesActive.Load())
		fmt.Fprintf(w, "# HELP cellp_celld_healthy Healthy celld upstreams.\n")
		fmt.Fprintf(w, "# TYPE cellp_celld_healthy gauge\n")
		fmt.Fprintf(w, "cellp_celld_healthy %d\n", celldHealthy.Load())
		fmt.Fprintf(w, "# HELP cellp_celld_unhealthy Unhealthy celld upstreams.\n")
		fmt.Fprintf(w, "# TYPE cellp_celld_unhealthy gauge\n")
		fmt.Fprintf(w, "cellp_celld_unhealthy %d\n", celldUnhealthy.Load())
	})
}

// StartCollector launches a background metrics collection loop.
func StartCollector(ctx context.Context, store registry.Store, rm *runtime.Manager, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		log.Printf("metrics: collector every %v", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		collectAndLog(ctx, store, rm)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectAndLog(ctx, store, rm)
			}
		}
	}()
}

func collectAndLog(ctx context.Context, store registry.Store, rm *runtime.Manager) {
	if err := Collect(ctx, store, rm); err != nil {
		log.Printf("metrics: collect failed: %v", err)
	}
}

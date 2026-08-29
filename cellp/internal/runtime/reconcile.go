package runtime

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

const defaultReconcileInterval = 30 * time.Second

// ReconcileConfig controls background fleet reconciliation.
type ReconcileConfig struct {
	Interval   time.Duration
	Background bool // periodic ticker; boot reconcile always runs once
}

// LoadReconcileConfig reads CELLP_FLEET_RECONCILE_INTERVAL (default 30s, "0" = boot only).
func LoadReconcileConfig() ReconcileConfig {
	cfg := ReconcileConfig{Interval: defaultReconcileInterval, Background: true}
	v := os.Getenv("CELLP_FLEET_RECONCILE_INTERVAL")
	if v == "" {
		return cfg
	}
	if v == "0" {
		cfg.Background = false
		return cfg
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("fleet: invalid CELLP_FLEET_RECONCILE_INTERVAL %q, using %v", v, defaultReconcileInterval)
		return cfg
	}
	cfg.Interval = d
	return cfg
}

// ReconcileFleet restarts celld for active routes that are not healthy.
// Returns started count, skipped (already healthy) count, and error.
func ReconcileFleet(ctx context.Context, store registry.Store, m *Manager) (started, skipped int, err error) {
	if !CelldInstalled() {
		return 0, 0, nil
	}
	routes, err := store.ListAllActiveRoutes(ctx)
	if err != nil {
		return 0, 0, err
	}
	unhealthy := 0
	for _, route := range routes {
		host := route.UpstreamHost
		if host == "" {
			host = "127.0.0.1"
		}
		if m.routeManaged(route.ProjectID, route.VersionID, route.UpstreamPort) || m.Health(ctx, host, route.UpstreamPort) {
			if err := m.SeedPort(route.ProjectID, route.VersionID, route.UpstreamPort); err != nil {
				log.Printf("fleet: seed port %s/%s:%d: %v", route.ProjectID, route.VersionID, route.UpstreamPort, err)
			}
			skipped++
			continue
		}
		if err := m.SeedPort(route.ProjectID, route.VersionID, route.UpstreamPort); err != nil {
			log.Printf("fleet: seed port %s/%s: %v", route.ProjectID, route.VersionID, err)
			unhealthy++
			continue
		}
		if _, _, err := m.StartOnPort(ctx, route.ProjectID, route.VersionID, host, route.UpstreamPort); err != nil {
			log.Printf("fleet: restart %s/%s on %s:%d: %v", route.ProjectID, route.VersionID, host, route.UpstreamPort, err)
			unhealthy++
			continue
		}
		started++
	}
	if unhealthy > 0 {
		log.Printf("fleet: reconcile unhealthy routes=%d", unhealthy)
	}
	return started, skipped, nil
}

// StartReconciler runs ReconcileFleet on a ticker when Background is enabled.
func StartReconciler(ctx context.Context, store registry.Store, m *Manager, cfg ReconcileConfig) {
	if !cfg.Background {
		return
	}
	go func() {
		log.Printf("fleet: background reconcile every %v", cfg.Interval)
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if started, skipped, err := ReconcileFleet(ctx, store, m); err != nil {
					log.Printf("fleet: reconcile failed: %v", err)
				} else if started > 0 {
					log.Printf("fleet: reconcile started=%d skipped=%d", started, skipped)
				}
			}
		}
	}()
}

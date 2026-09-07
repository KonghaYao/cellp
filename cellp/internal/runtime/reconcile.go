package runtime

import (
	"log"
	"os"
	"time"
)

const defaultReconcileInterval = 30 * time.Second

// ReconcileConfig controls background fleet reconciliation interval (metrics collector).
type ReconcileConfig struct {
	Interval   time.Duration
	Background bool // periodic ticker; legacy fleet reconciler removed
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

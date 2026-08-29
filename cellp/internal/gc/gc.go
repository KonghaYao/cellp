package gc

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

const (
	defaultInterval  = time.Hour
	defaultRetention = 7 * 24 * time.Hour
)

// Config holds GC runtime settings from environment.
type Config struct {
	Interval  time.Duration
	Retention time.Duration
	Enabled   bool
}

// LoadConfig reads GC settings from environment.
//
// CELLP_GC_INTERVAL — tick interval (default 1h); "0" disables background GC.
// CELLP_GC_RETENTION_DAYS — retention for completed jobs and destroyed versions (default 7).
func LoadConfig() Config {
	cfg := Config{
		Interval:  defaultInterval,
		Retention: defaultRetention,
		Enabled:   true,
	}
	if v := os.Getenv("CELLP_GC_INTERVAL"); v != "" {
		if v == "0" {
			cfg.Enabled = false
			return cfg
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Printf("gc: invalid CELLP_GC_INTERVAL %q, using %v", v, defaultInterval)
		} else {
			cfg.Interval = d
		}
	}
	if v := os.Getenv("CELLP_GC_RETENTION_DAYS"); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 {
			log.Printf("gc: invalid CELLP_GC_RETENTION_DAYS %q, using 7", v)
		} else {
			cfg.Retention = time.Duration(days) * 24 * time.Hour
		}
	}
	return cfg
}

// Result summarizes a single GC pass.
type Result struct {
	JobsDeleted     int64
	VersionsDeleted int64
}

// RunOnce purges stale jobs and destroyed version metadata.
func RunOnce(ctx context.Context, store registry.Store, retention time.Duration) (Result, error) {
	cutoff := time.Now().UTC().Add(-retention)
	var res Result

	jobs, err := store.PurgeCompletedJobs(ctx, cutoff)
	if err != nil {
		return res, err
	}
	res.JobsDeleted = jobs

	versions, err := store.PurgeDestroyedVersions(ctx, cutoff)
	if err != nil {
		return res, err
	}
	res.VersionsDeleted = versions
	return res, nil
}

// Start launches a background GC loop; no-op when cfg.Enabled is false.
func Start(ctx context.Context, store registry.Store, cfg Config) {
	if !cfg.Enabled {
		log.Println("gc: background GC disabled (CELLP_GC_INTERVAL=0)")
		return
	}
	go func() {
		log.Printf("gc: background GC every %v, retention %v", cfg.Interval, cfg.Retention)
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		runAndLog(ctx, store, cfg.Retention)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runAndLog(ctx, store, cfg.Retention)
			}
		}
	}()
}

func runAndLog(ctx context.Context, store registry.Store, retention time.Duration) {
	res, err := RunOnce(ctx, store, retention)
	if err != nil {
		log.Printf("gc: purge failed: %v", err)
		return
	}
	if res.JobsDeleted > 0 || res.VersionsDeleted > 0 {
		log.Printf("gc: purged jobs=%d versions=%d", res.JobsDeleted, res.VersionsDeleted)
	}
}

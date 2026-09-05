package autoscaler

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

const defaultInterval = 30 * time.Second

// Config controls the background autoscaler ticker.
type Config struct {
	Interval   time.Duration
	Background bool
}

// LoadConfig reads CELLP_AUTOSCALER_INTERVAL (default 30s, "0" = disabled background).
func LoadConfig() Config {
	cfg := Config{Interval: defaultInterval, Background: true}
	v := os.Getenv("CELLP_AUTOSCALER_INTERVAL")
	if v == "" {
		return cfg
	}
	if v == "0" {
		cfg.Background = false
		return cfg
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("autoscaler: invalid CELLP_AUTOSCALER_INTERVAL %q, using %v", v, defaultInterval)
		return cfg
	}
	cfg.Interval = d
	return cfg
}

// VersionGap compares desired vs ready for one version.
type VersionGap struct {
	ProjectID       string
	VersionID       string
	DesiredReplicas int
	ReadyReplicas   int
	Gap             int // desired - ready
	Reason          string
}

// TickReport is the outcome of one autoscaler pass.
type TickReport struct {
	Skipped bool
	Gaps    []VersionGap
}

// Loop runs periodic desired-vs-ready reconciliation (stub until WP-SCALE algorithm).
type Loop struct {
	Store Store
	// BackgroundGuard is applied when reading policy bounds.
	BackgroundGuard contract.BackgroundGuardOptions
}

// Enabled reports whether elastic autoscaler logic may run.
func Enabled() bool {
	return contract.ElasticRuntimeEnabled()
}

// Tick reads policies and desires, compares desired vs ready replica counts.
// When CELLP_ELASTIC_RUNTIME=0 this is a no-op (Skipped=true).
func (l *Loop) Tick(ctx context.Context) (TickReport, error) {
	if !Enabled() {
		return TickReport{Skipped: true}, nil
	}
	if l == nil || l.Store == nil {
		return TickReport{}, nil
	}
	policies, err := l.Store.ListElasticServingPolicies(ctx)
	if err != nil {
		return TickReport{}, err
	}
	var gaps []VersionGap
	for _, pol := range policies {
		if err := contract.ValidateServingPolicyBackground(contract.ServingPolicy{
			Revision:        pol.Revision,
			MinReplicas:     pol.MinReplicas,
			MaxReplicas:     pol.MaxReplicas,
			Priority:        pol.Priority,
			BackgroundMode:  pol.BackgroundMode,
			ElasticEnrolled: pol.ElasticEnrolled,
		}, l.BackgroundGuard); err != nil {
			log.Printf("autoscaler: skip %s/%s policy guard: %v", pol.ProjectID, pol.VersionID, err)
			continue
		}
		desire, err := l.Store.GetServingDesire(ctx, pol.ProjectID, pol.VersionID)
		if err != nil {
			return TickReport{}, err
		}
		desired := pol.MinReplicas
		reason := "policy_min"
		if desire != nil {
			desired = desire.DesiredReplicas
			reason = desire.Reason
		}
		if desired < pol.MinReplicas {
			desired = pol.MinReplicas
			reason = "clamp_min"
		}
		if desired > pol.MaxReplicas {
			desired = pol.MaxReplicas
			reason = "clamp_max"
		}
		replicas, err := l.Store.ListRuntimeReplicas(ctx, pol.ProjectID, pol.VersionID)
		if err != nil {
			return TickReport{}, err
		}
		ready := CountReadyReplicas(replicas)
		gap := desired - ready
		if gap != 0 {
			gaps = append(gaps, VersionGap{
				ProjectID:       pol.ProjectID,
				VersionID:       pol.VersionID,
				DesiredReplicas: desired,
				ReadyReplicas:   ready,
				Gap:             gap,
				Reason:          reason,
			})
		}
	}
	return TickReport{Gaps: gaps}, nil
}

// Run executes Tick on interval until ctx is cancelled. No-op when Background=false.
func Run(ctx context.Context, store Store, cfg Config, guard contract.BackgroundGuardOptions) {
	if !cfg.Background {
		return
	}
	loop := &Loop{Store: store, BackgroundGuard: guard}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !Enabled() {
				continue
			}
			if _, err := loop.Tick(ctx); err != nil {
				log.Printf("autoscaler tick: %v", err)
			}
		}
	}
}

// Start begins the background loop when cfg.Background is set (per-tick still no-op if flag=0).
func Start(ctx context.Context, store registry.ServingStore, cfg Config) {
	if !cfg.Background {
		return
	}
	go Run(ctx, RegistryStore{ServingStore: store}, cfg, contract.BackgroundGuardOptions{})
}

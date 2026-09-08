package scheduler

import (
	"context"
	"log"
	"os"
	"time"
)

const defaultInterval = 5 * time.Second

// Config controls the background scheduler ticker.
type Config struct {
	Interval   time.Duration
	Background bool
}

// LoadConfig reads CELLP_SCHEDULER_INTERVAL (default 5s, "0" = disabled background).
func LoadConfig() Config {
	cfg := Config{Interval: defaultInterval, Background: true}
	v := os.Getenv("CELLP_SCHEDULER_INTERVAL")
	if v == "" {
		return cfg
	}
	if v == "0" {
		cfg.Background = false
		return cfg
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("scheduler: invalid CELLP_SCHEDULER_INTERVAL %q, using %v", v, defaultInterval)
		return cfg
	}
	cfg.Interval = d
	return cfg
}

// Run executes Tick on interval until ctx is cancelled. Fatal guard errors stop the loop.
func Run(ctx context.Context, ctrl *Controller, cfg Config, errCh chan<- error) {
	if ctrl == nil {
		return
	}
	tick := func() bool {
		if _, err := ctrl.Tick(ctx); err != nil {
			if ctx.Err() != nil {
				return false
			}
			if TickFatal(err) {
				if errCh != nil {
					errCh <- err
				}
				return false
			}
			if !IsTransientAgentOrRegistry(err) {
				log.Printf("scheduler tick: %v", err)
			}
		}
		return true
	}
	if !tick() {
		return
	}
	if !cfg.Background {
		return
	}
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
			if !tick() {
				return
			}
		}
	}
}

// Start begins the scheduler loop when cfg.Background is set.
func Start(ctx context.Context, ctrl *Controller, cfg Config, errCh chan<- error) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, ctrl, cfg, errCh)
	}()
	return done
}

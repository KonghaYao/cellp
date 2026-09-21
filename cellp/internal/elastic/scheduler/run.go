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
	// Renewals run on their own lane: a pass can block for the length of an agent call,
	// and an assignment lease may never outlive its node lease, so renewals inside the
	// pass would let healthy replicas expire behind any slow start. The lane is cancelled
	// on return, so a foreground (Background=false) run exits after its single tick
	// instead of waiting for a loop that no one stops.
	renewCtx, stopRenew := context.WithCancel(ctx)
	defer stopRenew()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		runAssignmentRenewals(renewCtx, ctrl)
	}()
	defer func() {
		stopRenew()
		<-renewDone
	}()
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

// runAssignmentRenewals renews live assignments on a fixed interval, independent of the
// serialized pass. Expired or fenced assignments are left to the pass.
func runAssignmentRenewals(ctx context.Context, ctrl *Controller) {
	ticker := time.NewTicker(assignmentRenewDuringAgentInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !Enabled() {
				continue
			}
			if err := ctrl.RenewAssignments(ctx); err != nil {
				if ctx.Err() != nil || GuardLostFatal(err) {
					return
				}
				if !IsTransientAgentOrRegistry(err) {
					log.Printf("scheduler renewal: %v", err)
				}
			}
		}
	}
}

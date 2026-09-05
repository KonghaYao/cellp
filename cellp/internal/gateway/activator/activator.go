package activator

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// Config holds activator limits (values are not frozen; SP-E6 calibrates).
type Config struct {
	MaxBufferedBodyBytes int64
	WakeTimeout          time.Duration
	RetryAfterSec        int
	PollInterval         time.Duration
	GlobalWaitBudget     int
	PerVersionWaitBudget int
}

// DefaultConfig returns safe defaults for dev/E2.
func DefaultConfig() Config {
	return Config{
		MaxBufferedBodyBytes: 64 * 1024,
		WakeTimeout:          30 * time.Second,
		RetryAfterSec:        1,
		PollInterval:         100 * time.Millisecond,
		GlobalWaitBudget:     256,
		PerVersionWaitBudget: 32,
	}
}

// EndpointLookup returns true when a warm upstream is available.
type EndpointLookup func() (upstream string, ok bool)

// Activator merges cold starts and calls EnsureCapacity (AD-15 E2 / WP-GW-ACT).
type Activator struct {
	enabled bool
	cfg     Config
	client  EnsureCapacityClient
	budget  *Budget
	group   Group
}

// New builds an activator. When enabled is false, Admit is a no-op allow.
func New(enabled bool, client EnsureCapacityClient, cfg Config) *Activator {
	if cfg.MaxBufferedBodyBytes <= 0 {
		cfg.MaxBufferedBodyBytes = DefaultConfig().MaxBufferedBodyBytes
	}
	if cfg.WakeTimeout <= 0 {
		cfg.WakeTimeout = DefaultConfig().WakeTimeout
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultConfig().PollInterval
	}
	if cfg.RetryAfterSec <= 0 {
		cfg.RetryAfterSec = DefaultConfig().RetryAfterSec
	}
	return &Activator{
		enabled: enabled,
		cfg:     cfg,
		client:  client,
		budget:  NewBudget(cfg.GlobalWaitBudget, cfg.PerVersionWaitBudget),
	}
}

// Enabled reports whether elastic activator logic is active.
func (a *Activator) Enabled() bool {
	return a != nil && a.enabled
}

// Admit handles deploy_ready+cold when elastic runtime is on.
func (a *Activator) Admit(ctx context.Context, r *http.Request, projectID, versionID, versionStatus string, desiredGeneration int64, lookup EndpointLookup) AdmitResult {
	if a == nil || !a.enabled {
		return AdmitResult{AllowProxy: true}
	}
	if versionStatus == contract.StatusArchived {
		return AdmitResult{AllowProxy: false, Reason: ReasonVersionArchived, RetryAfterSec: a.cfg.RetryAfterSec}
	}
	if versionStatus != contract.StatusDeployReady {
		return AdmitResult{AllowProxy: true}
	}
	if lookup != nil {
		if _, ok := lookup(); ok {
			return AdmitResult{AllowProxy: true}
		}
	}

	waitClass := ClassifyRequest(r, a.cfg.MaxBufferedBodyBytes)
	key := singleflightKey(projectID, versionID, desiredGeneration)

	if waitClass == WaitClassFastFail {
		a.triggerEnsure(key, projectID, versionID)
		return AdmitResult{AllowProxy: false, Reason: ReasonWakeRetry, RetryAfterSec: a.cfg.RetryAfterSec}
	}

	if !a.budget.TryAcquire(projectID, versionID) {
		return AdmitResult{AllowProxy: false, Reason: ReasonWakeQueueFull, RetryAfterSec: a.cfg.RetryAfterSec}
	}
	defer a.budget.Release(projectID, versionID)

	deadline := a.cfg.WakeTimeout
	if d, ok := ctx.Deadline(); ok {
		remaining := time.Until(d)
		if remaining > 0 && remaining < deadline {
			deadline = remaining
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	_, err, _ := a.group.Do(key, func() (interface{}, error) {
		if err := a.client.EnsureCapacity(waitCtx, projectID, versionID, 1); err != nil {
			return nil, err
		}
		return a.pollEndpoint(waitCtx, lookup), nil
	})

	if err != nil {
		return AdmitResult{AllowProxy: false, Reason: ReasonControlUnavailable, RetryAfterSec: a.cfg.RetryAfterSec}
	}

	if lookup != nil {
		if _, ok := lookup(); ok {
			return AdmitResult{AllowProxy: true}
		}
	}
	return AdmitResult{AllowProxy: false, Reason: ReasonWakeTimeout, RetryAfterSec: a.cfg.RetryAfterSec}
}

func (a *Activator) triggerEnsure(key, projectID, versionID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _, _ = a.group.Do(key, func() (interface{}, error) {
			return nil, a.client.EnsureCapacity(ctx, projectID, versionID, 1)
		})
	}()
}

func (a *Activator) pollEndpoint(ctx context.Context, lookup EndpointLookup) bool {
	if lookup == nil {
		return false
	}
	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if _, ok := lookup(); ok {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func singleflightKey(projectID, versionID string, desiredGeneration int64) string {
	return fmt.Sprintf("%s/%s/%d", projectID, versionID, desiredGeneration)
}

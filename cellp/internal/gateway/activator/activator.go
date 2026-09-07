package activator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// Config holds activator limits (values are not frozen; SP-E6 calibrates).
type Config struct {
	MaxBufferedBodyBytes        int64
	WakeTimeout                 time.Duration
	RetryAfterSec               int
	PollInterval                time.Duration
	GlobalWaitBudget            int
	PerVersionWaitBudget        int
	GlobalPendingBytes          int64
	PerVersionPendingBytes      int64
	GlobalActivationFlights     int
	PerVersionActivationFlights int
}

// DefaultConfig returns bounded defaults for dev/E2; SP-E6 must calibrate production values.
func DefaultConfig() Config {
	return Config{
		MaxBufferedBodyBytes:        64 * 1024,
		WakeTimeout:                 30 * time.Second,
		RetryAfterSec:               1,
		PollInterval:                100 * time.Millisecond,
		GlobalWaitBudget:            256,
		PerVersionWaitBudget:        32,
		GlobalPendingBytes:          defaultGlobalPendingBytes,
		PerVersionPendingBytes:      defaultPerVersionPendingBytes,
		GlobalActivationFlights:     64,
		PerVersionActivationFlights: 1,
	}
}

// EndpointLookup returns a ready upstream from the immutable Gateway snapshot.
type EndpointLookup func() (upstream string, ok bool)

// Activator merges cold starts and calls EnsureCapacity (AD-15 E2 / WP-GW-ACT).
type Activator struct {
	enabled bool
	cfg     Config
	client  EnsureCapacityClient
	budget  *Budget
	flights *FlightLimiter
	group   Group
	workCtx context.Context
	cancel  context.CancelFunc
}

// New builds an activator. When enabled is false, Admit is a no-op allow.
func New(enabled bool, client EnsureCapacityClient, cfg Config) *Activator {
	cfg = normalizeConfig(cfg)
	workCtx, cancel := context.WithCancel(context.Background())
	return &Activator{
		enabled: enabled,
		cfg:     cfg,
		client:  client,
		budget: NewBudgetWithBytes(
			cfg.GlobalWaitBudget,
			cfg.PerVersionWaitBudget,
			cfg.GlobalPendingBytes,
			cfg.PerVersionPendingBytes,
		),
		flights: NewFlightLimiter(cfg.GlobalActivationFlights, cfg.PerVersionActivationFlights),
		workCtx: workCtx,
		cancel:  cancel,
	}
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.MaxBufferedBodyBytes <= 0 {
		cfg.MaxBufferedBodyBytes = defaults.MaxBufferedBodyBytes
	}
	if cfg.WakeTimeout <= 0 {
		cfg.WakeTimeout = defaults.WakeTimeout
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaults.PollInterval
	}
	if cfg.RetryAfterSec <= 0 {
		cfg.RetryAfterSec = defaults.RetryAfterSec
	}
	if cfg.GlobalWaitBudget <= 0 {
		cfg.GlobalWaitBudget = defaults.GlobalWaitBudget
	}
	if cfg.PerVersionWaitBudget <= 0 {
		cfg.PerVersionWaitBudget = defaults.PerVersionWaitBudget
	}
	if cfg.GlobalPendingBytes <= 0 {
		cfg.GlobalPendingBytes = defaults.GlobalPendingBytes
	}
	if cfg.PerVersionPendingBytes <= 0 {
		cfg.PerVersionPendingBytes = defaults.PerVersionPendingBytes
	}
	if cfg.GlobalActivationFlights <= 0 {
		cfg.GlobalActivationFlights = defaults.GlobalActivationFlights
	}
	if cfg.PerVersionActivationFlights <= 0 {
		cfg.PerVersionActivationFlights = defaults.PerVersionActivationFlights
	}
	return cfg
}

// Enabled reports whether elastic activator logic is active.
func (a *Activator) Enabled() bool {
	return a != nil && a.enabled
}

// Shutdown rejects new activation work, cancels shared flights, and proves quiescence.
func (a *Activator) Shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if a.cancel != nil {
		a.cancel()
	}
	return a.group.Shutdown(ctx)
}

// Admit handles deploy_ready+cold when elastic runtime is on.
func (a *Activator) Admit(ctx context.Context, r *http.Request, projectID, versionID, versionStatus string, desiredGeneration int64, lookup EndpointLookup) AdmitResult {
	if a == nil || !a.enabled {
		return AdmitResult{AllowProxy: true}
	}
	if versionStatus == contract.StatusArchived {
		return a.reject(ReasonVersionArchived)
	}
	if versionStatus != contract.StatusDeployReady {
		return AdmitResult{AllowProxy: true}
	}
	if upstream, ok := lookupEndpoint(lookup); ok {
		return AdmitResult{AllowProxy: true, Upstream: upstream}
	}
	if a.client == nil || a.workCtx == nil || a.workCtx.Err() != nil {
		return a.reject(ReasonControlUnavailable)
	}

	waitClass := ClassifyRequest(r, a.cfg.MaxBufferedBodyBytes)
	key := singleflightKey(projectID, versionID, desiredGeneration)
	if waitClass == WaitClassBounded {
		reserved, bodyCancel, err := BufferBoundedBody(r, a.cfg.MaxBufferedBodyBytes)
		if err != nil {
			if errors.Is(err, ErrBodyTooLarge) {
				return a.reject(ReasonRequestTooLarge)
			}
			return a.reject(ReasonControlUnavailable)
		}
		defer bodyCancel()
		if !a.budget.TryAcquireBytes(projectID, versionID, reserved) {
			return a.reject(ReasonWakeQueueFull)
		}
		defer a.budget.ReleaseBytes(projectID, versionID, reserved)

		waitCtx, cancel := boundedCallerContext(ctx, a.cfg.WakeTimeout)
		defer cancel()
		value, err, _ := a.group.Do(waitCtx, key, func() (interface{}, error) {
			if !a.flights.TryAcquire(projectID, versionID) {
				return "", errFlightAdmissionDenied
			}
			defer a.flights.Release(projectID, versionID)
			activationCtx, cancelActivation := context.WithTimeout(a.workCtx, a.cfg.WakeTimeout)
			defer cancelActivation()
			if err := a.client.EnsureCapacity(activationCtx, projectID, versionID, 1); err != nil {
				return "", err
			}
			return a.pollEndpoint(activationCtx, lookup), nil
		})
		return a.finishAdmit(value, err)
	}

	if !a.beginSharedActivation(key, projectID, versionID, lookup) {
		return a.reject(ReasonWakeQueueFull)
	}
	return a.reject(ReasonWakeRetry)
}

func (a *Activator) finishAdmit(value interface{}, err error) AdmitResult {
	if err != nil {
		if errors.Is(err, errFlightAdmissionDenied) {
			return a.reject(ReasonWakeQueueFull)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return a.reject(ReasonWakeTimeout)
		}
		if errors.Is(err, ErrActivationCapacityUnavailable) {
			return a.reject(ReasonCapacityExhausted)
		}
		if errors.Is(err, ErrActivationNotQualified) {
			return a.reject(ReasonVersionNotReady)
		}
		if errors.Is(err, ErrActivationNotEligible) {
			return a.reject(ReasonVersionNotReady)
		}
		if errors.Is(err, errGroupClosed) {
			return a.reject(ReasonControlUnavailable)
		}
		return a.reject(ReasonControlUnavailable)
	}
	if upstream, ok := value.(string); ok && upstream != "" {
		return AdmitResult{AllowProxy: true, Upstream: upstream}
	}
	return a.reject(ReasonWakeTimeout)
}

var errFlightAdmissionDenied = errors.New("activator: flight admission denied")

func (a *Activator) reject(reason string) AdmitResult {
	return AdmitResult{Reason: reason, RetryAfterSec: a.cfg.RetryAfterSec}
}

func (a *Activator) beginSharedActivation(key, projectID, versionID string, lookup EndpointLookup) bool {
	newFlight := !a.group.Has(key)
	if newFlight && !a.flights.TryAcquire(projectID, versionID) {
		return false
	}
	err := a.group.Start(key, func() (interface{}, error) {
		if newFlight {
			defer a.flights.Release(projectID, versionID)
		}
		ctx, cancel := context.WithTimeout(a.workCtx, a.cfg.WakeTimeout)
		defer cancel()
		if err := a.client.EnsureCapacity(ctx, projectID, versionID, 1); err != nil {
			return nil, err
		}
		_ = a.pollEndpoint(ctx, lookup)
		return nil, nil
	})
	if err != nil {
		if newFlight {
			a.flights.Release(projectID, versionID)
		}
		return false
	}
	return true
}

func (a *Activator) pollEndpoint(ctx context.Context, lookup EndpointLookup) string {
	if lookup == nil {
		return ""
	}
	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if upstream, ok := lookupEndpoint(lookup); ok {
			return upstream
		}
		select {
		case <-ctx.Done():
			return ""
		case <-ticker.C:
		}
	}
}

func lookupEndpoint(lookup EndpointLookup) (string, bool) {
	if lookup == nil {
		return "", false
	}
	upstream, ok := lookup()
	return upstream, ok && upstream != ""
}

func pendingBodyBytes(r *http.Request) int64 {
	if r == nil || r.Method == http.MethodGet || r.Method == http.MethodHead || r.ContentLength < 0 {
		return 0
	}
	return r.ContentLength
}

func boundedCallerContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, timeout)
}

func singleflightKey(projectID, versionID string, desiredGeneration int64) string {
	return fmt.Sprintf("%s/%s/%d", projectID, versionID, desiredGeneration)
}

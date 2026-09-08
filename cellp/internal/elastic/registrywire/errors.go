package registrywire

import (
	"errors"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// ErrRegistryUnavailable marks transient controller/registry reachability failures.
var ErrRegistryUnavailable = errors.New("registry_unavailable")

// AuthoritativeErr binds a normalized wire reason to registry sentinels for errors.Is after remote calls.
type AuthoritativeErr struct {
	Reason contract.ReasonCode
}

func (e *AuthoritativeErr) Error() string { return string(e.Reason) }

// Is lets local and remote agent paths treat generation_stale / lease_expired identically.
func (e *AuthoritativeErr) Is(target error) bool {
	switch e.Reason {
	case contract.ReasonGenerationStale:
		return errors.Is(target, registry.ErrObservationStale) ||
			errors.Is(target, registry.ErrNodeLeaseCASConflict)
	case contract.ReasonLeaseExpired:
		return errors.Is(target, registry.ErrLeaseExpired)
	case contract.ReasonRuntimeNodeNotFound:
		return errors.Is(target, registry.ErrRuntimeNodeNotFound)
	case contract.ReasonRuntimeReplicaNotFound:
		return errors.Is(target, registry.ErrRuntimeReplicaNotFound)
	default:
		return false
	}
}

// AuthoritativeFromReason returns a sentinel error for a known authoritative wire reason.
func AuthoritativeFromReason(reason contract.ReasonCode) error {
	switch reason {
	case contract.ReasonGenerationStale,
		contract.ReasonLeaseExpired,
		contract.ReasonRuntimeNodeNotFound,
		contract.ReasonRuntimeReplicaNotFound:
		return &AuthoritativeErr{Reason: reason}
	default:
		return nil
	}
}

// IsAuthoritativeGenerationStale reports confirmed stale generation (local registry or remote wire).
func IsAuthoritativeGenerationStale(err error) bool {
	if err == nil {
		return false
	}
	var auth *AuthoritativeErr
	if errors.As(err, &auth) && auth.Reason == contract.ReasonGenerationStale {
		return true
	}
	if errors.Is(err, registry.ErrNodeLeaseCASConflict) {
		return true
	}
	return false
}

// IsAuthoritativeLeaseExpired reports confirmed lease expiry from remote wire (AuthoritativeErr).
// Bare registry.ErrLeaseExpired from local observation CAS is not authoritative assignment loss.
func IsAuthoritativeLeaseExpired(err error) bool {
	if err == nil {
		return false
	}
	var auth *AuthoritativeErr
	return errors.As(err, &auth) && auth.Reason == contract.ReasonLeaseExpired
}

// IsRegistryUnavailable reports transient registry/controller faults.
func IsRegistryUnavailable(err error) bool {
	return errors.Is(err, ErrRegistryUnavailable)
}

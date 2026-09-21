package agent

import (
	"errors"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

// ErrReconcileAuthority marks a reconcile that could not prove it still owns its
// assignments: the node read its own leases and found them gone. A bare sentinel is not
// proof of that (record CAS races are benign and stay retryable); only the read that
// establishes authority may speak for it, so the read path adds this marker.
var ErrReconcileAuthority = errors.New("reconcile authority unavailable")

// ErrReconcileAuthorityFatal reports whether err is a read that lost the node's authority.
func ErrReconcileAuthorityFatal(err error) bool {
	return errors.Is(err, ErrReconcileAuthority)
}

// ReconcileNodeErrorFatal reports whether ReconcileNode loss requires taking the node offline.
// Transient registry read/write failures return false so callers can retry without stopping healthy processes.
func ReconcileNodeErrorFatal(err error) bool {
	if err == nil {
		return false
	}
	if registryrelay.IsRegistryUnavailable(err) || registrywire.IsRegistryUnavailable(err) {
		return false
	}
	if errors.Is(err, ErrReconcileAuthority) {
		return true
	}
	if registrywire.IsAuthoritativeGenerationStale(err) || registrywire.IsAuthoritativeLeaseExpired(err) {
		return true
	}
	if errors.Is(err, registry.ErrNodeLeaseCASConflict) {
		return true
	}
	if ReconcileRecordBenign(err) {
		return false
	}
	var cmd *CommandError
	if errors.As(err, &cmd) && cmd.Reason == contract.ReasonGenerationStale {
		return true
	}
	return false
}

// ReconcileRecordBenign reports local observation CAS races during reconcile cleanup/record.
// These are retried on the next reconcile tick and must not take the node offline.
func ReconcileRecordBenign(err error) bool {
	if err == nil {
		return false
	}
	if registrywire.IsAuthoritativeGenerationStale(err) || registrywire.IsAuthoritativeLeaseExpired(err) {
		return false
	}
	return errors.Is(err, registry.ErrObservationStale) ||
		errors.Is(err, registry.ErrReplicaTransitionInvalid) ||
		errors.Is(err, registry.ErrLeaseExpired)
}

// RuntimeNodeHeartbeatFatal reports whether a heartbeat lease renew error means this generation lost ownership.
func RuntimeNodeHeartbeatFatal(err error) bool {
	if err == nil {
		return false
	}
	return registrywire.IsAuthoritativeGenerationStale(err) || registrywire.IsAuthoritativeLeaseExpired(err) ||
		errors.Is(err, registry.ErrNodeLeaseCASConflict) || errors.Is(err, registry.ErrLeaseExpired)
}

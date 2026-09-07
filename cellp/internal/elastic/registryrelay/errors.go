package registryrelay

import (
	"errors"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

// ErrRegistryUnavailable marks transient controller/registry reachability failures.
var ErrRegistryUnavailable = registrywire.ErrRegistryUnavailable

// RelayError carries a contract reason for registry relay wire mapping.
type RelayError struct {
	Reason  contract.ReasonCode
	Message string
}

func (e *RelayError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Reason)
}

// MapRegistryError maps durable registry errors to relay reasons (never maps uncertainty to absence).
func MapRegistryError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, registry.ErrObservationStale),
		errors.Is(err, registry.ErrNodeLeaseCASConflict):
		return &RelayError{Reason: contract.ReasonGenerationStale, Message: string(contract.ReasonGenerationStale)}
	case errors.Is(err, registry.ErrLeaseExpired):
		return &RelayError{Reason: contract.ReasonLeaseExpired, Message: string(contract.ReasonLeaseExpired)}
	case errors.Is(err, registry.ErrAgentCommandConflict),
		errors.Is(err, registry.ErrAgentCommandInProgress),
		errors.Is(err, registry.ErrAssignmentCASConflict),
		errors.Is(err, registry.ErrReplicaTransitionInvalid):
		return &RelayError{Reason: contract.ReasonConflict, Message: string(contract.ReasonConflict)}
	case errors.Is(err, registry.ErrRuntimeNodeNotFound):
		return &RelayError{Reason: contract.ReasonRuntimeNodeNotFound, Message: string(contract.ReasonRuntimeNodeNotFound)}
	case errors.Is(err, registry.ErrRuntimeReplicaNotFound):
		return &RelayError{Reason: contract.ReasonRuntimeReplicaNotFound, Message: string(contract.ReasonRuntimeReplicaNotFound)}
	case errors.Is(err, registry.ErrControllerGuardHeld):
		return errors.Join(ErrRegistryUnavailable, err)
	default:
		return errors.Join(ErrRegistryUnavailable, err)
	}
}

// IsRegistryUnavailable reports transient relay/controller faults.
func IsRegistryUnavailable(err error) bool {
	return registrywire.IsRegistryUnavailable(err)
}

// MapClientError restores registry authoritative sentinels from relay client/transport errors.
func MapClientError(err error) error {
	if err == nil {
		return nil
	}
	if IsRegistryUnavailable(err) {
		return err
	}
	var relay *RelayError
	if errors.As(err, &relay) && relay.Reason != "" {
		if auth := registrywire.AuthoritativeFromReason(relay.Reason); auth != nil {
			return auth
		}
		return relay
	}
	var auth *registrywire.AuthoritativeErr
	if errors.As(err, &auth) {
		return err
	}
	return err
}

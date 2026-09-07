package nodereg

import (
	"errors"
	"net/http"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

// ErrRegistryUnavailable marks transient controller/registry reachability failures.
var ErrRegistryUnavailable = registrywire.ErrRegistryUnavailable

// HandlerError carries a contract reason for node registration wire mapping.
type HandlerError struct {
	Reason     contract.ReasonCode
	Message    string
	HTTPStatus int
}

func (e *HandlerError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Reason)
}

// MapHandlerError maps registry failures to contract reason codes for transport.
func MapHandlerError(err error) error {
	if err == nil {
		return nil
	}
	var he *HandlerError
	if errors.As(err, &he) {
		return he
	}
	switch {
	case errors.Is(err, registry.ErrNodeLeaseCASConflict):
		return &HandlerError{Reason: contract.ReasonGenerationStale, Message: string(contract.ReasonGenerationStale)}
	case errors.Is(err, registry.ErrLeaseExpired):
		return &HandlerError{Reason: contract.ReasonLeaseExpired, Message: string(contract.ReasonLeaseExpired)}
	case errors.Is(err, registry.ErrControllerGuardHeld):
		return errors.Join(ErrRegistryUnavailable, err)
	default:
		return errors.Join(ErrRegistryUnavailable, err)
	}
}

// HandlerHTTPStatus returns the wire HTTP status for a handler error.
func HandlerHTTPStatus(he *HandlerError) int {
	if he == nil {
		return http.StatusServiceUnavailable
	}
	if he.HTTPStatus != 0 {
		return he.HTTPStatus
	}
	return noderegReasonHTTPStatus(he.Reason)
}

// IsRegistryUnavailable reports transient controller/registry reachability failures.
func IsRegistryUnavailable(err error) bool {
	return registrywire.IsRegistryUnavailable(err)
}

func noderegReasonHTTPStatus(reason contract.ReasonCode) int {
	switch reason {
	case contract.ReasonLeaseExpired, contract.ReasonGenerationStale, contract.ReasonReplayRejected, contract.ReasonConflict:
		return http.StatusConflict
	case contract.ReasonRegistryUnavailable, contract.ReasonCapacityExhausted:
		return http.StatusServiceUnavailable
	case contract.ReasonNotFound:
		return http.StatusNotFound
	case contract.ReasonAuthFailed:
		return http.StatusUnauthorized
	default:
		return http.StatusServiceUnavailable
	}
}

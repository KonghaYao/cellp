package scheduler

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// CommandErrorFatal reports whether an Agent command error is authoritative fencing.
func CommandErrorFatal(err error) bool {
	if err == nil {
		return false
	}
	var cmd *agent.CommandError
	if errors.As(err, &cmd) {
		return cmd.Reason == contract.ReasonGenerationStale
	}
	return errors.Is(err, registry.ErrObservationStale) ||
		errors.Is(err, registry.ErrLeaseExpired) ||
		errors.Is(err, registry.ErrAssignmentCASConflict)
}

// RegistryErrorTransient reports whether a registry error should be retried without stopping replicas.
func RegistryErrorTransient(err error) bool {
	if err == nil {
		return false
	}
	if GuardLostFatal(err) {
		return false
	}
	if CommandErrorFatal(err) {
		return false
	}
	return true
}

// IsTransientAgentOrRegistry returns true for errors that should not terminalize healthy assignments.
func IsTransientAgentOrRegistry(err error) bool {
	if err == nil {
		return false
	}
	if GuardLostFatal(err) {
		return false
	}
	if CommandErrorFatal(err) {
		return false
	}
	var cmd *agent.CommandError
	if errors.As(err, &cmd) {
		switch cmd.Reason {
		case contract.ReasonColdActivating, contract.ReasonReplayRejected:
			return true
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") || strings.Contains(msg, "temporary") {
		return true
	}
	return RegistryErrorTransient(err)
}

// TickFatal reports whether the scheduler loop must stop after a tick error.
func TickFatal(err error) bool {
	return GuardLostFatal(err)
}

// ignoreContext returns true when ctx is cancelled but the error should not fail the tick.
func ignoreContext(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err())
}

package transport

import (
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

type wireError struct {
	Reason contract.ReasonCode `json:"reason"`
}

type startRequest struct {
	Spec contract.StartReplicaSpec `json:"spec"`
}

type scopeRequest struct {
	Scope contract.CommandScope `json:"scope"`
}

type drainRequest struct {
	Scope    contract.CommandScope `json:"scope"`
	Deadline string                `json:"deadline,omitempty"` // RFC3339; optional scaffold hint
}

type startResponse struct {
	Replica contract.RuntimeReplica `json:"replica"`
}

type probeResponse struct {
	Result probeWire `json:"result"`
}

type probeWire struct {
	ReplicaID  string                `json:"replica_id"`
	State      contract.ReplicaState `json:"state"`
	Generation int64                 `json:"generation"`
}

type stopResponse struct {
	Replica contract.RuntimeReplica `json:"replica"`
}

type drainResponse struct {
	Replica contract.RuntimeReplica `json:"replica"`
}

type listResponse struct {
	Replicas []contract.RuntimeReplica `json:"replicas"`
}

func parseMediaType(v string) (string, error) {
	v = strings.TrimSpace(strings.Split(v, ";")[0])
	if v == "" {
		return "", errBadContentType
	}
	return strings.ToLower(v), nil
}

func isOversize(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "request body too large")
}

// parseOptionalDrainDeadline accepts empty (no deadline) or a strict RFC3339 instant in the future.
func parseOptionalDrainDeadline(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	deadline, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errBadDrainDeadline
	}
	deadline = deadline.UTC()
	if !deadline.After(now.UTC()) {
		return time.Time{}, errBadDrainDeadline
	}
	return deadline, nil
}

var (
	errBadContentType   = errWire{reason: contract.ReasonAuthFailed}
	errBadJSON          = errWire{reason: contract.ReasonAuthFailed}
	errTrailingJSON     = errWire{reason: contract.ReasonAuthFailed}
	errRequestTooLarge  = errWire{reason: contract.ReasonRequestTooLarge}
	errResponseTooLarge = errWire{reason: contract.ReasonRequestTooLarge}
	errBadDrainDeadline = errWire{reason: contract.ReasonAuthFailed}
)

type errWire struct {
	reason contract.ReasonCode
}

func (e errWire) Reason() contract.ReasonCode { return e.reason }

func (e errWire) Error() string { return string(e.reason) }

func validateIdempotencyKey(h string) error {
	h = strings.TrimSpace(h)
	if h == "" {
		return nil
	}
	if len(h) > maxIdempotencyKeyLen {
		return errWire{reason: contract.ReasonAuthFailed}
	}
	for _, c := range h {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return errWire{reason: contract.ReasonAuthFailed}
		}
	}
	return nil
}

func validateSecretRefs(refs []contract.SecretRef) error {
	for _, ref := range refs {
		if strings.TrimSpace(ref.Name) == "" {
			return errWire{reason: contract.ReasonAuthFailed}
		}
	}
	return nil
}

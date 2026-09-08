package transport

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	"github.com/cellp/cellp/internal/elastic/registrywire"
)

// ClientError is a parsed node-registration wire failure.
type ClientError struct {
	Reason    contract.ReasonCode
	Retryable bool
}

func (e *ClientError) Error() string { return string(e.Reason) }

// IsClientRetryable reports whether the client should backoff and retry.
func IsClientRetryable(err error) bool {
	var ce *ClientError
	return err != nil && errors.As(err, &ce) && ce.Retryable
}

func decodeClientError(status int, data []byte, retryAfter string) error {
	var we wireError
	if err := json.Unmarshal(data, &we); err == nil && we.Reason != "" {
		reason := contract.NormalizeWireReason(we.Reason)
		switch reason {
		case contract.ReasonCapacityExhausted:
			return &ClientError{Reason: reason, Retryable: true}
		case contract.ReasonRegistryUnavailable:
			return nodereg.ErrRegistryUnavailable
		case contract.ReasonGenerationStale, contract.ReasonLeaseExpired:
			if auth := registrywire.AuthoritativeFromReason(reason); auth != nil {
				return auth
			}
		}
		if reason == contract.ReasonAuthFailed {
			return &nodereg.HandlerError{Reason: reason}
		}
		if auth := registrywire.AuthoritativeFromReason(reason); auth != nil {
			return auth
		}
		return &ClientError{Reason: reason}
	}
	if status == http.StatusServiceUnavailable {
		return nodereg.ErrRegistryUnavailable
	}
	if status >= 500 {
		return nodereg.ErrRegistryUnavailable
	}
	return &ClientError{Reason: contract.ReasonAuthFailed}
}

// DecodeClientErrorForTest exposes wire decoding for unit tests.
func DecodeClientErrorForTest(status int, data []byte, retryAfter string) error {
	return decodeClientError(status, data, retryAfter)
}

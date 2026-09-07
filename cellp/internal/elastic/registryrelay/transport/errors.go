package transport

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/elastic/registrywire"
)

var errReplayCacheCapacity = agenttransport.ErrReplayCacheAtCapacity{}

type wireError struct {
	Reason contract.ReasonCode `json:"reason"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeWireError(w http.ResponseWriter, status int, reason contract.ReasonCode) {
	writeJSON(w, status, wireError{Reason: reason})
}

func writeWireErrorWithRetry(w http.ResponseWriter, status int, reason contract.ReasonCode, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconvItoa(int(retryAfter.Seconds())))
	}
	writeWireError(w, status, reason)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "too large") {
		writeWireError(w, http.StatusRequestEntityTooLarge, contract.ReasonRequestTooLarge)
		return
	}
	writeWireError(w, http.StatusBadRequest, contract.ReasonAuthFailed)
}

func writeHandlerError(w http.ResponseWriter, err error) {
	if errors.Is(err, registryrelay.ErrRegistryUnavailable) {
		writeWireError(w, http.StatusServiceUnavailable, contract.ReasonRegistryUnavailable)
		return
	}
	if errors.Is(err, errReplayCacheCapacity) {
		writeWireErrorWithRetry(w, http.StatusServiceUnavailable, contract.ReasonCapacityExhausted, 5)
		return
	}
	var relay *registryrelay.RelayError
	if errors.As(err, &relay) && relay.Reason != "" {
		writeWireError(w, reasonHTTPStatus(relay.Reason), relay.Reason)
		return
	}
	writeWireError(w, http.StatusServiceUnavailable, contract.ReasonRegistryUnavailable)
}

func reasonHTTPStatus(reason contract.ReasonCode) int {
	switch reason {
	case contract.ReasonRequestTooLarge:
		return http.StatusRequestEntityTooLarge
	case contract.ReasonRegistryUnavailable, contract.ReasonCapacityExhausted:
		return http.StatusServiceUnavailable
	case contract.ReasonGenerationStale, contract.ReasonReplayRejected, contract.ReasonConflict, contract.ReasonLeaseExpired:
		return http.StatusConflict
	case contract.ReasonNotFound,
		contract.ReasonRuntimeNodeNotFound,
		contract.ReasonRuntimeReplicaNotFound:
		return http.StatusNotFound
	case contract.ReasonAuthFailed:
		return http.StatusUnauthorized
	default:
		return http.StatusServiceUnavailable
	}
}

func decodeRelayError(status int, data []byte) error {
	var we wireError
	if err := json.Unmarshal(data, &we); err == nil && we.Reason != "" {
		reason := contract.NormalizeWireReason(we.Reason)
		if reason == contract.ReasonRegistryUnavailable || status == http.StatusServiceUnavailable {
			if reason == contract.ReasonCapacityExhausted {
				return &registryrelay.RelayError{Reason: contract.ReasonCapacityExhausted}
			}
			return registryrelay.ErrRegistryUnavailable
		}
		if auth := registrywire.AuthoritativeFromReason(reason); auth != nil {
			return auth
		}
		return &registryrelay.RelayError{Reason: reason}
	}
	if status == http.StatusServiceUnavailable {
		return registryrelay.ErrRegistryUnavailable
	}
	if status >= 500 {
		return registryrelay.ErrRegistryUnavailable
	}
	return &registryrelay.RelayError{Reason: contract.ReasonAuthFailed}
}

func strconvItoa(v int) string {
	if v <= 0 {
		return "1"
	}
	return fmtInt(v)
}

func fmtInt(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

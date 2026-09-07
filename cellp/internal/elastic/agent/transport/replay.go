package transport

import (
	"errors"
	"sync"
	"time"
)

// ReplayCache rejects duplicate command nonces (fail-closed at capacity).
type ReplayCache struct {
	mu         sync.Mutex
	maxEntries int
	entries    map[string]time.Time // key -> expiry
}

// NewReplayCache creates a bounded replay table.
func NewReplayCache(maxEntries int) *ReplayCache {
	if maxEntries <= 0 {
		maxEntries = 4096
	}
	return &ReplayCache{
		maxEntries: maxEntries,
		entries:    make(map[string]time.Time),
	}
}

// ReplayKey material binds authenticated principal and command scope (CF-MTLS).
func ReplayKey(principal string, nodeID, projectID, versionID, replicaID, action string, generation int64, nonce string) string {
	return principal + "\x00" + nodeID + "\x00" + projectID + "\x00" + versionID + "\x00" + replicaID + "\x00" + action + "\x00" + fmtInt64(generation) + "\x00" + nonce
}

func fmtInt64(v int64) string {
	// Small helper to avoid fmt import in hot path.
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
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

// Consume records a nonce until expiry. Returns replay=true when the same key is still valid.
// When at capacity and the key is new, returns errAtCapacity (fail-closed).
func (c *ReplayCache) Consume(key string, expiry time.Time) (replay bool, err error) {
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeExpired(now)
	if exp, ok := c.entries[key]; ok {
		if exp.After(now) {
			return true, nil
		}
		delete(c.entries, key)
	}
	if len(c.entries) >= c.maxEntries {
		return false, errReplayCacheFull
	}
	c.entries[key] = expiry.UTC()
	return false, nil
}

func (c *ReplayCache) purgeExpired(now time.Time) {
	for k, exp := range c.entries {
		if !exp.After(now) {
			delete(c.entries, k)
		}
	}
}

var errReplayCacheFull = ErrReplayCacheAtCapacity{}

// ErrReplayCacheAtCapacity is returned when the replay table cannot admit a new nonce.
type ErrReplayCacheAtCapacity struct{}

func (ErrReplayCacheAtCapacity) Error() string { return "replay cache at capacity" }

// IsReplayCacheAtCapacity reports bounded replay table exhaustion (distinct from true replay).
func IsReplayCacheAtCapacity(err error) bool {
	var capErr ErrReplayCacheAtCapacity
	return errors.As(err, &capErr)
}

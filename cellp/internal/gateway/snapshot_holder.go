package gateway

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

const defaultSnapshotPollInterval = 2 * time.Second

// RouteSnapshotHolder keeps the last-known-good immutable route snapshot (AD-15 E1).
type RouteSnapshotHolder struct {
	mu              sync.RWMutex
	snap            contract.RouteSnapshot
	hasLKG          bool
	lastAppliedRev  int64
	pollErrors      atomic.Uint64
}

// NewRouteSnapshotHolder returns an empty holder.
func NewRouteSnapshotHolder() *RouteSnapshotHolder {
	return &RouteSnapshotHolder{}
}

// Snapshot returns a copy of the current LKG snapshot and whether any snapshot was ever applied.
func (h *RouteSnapshotHolder) Snapshot() (contract.RouteSnapshot, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.snap, h.hasLKG
}

// LastAppliedRevision returns the revision of the held snapshot (0 if none).
func (h *RouteSnapshotHolder) LastAppliedRevision() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastAppliedRev
}

// PollErrors returns the count of failed poll/validate attempts since start.
func (h *RouteSnapshotHolder) PollErrors() uint64 {
	return h.pollErrors.Load()
}

// PollOnce refreshes from the registry when route revision advances.
func (h *RouteSnapshotHolder) PollOnce(ctx context.Context, store registry.Store) {
	rev, err := store.GetRouteRevision(ctx)
	if err != nil {
		h.pollErrors.Add(1)
		return
	}
	h.mu.RLock()
	last := h.lastAppliedRev
	h.mu.RUnlock()
	if rev <= last {
		return
	}
	snap, err := store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		h.pollErrors.Add(1)
		return
	}
	if err := contract.ValidateRouteSnapshot(last, snap, time.Now().UTC()); err != nil {
		h.pollErrors.Add(1)
		log.Printf("gateway snapshot: validate failed rev=%d: %v", snap.Revision, err)
		return
	}
	h.mu.Lock()
	h.snap = snap
	h.hasLKG = true
	h.lastAppliedRev = snap.Revision
	h.mu.Unlock()
}

// StartSnapshotPoller runs PollOnce on an interval until ctx is done.
func StartSnapshotPoller(ctx context.Context, store registry.Store, h *RouteSnapshotHolder, interval time.Duration) {
	if h == nil || store == nil {
		return
	}
	if interval <= 0 {
		interval = defaultSnapshotPollInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		h.PollOnce(ctx, store)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.PollOnce(ctx, store)
			}
		}
	}()
}

// LookupUpstreamFromSnapshot returns host:port for ready legacy route if snapshot contains an endpoint.
func (h *RouteSnapshotHolder) LookupUpstreamFromSnapshot(projectID, versionID string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.hasLKG {
		return "", false
	}
	for _, es := range h.snap.EndpointSets {
		if es.ProjectID != projectID || es.VersionID != versionID {
			continue
		}
		for _, ep := range es.Endpoints {
			if ep.State == contract.EndpointReady && ep.Address != "" {
				return ep.Address, true
			}
		}
	}
	return "", false
}

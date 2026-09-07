package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

const defaultSnapshotWaitTimeout = 30 * time.Second

// WaitPublished implements orch.RouteSnapshotAck.
// It polls the registry qualification read model (deploy_ready endpoints), not the public LKG holder.
func (h *RouteSnapshotHolder) WaitPublished(ctx context.Context, store registry.Store, minRevision int64, projectID, versionID string) error {
	return h.WaitRouteSnapshotPublished(ctx, store, minRevision, projectID, versionID)
}

// WaitRouteSnapshotPublished waits until the qualification view revision is at least minRevision
// and includes a non-empty endpoint set for the version. Public RouteSnapshotHolder/LKG is not used
// for qualification and must not be updated with deploy_ready traffic.
func (h *RouteSnapshotHolder) WaitRouteSnapshotPublished(ctx context.Context, store registry.Store, minRevision int64, projectID, versionID string) error {
	if store == nil {
		return fmt.Errorf("route snapshot waiter not configured")
	}
	deadline := time.Now().Add(defaultSnapshotWaitTimeout)
	after := minRevision - 1
	var lastRev int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		view, ok, err := store.BuildQualificationViewAfter(ctx, after)
		if err != nil {
			return err
		}
		if ok {
			lastRev = view.RouteRevision
			if view.RouteRevision >= minRevision && qualificationEndpointsPresent(view, projectID, versionID) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("qualification route view not published for %s/%s (need rev>=%d, last=%d)",
				projectID, versionID, minRevision, lastRev)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func qualificationEndpointsPresent(view registry.QualificationView, projectID, versionID string) bool {
	for _, set := range view.EndpointSets {
		if set.ProjectID == projectID && set.VersionID == versionID && len(set.Endpoints) > 0 {
			return true
		}
	}
	return false
}

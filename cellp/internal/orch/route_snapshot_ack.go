package orch

import (
	"context"

	"github.com/cellp/cellp/internal/registry"
)

// RouteSnapshotAck waits until the registry qualification view (deploy_ready endpoints)
// reflects at least minRevision and includes endpoints for the version (AD-15).
// Public gateway LKG snapshots exclude deploy_ready and are not used for this wait.
type RouteSnapshotAck interface {
	WaitPublished(ctx context.Context, store registry.Store, minRevision int64, projectID, versionID string) error
}

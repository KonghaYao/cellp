package registry

import (
	"context"
	"errors"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// StatusDeployReady is the additive AD-15 lifecycle state (canonical string in contract).
const StatusDeployReady = contract.StatusDeployReady

// ErrDesiredCASConflict is returned when CompareAndSetDesired loses the generation race.
var ErrDesiredCASConflict = errors.New("serving_desire_cas_conflict")

// ErrControllerGuardHeld is returned when another holder owns the singleton guard.
var ErrControllerGuardHeld = errors.New("controller_guard_held")

// ServingPolicyRow is the persisted serving policy for a version.
type ServingPolicyRow struct {
	ProjectID       string
	VersionID       string
	Revision        int64
	MinReplicas     int
	MaxReplicas     int
	Priority        int
	BackgroundMode  contract.BackgroundMode
	ElasticEnrolled bool
	UpdatedAt       time.Time
}

// ServingDesireRow is the autoscaler-desired replica count.
type ServingDesireRow struct {
	ProjectID        string
	VersionID        string
	DesiredReplicas  int
	Generation       int64
	Reason           string
	UpdatedAt        time.Time
}

// ControllerGuardState exposes the active writer lease metadata.
type ControllerGuardState struct {
	HolderID   string
	AcquiredAt *time.Time
	HolderPID  int
}

// ServingStore extends registry with AD-15 control-plane facts (WP-REG).
type ServingStore interface {
	GetRouteRevision(ctx context.Context) (int64, error)
	BumpRouteRevision(ctx context.Context) (int64, error)

	UpsertServingPolicy(ctx context.Context, row ServingPolicyRow) error
	GetServingPolicy(ctx context.Context, projectID, versionID string) (*ServingPolicyRow, error)
	ListElasticServingPolicies(ctx context.Context) ([]ServingPolicyRow, error)

	CompareAndSetDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire ServingDesireRow) error
	GetServingDesire(ctx context.Context, projectID, versionID string) (*ServingDesireRow, error)

	UpsertRuntimeNode(ctx context.Context, node contract.RuntimeNode) error
	GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error)
	ListRuntimeNodes(ctx context.Context) ([]contract.RuntimeNode, error)

	UpsertRuntimeReplica(ctx context.Context, rep contract.RuntimeReplica) error
	ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error)

	TryAcquireControllerGuard(ctx context.Context, holderID string, pid int) error
	ReleaseControllerGuard(ctx context.Context, holderID string) error
	GetControllerGuard(ctx context.Context) (*ControllerGuardState, error)

	BuildLegacyRouteSnapshot(ctx context.Context) (contract.RouteSnapshot, error)
}

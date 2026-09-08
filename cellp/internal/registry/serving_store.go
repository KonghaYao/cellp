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

// ErrElasticVersionStatusCASConflict is returned when an enrolled version changed concurrently.
var ErrElasticVersionStatusCASConflict = errors.New("elastic_version_status_cas_conflict")

// ErrActivationNotEligible is returned when an activation write targets a non-cold/non-enrolled version.
var ErrActivationNotEligible = errors.New("activation_not_eligible")

// ErrServingCapacityUnavailable is returned when policy cannot satisfy activation min capacity.
var ErrServingCapacityUnavailable = errors.New("serving_capacity_unavailable")

// ErrControllerGuardHeld is returned when another holder owns the singleton guard.
var ErrControllerGuardHeld = errors.New("controller_guard_held")

// ErrAssignmentCASConflict is returned when ClaimAssignment loses the generation race.
var ErrAssignmentCASConflict = errors.New("assignment_cas_conflict")

// ErrObservationStale is returned when RecordObservation sees a stale generation.
var ErrObservationStale = errors.New("replica_observation_stale")

// ErrReplicaTransitionInvalid is returned when a replica state transition is not allowed.
var ErrReplicaTransitionInvalid = errors.New("replica_transition_invalid")

// ErrLeaseExpired is returned when a node or assignment lease is no longer valid.
var ErrLeaseExpired = errors.New("lease_expired")

// ErrNodeLeaseCASConflict is returned when a generation-fenced runtime node lease operation loses the race.
var ErrNodeLeaseCASConflict = errors.New("runtime_node_lease_cas_conflict")

// ErrRuntimeNodeNotFound is returned when a runtime node record is required but absent.
var ErrRuntimeNodeNotFound = errors.New("runtime_node_not_found")

// ErrRuntimeReplicaNotFound is returned when a runtime replica record is required but absent.
var ErrRuntimeReplicaNotFound = errors.New("runtime_replica_not_found")

// ErrAgentVersionEnvForbidden is returned when a node is not authorized to read worker env for a version.
var ErrAgentVersionEnvForbidden = errors.New("agent_version_env_forbidden")

// ErrAgentCommandConflict is returned when an idempotency key is reused for a different command.
var ErrAgentCommandConflict = errors.New("agent_command_scope_conflict")

// ErrAgentCommandInProgress is returned while another handler owns an unfinished command.
var ErrAgentCommandInProgress = errors.New("agent_command_in_progress")

// AgentCommand stores only bounded lifecycle result metadata; request payloads and secrets are excluded.
type AgentCommand struct {
	IdempotencyKey string                   `json:"idempotency_key"`
	Action         contract.LifecycleAction `json:"action"`
	NodeID         string                   `json:"node_id"`
	ProjectID      string                   `json:"project_id"`
	VersionID      string                   `json:"version_id"`
	ReplicaID      string                   `json:"replica_id"`
	Generation     int64                    `json:"generation"`
	Status         string                   `json:"status"`
	ResultState    contract.ReplicaState    `json:"result_state"`
	Reason         contract.ReasonCode      `json:"reason"`
	ExpiresAt      time.Time                `json:"expires_at"`
	LeaseExpiresAt time.Time                `json:"lease_expires_at"`
	AttemptToken   string                   `json:"attempt_token"`
}

// AgentCommandClaim is the result of an atomic durable claim.
type AgentCommandClaim struct {
	Claimed bool         `json:"claimed"`
	Command AgentCommand `json:"command"`
}

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
	ProjectID       string
	VersionID       string
	DesiredReplicas int
	Generation      int64
	Reason          string
	UpdatedAt       time.Time
}

// ControllerGuardState exposes the active writer lease metadata.
type ControllerGuardState struct {
	HolderID   string
	AcquiredAt *time.Time
	HolderPID  int
}

// AssignmentClaim is a Scheduler placement write (generation-fenced).
type AssignmentClaim struct {
	ReplicaID              string
	ProjectID              string
	VersionID              string
	NodeID                 string
	Generation             int64 // must match serving_desires.generation
	ExpectedNodeGeneration int64 // must match runtime_nodes.generation
	ActiveLimit            int   // optional atomic cap for Scheduler claims; zero keeps compatibility callers unchanged
	ValidUntil             time.Time
}

// QualificationView exposes healthy endpoints for deploy_ready qualification (not public gateway traffic).
type QualificationView struct {
	RouteRevision int64
	EndpointSets  []contract.EndpointSet
}

// ReplicaObservation is an Agent-driven replica state report (strict CAS path).
type ReplicaObservation struct {
	ReplicaID            string                 `json:"replica_id"`
	ProjectID            string                 `json:"project_id"`
	VersionID            string                 `json:"version_id"`
	NodeID               string                 `json:"node_id"`
	Generation           int64                  `json:"generation"`
	State                contract.ReplicaState  `json:"state"`
	ListenHost           string                 `json:"listen_host"`
	ListenPort           int                    `json:"listen_port"`
	EndpointState        contract.EndpointState `json:"endpoint_state"`
	AssignmentValidUntil *time.Time             `json:"assignment_valid_until,omitempty"`
	EndpointValidUntil   *time.Time             `json:"endpoint_valid_until,omitempty"`
}

// ElasticVersionRef is a version enrolled in elastic serving (reconcile queries).
type ElasticVersionRef struct {
	ProjectID string
	VersionID string
}

// ServingStore extends registry with AD-15 control-plane facts (WP-REG).
type ServingStore interface {
	GetRouteRevision(ctx context.Context) (int64, error)
	BumpRouteRevision(ctx context.Context) (int64, error)
	GetVersion(ctx context.Context, projectID, versionID string) (*Version, error)

	// CompareAndSetElasticVersionStatus transitions an enrolled version and desire generation atomically.
	CompareAndSetElasticVersionStatus(ctx context.Context, projectID, versionID, expectedStatus, newStatus string, expectedDesireGeneration int64, expectedDesiredReplicas int) error

	UpsertServingPolicy(ctx context.Context, row ServingPolicyRow) error
	GetServingPolicy(ctx context.Context, projectID, versionID string) (*ServingPolicyRow, error)
	ListElasticServingPolicies(ctx context.Context) ([]ServingPolicyRow, error)

	CompareAndSetDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire ServingDesireRow) error
	EnsureActivationDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire ServingDesireRow, minReplicas int) error
	GetServingDesire(ctx context.Context, projectID, versionID string) (*ServingDesireRow, error)

	UpsertRuntimeNode(ctx context.Context, node contract.RuntimeNode) error
	ActivateRuntimeNode(ctx context.Context, node contract.RuntimeNode, expectedGeneration int64) error
	RenewRuntimeNodeLease(ctx context.Context, nodeID string, expectGen int64, leaseExpiry time.Time) error
	ReleaseRuntimeNodeLease(ctx context.Context, nodeID string, expectGen int64) error
	CordonRuntimeNode(ctx context.Context, nodeID string, expectGen int64) error
	GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error)
	ListRuntimeNodes(ctx context.Context) ([]contract.RuntimeNode, error)

	ClaimAssignment(ctx context.Context, claim AssignmentClaim) error
	RenewAssignmentLease(ctx context.Context, replicaID, nodeID string, generation, expectedNodeGeneration int64, expectedExpiry, newExpiry time.Time) error
	RecordObservation(ctx context.Context, obs ReplicaObservation) error
	GetRuntimeReplica(ctx context.Context, replicaID string) (*contract.RuntimeReplica, error)
	ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error)
	ValidateAgentCleanupAssignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error)
	UpsertRuntimeReplica(ctx context.Context, rep contract.RuntimeReplica) error
	ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error)
	ListRuntimeReplicasByNode(ctx context.Context, nodeID string) ([]contract.RuntimeReplica, error)
	ListRuntimeReplicasForReconcile(ctx context.Context) ([]contract.RuntimeReplica, error)
	ListExpiredAssignments(ctx context.Context, now time.Time) ([]contract.RuntimeReplica, error)
	ListElasticEnrolledVersions(ctx context.Context) ([]ElasticVersionRef, error)

	ClaimAgentCommand(ctx context.Context, command AgentCommand) (AgentCommandClaim, error)
	RenewAgentCommandLease(ctx context.Context, command AgentCommand, expiry time.Time) error
	CompleteAgentCommand(ctx context.Context, command AgentCommand) error
	RecordObservationAndCompleteAgentCommand(ctx context.Context, obs ReplicaObservation, command AgentCommand) error
	WithdrawReplica(ctx context.Context, replicaID, projectID, versionID, nodeID string, generation int64) error
	TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error

	TryAcquireControllerGuard(ctx context.Context, holderID string, pid int) error
	ReleaseControllerGuard(ctx context.Context, holderID string) error
	GetControllerGuard(ctx context.Context) (*ControllerGuardState, error)

	BuildLegacyRouteSnapshot(ctx context.Context) (contract.RouteSnapshot, error)
	BuildSnapshotAfter(ctx context.Context, afterRevision int64) (contract.RouteSnapshot, bool, error)
	BuildQualificationViewAfter(ctx context.Context, afterRevision int64) (QualificationView, bool, error)

	GetAuthorizedAgentVersionEnv(ctx context.Context, nodeID, projectID, versionID string, now time.Time) (map[string]string, error)
	GetVersionEnv(ctx context.Context, projectID, versionID string) (map[string]string, error)
}

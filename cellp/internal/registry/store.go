package registry

import (
	"context"
	"errors"
	"time"
)

// Version status constants per DESIGN §2.5.
const (
	StatusPending   = "pending"
	StatusFetching  = "fetching"
	StatusBranching = "branching"
	StatusPreparing = "preparing"
	StatusDeploying = "deploying"
	StatusReady     = "ready"
	StatusArchived  = "archived"
	StatusDraining  = "draining"
	StatusDestroyed = "destroyed"
	StatusFailed    = "failed"
)

// Project represents a cellp project.
type Project struct {
	ID                    string     `json:"id"`
	GitRemote             *string    `json:"git_remote,omitempty"`
	ProdVersionID         *string    `json:"prod_version_id,omitempty"`
	PreviousProdVersionID *string    `json:"previous_prod_version_id,omitempty"`
	PreviousProdAt        *time.Time `json:"previous_prod_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

// Version represents a deployed version.
type Version struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	ParentVersionID *string    `json:"parent_version_id,omitempty"`
	GitRef          string     `json:"git_ref,omitempty"`
	GitSHA          string     `json:"git_sha,omitempty"`
	ArtifactURI     string     `json:"artifact_uri,omitempty"`
	ArtifactDigest  string     `json:"artifact_digest,omitempty"`
	DataBranch      string     `json:"data_branch,omitempty"`
	PreviewURL      string     `json:"preview_url,omitempty"`
	Status          string     `json:"status"`
	Error           *string    `json:"error,omitempty"`
	TTL             *time.Time `json:"ttl,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ReadyAt         *time.Time `json:"ready_at,omitempty"`
	Pinned          bool       `json:"pinned"`
	LastAccessAt    *time.Time `json:"last_access_at,omitempty"`
}

// Route maps a version to an upstream celld instance.
type Route struct {
	ProjectID    string `json:"project_id"`
	VersionID    string `json:"version_id"`
	Active       bool   `json:"active"`
	UpstreamHost string `json:"upstream_host"`
	UpstreamPort int    `json:"upstream_port"`
}

// Ingress binding roles (AD-12 / INGRESS-ROUTING §3.2).
const (
	IngressRolePreview = "preview"
	IngressRoleProd    = "prod"
)

// Default ingress dedicated listen port pool (INGRESS-ROUTING §3.3).
const (
	DefaultIngressPortMin = 19080
	DefaultIngressPortMax = 19999
)

// Port allocation purpose / stability / owner (INGRESS-PORT-DEPLOYMENT §3.1).
const (
	PortPurposeIngressListen = "ingress_listen"
	PortPurposeCelldUpstream = "celld_upstream"
	PortStabilityEphemeral   = "ephemeral"
	PortStabilityStable      = "stable"
	PortOwnerIngressBinding  = "ingress_binding"
	PortOwnerCelldRoute      = "celld_route"
)

// Port allocation sentinel errors (P5a).
var (
	ErrPortConflict              = errors.New("port conflict")
	ErrPortPoolExhausted         = errors.New("ingress port pool exhausted")
	ErrPortAllocationNotFound    = errors.New("port allocation not found")
	ErrPortInvalid               = errors.New("port invalid")
	ErrPortPurposeNotSupported   = errors.New("port purpose not supported in this release")
	ErrPortAllocationInputInvalid = errors.New("port allocation input invalid")
)

// ProdPortReserveOwnerID is the stable reserve owner for prod_listen_port (§4.1).
func ProdPortReserveOwnerID(projectID string) string {
	return projectID + "-prod-reserve"
}

// PortAllocation is an authoritative port ledger row (INGRESS-PORT-DEPLOYMENT §3.1).
type PortAllocation struct {
	AllocationID  string     `json:"allocation_id"`
	Port          int        `json:"port"`
	Purpose       string     `json:"purpose"`
	Stability     string     `json:"stability"`
	OwnerKind     string     `json:"owner_kind"`
	OwnerID       string     `json:"owner_id"`
	ProjectID     *string    `json:"project_id,omitempty"`
	GatewayID     *string    `json:"gateway_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
	ReleaseReason *string    `json:"release_reason,omitempty"`
}

// AllocateIngressListenPortInput allocates an ingress listen port in the pool.
type AllocateIngressListenPortInput struct {
	Stability string
	OwnerKind string
	OwnerID   string
	ProjectID *string
	GatewayID *string
}

// ReserveStablePortInput reserves a specific ingress listen port with stability=stable.
type ReserveStablePortInput struct {
	Port      int
	OwnerKind string
	OwnerID   string
	ProjectID *string
	GatewayID *string
}

// ReleasePortInput marks an active allocation as released.
type ReleasePortInput struct {
	AllocationID  string
	OwnerKind     string
	OwnerID       string
	ReleaseReason string
}

// OpenOptions configures registry open (tests may narrow ingress port pool).
type OpenOptions struct {
	IngressPortMin int
	IngressPortMax int
}

// IngressBinding maps external Host and/or listen port to a project version (or prod).
type IngressBinding struct {
	BindingID      string  `json:"binding_id"`
	ProjectID      string  `json:"project_id"`
	VersionID      *string `json:"version_id,omitempty"`
	Role           string  `json:"role"`
	Host           *string `json:"host,omitempty"`
	ListenPort     *int    `json:"listen_port,omitempty"`
	SyntheticHost  string  `json:"synthetic_host"`
	OwnerGatewayID *string `json:"owner_gateway_id,omitempty"`
	Active         bool    `json:"active"`
}

// Job represents a persisted orchestrator job.
type Job struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	VersionID  string     `json:"version_id"`
	Step       string     `json:"step"`
	Status     string     `json:"status"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateProjectInput holds project creation fields.
type CreateProjectInput struct {
	ID        string
	GitRemote *string
}

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

// ListProjectsOpts controls paginated project listing.
type ListProjectsOpts struct {
	Limit  int
	Cursor string
	Query  string
}

// ProjectListItem is a project with aggregate version count.
type ProjectListItem struct {
	Project
	VersionCount int `json:"version_count"`
}

// ListProjectsPage is a paginated project list result.
type ListProjectsPage struct {
	Projects   []ProjectListItem `json:"projects"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// ListVersionsOpts controls paginated version listing.
type ListVersionsOpts struct {
	Limit  int
	Cursor string
	Status string
	Since  *time.Time
}

// ListVersionsPage is a paginated version list result.
type ListVersionsPage struct {
	Versions   []Version `json:"versions"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// CreateVersionInput holds version creation fields.
type CreateVersionInput struct {
	ID              string
	ProjectID       string
	ParentVersionID *string
	GitRef          string
	GitSHA          string
	ArtifactURI     string
	ArtifactDigest  string
	PreviewURL      string
	Env             map[string]string
}

// Store is the frozen registry interface (Phase 1 T1).
type Store interface {
	CreateProject(ctx context.Context, in CreateProjectInput) (*Project, error)
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context, opts ListProjectsOpts) (*ListProjectsPage, error)

	CreateVersion(ctx context.Context, in CreateVersionInput) (*Version, error)
	GetVersion(ctx context.Context, projectID, versionID string) (*Version, error)
	ListVersions(ctx context.Context, projectID string, opts ListVersionsOpts) (*ListVersionsPage, error)
	CountVersions(ctx context.Context, projectID string) (int, error)
	UpdateVersionStatus(ctx context.Context, projectID, versionID, status string, errMsg *string) error
	SetVersionPreviewURL(ctx context.Context, projectID, versionID, previewURL string) error
	CountReadyVersions(ctx context.Context, projectID string) (int, error)

	SetVersionPinned(ctx context.Context, projectID, versionID string, pinned bool) error
	TouchLastAccess(ctx context.Context, projectID, versionID string) error
	GetVersionEnv(ctx context.Context, projectID, versionID string) (map[string]string, error)
	SetVersionEnv(ctx context.Context, projectID, versionID string, env map[string]string) error
	ListAllReadyVersions(ctx context.Context) ([]Version, error)
	CountChildVersions(ctx context.Context, projectID, parentVersionID string) (int, error)

	SetRoute(ctx context.Context, route Route) error
	SetRouteActive(ctx context.Context, projectID, versionID string, active bool) error
	GetRoute(ctx context.Context, projectID, versionID string) (*Route, error)
	ListActiveRoutes(ctx context.Context, projectID string) ([]Route, error)
	ListAllActiveRoutes(ctx context.Context) ([]Route, error)
	DeleteRoute(ctx context.Context, projectID, versionID string) error

	UpsertIngressBinding(ctx context.Context, b IngressBinding) error
	GetIngressBinding(ctx context.Context, bindingID string) (*IngressBinding, error)
	LookupIngressByHost(ctx context.Context, host string) (*IngressBinding, error)
	LookupIngressByListenPort(ctx context.Context, listenPort int, ownerGatewayID string) (*IngressBinding, error)
	SetIngressBindingActive(ctx context.Context, bindingID string, active bool) error
	ListActiveIngressBindings(ctx context.Context) ([]IngressBinding, error)
	ListIngressBindingsByVersion(ctx context.Context, projectID, versionID string) ([]IngressBinding, error)

	AllocateIngressListenPort(ctx context.Context, in AllocateIngressListenPortInput) (*PortAllocation, error)
	ReserveStablePort(ctx context.Context, in ReserveStablePortInput) (*PortAllocation, error)
	ReleasePort(ctx context.Context, in ReleasePortInput) error
	GetActivePortAllocationByOwner(ctx context.Context, ownerKind, ownerID string) (*PortAllocation, error)
	ListActivePortAllocations(ctx context.Context, purpose string) ([]PortAllocation, error)
	AttachIngressListenPort(ctx context.Context, binding IngressBinding, in AllocateIngressListenPortInput) error
	DetachIngressListenPort(ctx context.Context, bindingID, releaseReason string) error

	Ping(ctx context.Context) error

	SetProdVersion(ctx context.Context, projectID, versionID string) error
	SetProdVersionCAS(ctx context.Context, projectID, expected, new string) error

	EnqueueJob(ctx context.Context, projectID, versionID, step string) (*Job, error)
	ClaimJob(ctx context.Context, workerID string, lease time.Duration) (*Job, error)
	CountPendingJobs(ctx context.Context) (int, error)
	CompleteJob(ctx context.Context, jobID string) error
	UpdateJobStep(ctx context.Context, jobID, step string) error
	FailJob(ctx context.Context, jobID string) error

	// PurgeCompletedJobs deletes completed/failed jobs with updated_at before olderThan.
	PurgeCompletedJobs(ctx context.Context, olderThan time.Time) (int64, error)
	// PurgeDestroyedVersions deletes destroyed version metadata (and inactive routes)
	// with updated_at before olderThan; skips prod versions and active routes.
	PurgeDestroyedVersions(ctx context.Context, olderThan time.Time) (int64, error)
}

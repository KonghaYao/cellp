package registry

import (
	"context"
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
	StatusDraining  = "draining"
	StatusDestroyed = "destroyed"
	StatusFailed    = "failed"
)

// Project represents a cellp project.
type Project struct {
	ID            string    `json:"id"`
	GitRemote     *string   `json:"git_remote,omitempty"`
	ProdVersionID *string   `json:"prod_version_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
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
}

// Route maps a version to an upstream celld instance.
type Route struct {
	ProjectID    string `json:"project_id"`
	VersionID    string `json:"version_id"`
	Active       bool   `json:"active"`
	UpstreamHost string `json:"upstream_host"`
	UpstreamPort int    `json:"upstream_port"`
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
	CountReadyVersions(ctx context.Context, projectID string) (int, error)

	SetRoute(ctx context.Context, route Route) error
	SetRouteActive(ctx context.Context, projectID, versionID string, active bool) error
	GetRoute(ctx context.Context, projectID, versionID string) (*Route, error)
	ListActiveRoutes(ctx context.Context, projectID string) ([]Route, error)
	ListAllActiveRoutes(ctx context.Context) ([]Route, error)
	DeleteRoute(ctx context.Context, projectID, versionID string) error

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

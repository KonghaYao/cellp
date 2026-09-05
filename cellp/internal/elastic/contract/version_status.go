package contract

// Version lifecycle statuses (additive to DESIGN §2.5).
const (
	StatusPending      = "pending"
	StatusFetching     = "fetching"
	StatusBranching    = "branching"
	StatusPreparing    = "preparing"
	StatusDeploying    = "deploying"
	StatusDeployReady  = "deploy_ready"
	StatusReady        = "ready"
	StatusArchived     = "archived"
	StatusDraining     = "draining"
	StatusDestroyed    = "destroyed"
	StatusFailed       = "failed"
)

// AllVersionStatuses lists every known status for validation and OpenAPI alignment.
var AllVersionStatuses = []string{
	StatusPending,
	StatusFetching,
	StatusBranching,
	StatusPreparing,
	StatusDeploying,
	StatusDeployReady,
	StatusReady,
	StatusArchived,
	StatusDraining,
	StatusDestroyed,
	StatusFailed,
}

// ValidVersionStatus reports whether s is a known version status string.
func ValidVersionStatus(s string) bool {
	for _, v := range AllVersionStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsServingQualifiedReady is true when the version has completed qualification:
// at least one ready endpoint in snapshot (registry/orch enforce in WP-REG).
func IsServingQualifiedReady(status string) bool {
	return status == StatusReady
}

// ParentBranchEligible reports whether a parent version may be used for D1/KV/R2/Queue branch (AD-15 / CF-BRANCH).
func ParentBranchEligible(status string) bool {
	switch status {
	case StatusReady, StatusDeployReady, StatusArchived:
		return true
	default:
		return false
	}
}

// PromoteEligible requires a qualified ready version with routable endpoint (orchestrator enforces).
func PromoteEligible(status string) bool {
	return status == StatusReady
}

// ArchiveEligible matches current orch: only ready non-prod (elastic cold uses a different path).
func ArchiveEligible(status string) bool {
	return status == StatusReady
}

// WakeEligible matches current orch: explicit archived only.
func WakeEligible(status string) bool {
	return status == StatusArchived
}

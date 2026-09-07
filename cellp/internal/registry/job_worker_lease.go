package registry

import (
	"database/sql"
	"strings"
	"time"
)

// jobWorkerLeaseIsLive reports whether a claimed job row must not be taken over on normal
// discovery/claim paths (valid future lease, or unparseable lease treated as held).
func jobWorkerLeaseIsLive(leaseUntil sql.NullString, now time.Time) bool {
	if !leaseUntil.Valid || strings.TrimSpace(leaseUntil.String) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, leaseUntil.String)
	if err != nil {
		return true
	}
	return t.After(now)
}

// jobWorkerLeaseRenewable is true when the holder may extend lease_until (valid RFC3339Nano, not expired).
func jobWorkerLeaseRenewable(leaseUntil sql.NullString, now time.Time) bool {
	if !leaseUntil.Valid || strings.TrimSpace(leaseUntil.String) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, leaseUntil.String)
	if err != nil {
		return false
	}
	return t.After(now)
}

// jobWorkerLeaseClaimTakeoverBlocked is true when a claimed compensating job must not be
// taken over on ClaimCompensatingJob (NULL/empty/unparseable).
// Invalid (unparseable) leases are eligible for RepairClaimedJobLeaseInvariantForCompensation but still
// block direct takeover here; expired RFC3339 leases are not blocked on this path and may be taken over
// when no deploy-operation lease is active.
func jobWorkerLeaseClaimTakeoverBlocked(leaseUntil sql.NullString) bool {
	if !leaseUntil.Valid || strings.TrimSpace(leaseUntil.String) == "" {
		return true
	}
	_, err := time.Parse(time.RFC3339Nano, leaseUntil.String)
	return err != nil
}

// jobWorkerLeaseNeedsCompensationRepair is true for NULL/empty/unparseable/expired worker leases.
func jobWorkerLeaseNeedsCompensationRepair(leaseUntil sql.NullString, now time.Time) bool {
	if !leaseUntil.Valid || strings.TrimSpace(leaseUntil.String) == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339Nano, leaseUntil.String)
	if err != nil {
		return true
	}
	return !t.After(now)
}

// jobLeaseExactMatchSQL returns a WHERE fragment matching the stored lease_until value for CAS updates.
func jobLeaseExactMatchSQL(leaseUntil sql.NullString) (where string, arg any) {
	if !leaseUntil.Valid || strings.TrimSpace(leaseUntil.String) == "" {
		return `(lease_until IS NULL OR lease_until = '')`, nil
	}
	return `lease_until = ?`, leaseUntil.String
}

func claimedWorkerExactMatchSQL(claimedWorker sql.NullString) (where string, arg any) {
	if !claimedWorker.Valid || strings.TrimSpace(claimedWorker.String) == "" {
		return `(claimed_worker_id IS NULL OR claimed_worker_id = '')`, nil
	}
	return `claimed_worker_id = ?`, strings.TrimSpace(claimedWorker.String)
}

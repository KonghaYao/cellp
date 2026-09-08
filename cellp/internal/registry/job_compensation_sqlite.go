package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// ErrJobLeaseConflict is returned when a job lease renew or claim does not match the holder.
var ErrJobLeaseConflict = errors.New("job_lease_conflict")

// ErrJobClaimedLeaseInvariant is returned when a claimed job has no valid worker lease and
// cannot be repaired while deploy-operation ownership may still be active.
var ErrJobClaimedLeaseInvariant = errors.New("job_claimed_lease_invariant")

const compensatingStepPrefix = "compensating:"

// compensatingClaimCandidateBatch is the page size for ClaimCompensatingJob candidate queries.
const compensatingClaimCandidateBatch = 32

// compensatingClaimMaxScanPerPhase caps candidate rows examined in one phase of ClaimCompensatingJob.
const compensatingClaimMaxScanPerPhase = 256

// compensatingClaimMaxScanPerCall is the upper bound on total row examinations per transaction (phase 1 + phase 2).
const compensatingClaimMaxScanPerCall = compensatingClaimMaxScanPerPhase * 2

const jobSelectCols = `id, project_id, version_id, step, status, lease_until, updated_at, claimed_worker_id, claim_epoch`

// compensatingDirectClaimAttemptHook is set only by registry tests to simulate unclaimable Phase-1 rows.
var compensatingDirectClaimAttemptHook func(*Job) bool

func scanJobRow(row interface {
	Scan(dest ...any) error
}) (*Job, error) {
	var j Job
	var leaseUntil sql.NullString
	var updated string
	var claimedWorker sql.NullString
	if err := row.Scan(&j.ID, &j.ProjectID, &j.VersionID, &j.Step, &j.Status, &leaseUntil, &updated, &claimedWorker, &j.ClaimEpoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if leaseUntil.Valid && leaseUntil.String != "" {
		t, err := time.Parse(time.RFC3339Nano, leaseUntil.String)
		if err == nil {
			j.LeaseUntil = &t
		}
	}
	if claimedWorker.Valid {
		j.ClaimedWorkerID = strings.TrimSpace(claimedWorker.String)
	}
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &j, nil
}

func (s *SQLiteStore) GetJob(ctx context.Context, jobID string) (*Job, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, nil
	}
	return withRetry(func() (*Job, error) {
		row := s.db.QueryRowContext(ctx, `
SELECT `+jobSelectCols+`
FROM jobs WHERE id = ?`, jobID)
		return scanJobRow(row)
	})
}

func (s *SQLiteStore) RenewClaimedJobLease(ctx context.Context, workerID string, attempt DeployAttempt, lease time.Duration) error {
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	workerID = strings.TrimSpace(workerID)
	if attempt.JobID == "" || workerID == "" || lease <= 0 {
		return errors.New("invalid job lease renew")
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		now := time.Now().UTC()
		var leaseUntil sql.NullString
		err = tx.QueryRowContext(ctx, `
SELECT lease_until FROM jobs
WHERE id = ? AND claimed_worker_id = ? AND claim_epoch = ? AND status = 'claimed'`,
			attempt.JobID, workerID, attempt.ClaimEpoch).Scan(&leaseUntil)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJobLeaseConflict
		}
		if err != nil {
			return err
		}
		if !jobWorkerLeaseRenewable(leaseUntil, now) {
			return ErrJobLeaseConflict
		}
		until := now.Add(lease).Format(time.RFC3339Nano)
		nowStr := now.Format(time.RFC3339Nano)
		leaseWhere, leaseArg := jobLeaseExactMatchSQL(leaseUntil)
		query := `
UPDATE jobs SET lease_until = ?, updated_at = ?
WHERE id = ? AND claimed_worker_id = ? AND claim_epoch = ? AND status = 'claimed' AND ` + leaseWhere
		args := []any{until, nowStr, attempt.JobID, workerID, attempt.ClaimEpoch}
		if leaseArg != nil {
			args = append(args, leaseArg)
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrJobLeaseConflict
		}
		return tx.Commit()
	})
}

// UpdateJobStepForAttempt updates compensation step only for the exact worker claim generation with a live lease.
func (s *SQLiteStore) UpdateJobStepForAttempt(ctx context.Context, workerID string, attempt DeployAttempt, step string) error {
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	workerID = strings.TrimSpace(workerID)
	step = strings.TrimSpace(step)
	if attempt.JobID == "" || workerID == "" || step == "" {
		return ErrJobLeaseConflict
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		var leaseUntil sql.NullString
		err = tx.QueryRowContext(ctx, `
SELECT lease_until FROM jobs
WHERE id = ? AND claimed_worker_id = ? AND claim_epoch = ? AND status = 'claimed'`,
			attempt.JobID, workerID, attempt.ClaimEpoch).Scan(&leaseUntil)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJobLeaseConflict
		}
		if err != nil {
			return err
		}
		if !jobWorkerLeaseRenewable(leaseUntil, now) {
			return ErrJobLeaseConflict
		}
		leaseWhere, leaseArg := jobLeaseExactMatchSQL(leaseUntil)
		query := `
UPDATE jobs SET step = ?, updated_at = ?
WHERE id = ? AND claimed_worker_id = ? AND claim_epoch = ? AND status = 'claimed' AND ` + leaseWhere
		args := []any{step, nowStr, attempt.JobID, workerID, attempt.ClaimEpoch}
		if leaseArg != nil {
			args = append(args, leaseArg)
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrJobLeaseConflict
		}
		return tx.Commit()
	})
}

// RepairClaimedJobLeaseInvariantForCompensation moves a claimed job with NULL/invalid worker lease
// into compensating after confirming no active deploy-operation lease still references the job.
func (s *SQLiteStore) RepairClaimedJobLeaseInvariantForCompensation(ctx context.Context, jobID string) (repaired bool, err error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return false, nil
	}
	return withRetry(func() (bool, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		var status, step, projectID, versionID string
		var leaseUntil sql.NullString
		var claimEpoch int64
		var claimedWorker sql.NullString
		err = tx.QueryRowContext(ctx, `
SELECT project_id, version_id, status, step, lease_until, claim_epoch, claimed_worker_id
FROM jobs WHERE id = ?`, jobID).
			Scan(&projectID, &versionID, &status, &step, &leaseUntil, &claimEpoch, &claimedWorker)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if status != "claimed" {
			return false, nil
		}
		if !strings.HasPrefix(step, compensatingStepPrefix) {
			return false, nil
		}
		if !jobWorkerLeaseNeedsCompensationRepair(leaseUntil, now) {
			return false, nil
		}
		opActive, oerr := versionDeployOperationActiveForJob(ctx, tx, projectID, versionID, jobID, now)
		if oerr != nil {
			return false, oerr
		}
		if opActive {
			return false, ErrJobClaimedLeaseInvariant
		}

		leaseWhere, leaseArg := jobLeaseExactMatchSQL(leaseUntil)
		workerWhere, workerArg := claimedWorkerExactMatchSQL(claimedWorker)
		query := `
UPDATE jobs SET status = 'compensating', lease_until = NULL, claimed_worker_id = NULL,
  step = CASE WHEN step LIKE ? THEN step ELSE ? END,
  updated_at = ?
WHERE id = ? AND status = 'claimed' AND step LIKE ? AND claim_epoch = ? AND ` + workerWhere + ` AND ` + leaseWhere
		args := []any{compensatingStepPrefix + "%", compensatingStepPrefix + "desire", nowStr,
			jobID, compensatingStepPrefix + "%", claimEpoch}
		if workerArg != nil {
			args = append(args, workerArg)
		}
		if leaseArg != nil {
			args = append(args, leaseArg)
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return false, nil
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	})
}

// ReleaseClaimedJobToPending releases a worker claim back to pending when deploy cannot proceed.
func (s *SQLiteStore) ReleaseClaimedJobToPending(ctx context.Context, workerID string, attempt DeployAttempt) error {
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	workerID = strings.TrimSpace(workerID)
	if attempt.JobID == "" || workerID == "" {
		return ErrJobLeaseConflict
	}
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
UPDATE jobs SET status = 'pending', lease_until = NULL, claimed_worker_id = NULL, updated_at = ?
WHERE id = ? AND claimed_worker_id = ? AND claim_epoch = ? AND status = 'claimed'`,
			now, attempt.JobID, workerID, attempt.ClaimEpoch)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrJobLeaseConflict
		}
		return nil
	})
}

// FailJobForAttempt marks a job failed only when the claim generation still matches.
func (s *SQLiteStore) FailJobForAttempt(ctx context.Context, attempt DeployAttempt) error {
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	if attempt.JobID == "" {
		return ErrJobLeaseConflict
	}
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
UPDATE jobs SET status = 'failed', lease_until = NULL, claimed_worker_id = NULL, updated_at = ?
WHERE id = ? AND claim_epoch = ? AND status IN ('claimed', 'compensating')`,
			now, attempt.JobID, attempt.ClaimEpoch)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrJobLeaseConflict
		}
		return nil
	})
}

// MarkJobCompensatingForAttempt moves a claimed job into compensating when the attempt still matches.
func (s *SQLiteStore) MarkJobCompensatingForAttempt(ctx context.Context, workerID string, attempt DeployAttempt) error {
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	workerID = strings.TrimSpace(workerID)
	if attempt.JobID == "" || workerID == "" {
		return ErrJobLeaseConflict
	}
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
UPDATE jobs SET status = 'compensating', lease_until = NULL, claimed_worker_id = NULL,
  step = CASE WHEN step LIKE ? THEN step ELSE ? END,
  updated_at = ?
WHERE id = ? AND claimed_worker_id = ? AND claim_epoch = ? AND status = 'claimed'`,
			compensatingStepPrefix+"%", compensatingStepPrefix+"desire", now,
			attempt.JobID, workerID, attempt.ClaimEpoch)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrJobLeaseConflict
		}
		return nil
	})
}

// MarkJobCompensating is a compatibility helper for tests and discovery setup only.
// It refuses to touch jobs with an active worker claim (another epoch may own the row).
func (s *SQLiteStore) MarkJobCompensating(ctx context.Context, jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("job not found")
	}
	return withRetryErr(func() error {
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var status string
		var leaseUntil sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT status, lease_until FROM jobs WHERE id = ?`, jobID).Scan(&status, &leaseUntil)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("job not found")
		}
		if err != nil {
			return err
		}
		if status == "claimed" && jobWorkerLeaseIsLive(leaseUntil, now) {
			return ErrJobLeaseConflict
		}
		res, err := tx.ExecContext(ctx, `
UPDATE jobs SET status = 'compensating', lease_until = NULL, claimed_worker_id = NULL,
  step = CASE WHEN step LIKE ? THEN step ELSE ? END,
  updated_at = ?
WHERE id = ?
  AND status IN ('pending', 'failed', 'compensating', 'claimed')`,
			compensatingStepPrefix+"%", compensatingStepPrefix+"desire", nowStr, jobID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrJobLeaseConflict
		}
		return tx.Commit()
	})
}

func (s *SQLiteStore) ClaimCompensatingJob(ctx context.Context, workerID string, lease time.Duration) (*Job, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || lease <= 0 {
		return nil, errors.New("invalid compensating claim")
	}
	return withRetry(func() (*Job, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		now := time.Now().UTC()
		phase1Scanned := 0
		phase2Scanned := 0
		takeoverOffset := loadCompensatingTakeoverScanOffset(&s.compensatingTakeoverScanOffset)

		// Phase 1: directly claimable compensating rows (not blocked by claimed+invalid takeover rules).
		for offset := 0; phase1Scanned < compensatingClaimMaxScanPerPhase; offset += compensatingClaimCandidateBatch {
			remaining := compensatingClaimMaxScanPerPhase - phase1Scanned
			if remaining <= 0 {
				break
			}
			limit := compensatingClaimCandidateBatch
			if limit > remaining {
				limit = remaining
			}
			claimed, n, cerr := scanCompensatingClaimCandidates(ctx, tx, workerID, lease, now, `
SELECT `+jobSelectCols+`
FROM jobs
WHERE status = 'compensating'
ORDER BY updated_at, id
LIMIT ? OFFSET ?`, limit, offset)
			if cerr != nil {
				return nil, cerr
			}
			phase1Scanned += n
			if claimed != nil {
				if err := tx.Commit(); err != nil {
					return nil, err
				}
				return claimed, nil
			}
			if n < limit {
				break
			}
		}

		// Phase 2: claimed compensating takeover candidates with a rotating scan cursor (no fixed head-of-line window).
		for phase2Scanned < compensatingClaimMaxScanPerPhase {
			remaining := compensatingClaimMaxScanPerPhase - phase2Scanned
			limit := compensatingClaimCandidateBatch
			if limit > remaining {
				limit = remaining
			}
			claimed, n, cerr := scanCompensatingClaimCandidates(ctx, tx, workerID, lease, now, `
SELECT `+jobSelectCols+`
FROM jobs
WHERE status = 'claimed' AND step LIKE ?
ORDER BY updated_at, id
LIMIT ? OFFSET ?`, compensatingStepPrefix+"%", limit, takeoverOffset)
			if cerr != nil {
				return nil, cerr
			}
			phase2Scanned += n
			if claimed != nil {
				nextTakeoverOffset := takeoverOffset + n
				if err := commitCompensatingClaimAndStoreTakeoverCursor(tx, &s.compensatingTakeoverScanOffset, nextTakeoverOffset); err != nil {
					return nil, err
				}
				return claimed, nil
			}
			if n == 0 {
				if takeoverOffset == 0 {
					break
				}
				// Publish wrap when global cursor still matches this scan; always rescan from head locally
				// so this transaction makes progress even if another goroutine moved the cursor first.
				tryWrapCompensatingTakeoverScanOffset(&s.compensatingTakeoverScanOffset, takeoverOffset)
				takeoverOffset = 0
				continue
			}
			takeoverOffset += n
		}
		// Read-only scan (tx rolls back via defer); cursor still advances so later calls rotate fairly.
		storeCompensatingTakeoverScanOffset(&s.compensatingTakeoverScanOffset, takeoverOffset)
		return nil, nil
	})
}

// commitCompensatingClaimAndStoreTakeoverCursor commits the compensating claim transaction and only then
// advances the Phase-2 takeover scan cursor.
func commitCompensatingClaimAndStoreTakeoverCursor(tx *sql.Tx, counter *atomic.Int64, nextTakeoverOffset int) error {
	if err := tx.Commit(); err != nil {
		return err
	}
	storeCompensatingTakeoverScanOffset(counter, nextTakeoverOffset)
	return nil
}

func scanCompensatingClaimCandidates(ctx context.Context, tx *sql.Tx, workerID string, lease time.Duration, now time.Time, query string, args ...any) (*Job, int, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		n++
		var j Job
		var leaseUntil sql.NullString
		var updated string
		var claimedWorker sql.NullString
		if err := rows.Scan(&j.ID, &j.ProjectID, &j.VersionID, &j.Step, &j.Status, &leaseUntil, &updated, &claimedWorker, &j.ClaimEpoch); err != nil {
			return nil, n, err
		}
		if claimedWorker.Valid {
			j.ClaimedWorkerID = strings.TrimSpace(claimedWorker.String)
		}
		j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)

		got, ok, cerr := tryClaimCompensatingJobCandidate(ctx, tx, workerID, lease, now, &j)
		if cerr != nil {
			return nil, n, cerr
		}
		if ok {
			return got, n, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, n, err
	}
	return nil, n, nil
}

func tryClaimCompensatingJobCandidate(ctx context.Context, tx *sql.Tx, workerID string, lease time.Duration, now time.Time, j *Job) (*Job, bool, error) {
	if j == nil {
		return nil, false, nil
	}
	nowStr := now.Format(time.RFC3339Nano)

	active, aerr := jobHasActiveWorkerClaim(ctx, tx, j.ID, now)
	if aerr != nil {
		return nil, false, aerr
	}
	if active {
		return nil, false, nil
	}

	takeoverFromClaimed := j.Status == "claimed" && strings.HasPrefix(j.Step, compensatingStepPrefix)
	if j.Status != "compensating" && !takeoverFromClaimed {
		return nil, false, nil
	}
	if j.Status == "compensating" && compensatingDirectClaimAttemptHook != nil && compensatingDirectClaimAttemptHook(j) {
		return nil, false, nil
	}
	heldEpoch := j.ClaimEpoch
	until := now.Add(lease)
	var leaseRaw sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT lease_until FROM jobs WHERE id = ?`, j.ID).Scan(&leaseRaw); err != nil {
		return nil, false, err
	}
	if takeoverFromClaimed {
		if jobWorkerLeaseClaimTakeoverBlocked(leaseRaw) {
			return nil, false, nil
		}
		opActive, oerr := versionDeployOperationActiveForJob(ctx, tx, j.ProjectID, j.VersionID, j.ID, now)
		if oerr != nil {
			return nil, false, oerr
		}
		if opActive {
			return nil, false, nil
		}
	}
	leaseWhere, leaseArg := jobLeaseExactMatchSQL(leaseRaw)
	updateSQL := `
UPDATE jobs SET status = 'claimed',
  step = CASE WHEN step LIKE ? THEN step ELSE ? END,
  lease_until = ?, updated_at = ?, claimed_worker_id = ?,
  claim_epoch = claim_epoch + 1
WHERE id = ? AND claim_epoch = ? AND (
  status = 'compensating'
  OR (status = 'claimed' AND step LIKE ? AND ` + leaseWhere + `)
)`
	args := []any{
		compensatingStepPrefix + "%", compensatingStepPrefix + "desire",
		until.Format(time.RFC3339Nano), nowStr, workerID, j.ID, heldEpoch,
		compensatingStepPrefix + "%",
	}
	if leaseArg != nil {
		args = append(args, leaseArg)
	}
	res, err := tx.ExecContext(ctx, updateSQL, args...)
	if err != nil {
		return nil, false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, false, nil
	}
	j.ClaimEpoch++
	j.Status = "claimed"
	j.Step = normalizeCompensatingStep(j.Step)
	j.LeaseUntil = &until
	j.ClaimedWorkerID = workerID
	j.UpdatedAt = now
	return j, true, nil
}

func normalizeCompensatingStep(step string) string {
	if strings.HasPrefix(step, compensatingStepPrefix) {
		return step
	}
	return compensatingStepPrefix + "desire"
}

// compensatingTakeoverScanOffset is a process-local advisory fairness hint for Phase-2 scan rotation.
// It is not durable state: claim correctness is decided by SQLite CAS in tryClaimCompensatingJobCandidate.
// If the process exits after a successful commit but before the in-memory cursor is stored, restart resets
// the cursor to zero—only repeating scan work, never double-claiming or permanent head-of-line blocking.
func loadCompensatingTakeoverScanOffset(counter *atomic.Int64) int {
	v := counter.Load()
	if v < 0 {
		return 0
	}
	return int(v)
}

func compensatingTakeoverScanOffsetToInt64(offset int) int64 {
	if offset < 0 {
		return 0
	}
	return int64(offset)
}

// storeCompensatingTakeoverScanOffset advances the Phase-2 cursor with a monotonic max (never regresses).
// The cursor is a process-local advisory fairness hint only; claim correctness remains SQLite CAS.
// Wrap-to-zero is only via tryWrapCompensatingTakeoverScanOffset.
func storeCompensatingTakeoverScanOffset(counter *atomic.Int64, offset int) {
	newVal := compensatingTakeoverScanOffsetToInt64(offset)
	for {
		cur := counter.Load()
		if cur < 0 {
			cur = 0
		}
		if newVal <= cur {
			return
		}
		if counter.CompareAndSwap(cur, newVal) {
			return
		}
	}
}

// tryWrapCompensatingTakeoverScanOffset resets the cursor to 0 only when it still equals scanStart
// (end-of-list for this scan). If another goroutine advanced the cursor, the CAS fails and no reset occurs.
func tryWrapCompensatingTakeoverScanOffset(counter *atomic.Int64, scanStart int) bool {
	if scanStart <= 0 {
		return false
	}
	expected := compensatingTakeoverScanOffsetToInt64(scanStart)
	return counter.CompareAndSwap(expected, 0)
}

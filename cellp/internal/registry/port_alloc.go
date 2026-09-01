package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const portAllocSelectCols = `allocation_id, port, purpose, stability, owner_kind, owner_id, project_id, gateway_id, created_at, released_at, release_reason`

func (s *SQLiteStore) ingressPortBounds() (min, max int) {
	return s.ingressPortMin, s.ingressPortMax
}

func validateIngressPort(p, min, max int) error {
	if p < min || p > max {
		return fmt.Errorf("%w: port %d outside [%d,%d]", ErrPortInvalid, p, min, max)
	}
	return nil
}

func normalizeAllocateIngressInput(in *AllocateIngressListenPortInput) error {
	if in == nil {
		return fmt.Errorf("%w: nil input", ErrPortAllocationInputInvalid)
	}
	in.OwnerKind = strings.TrimSpace(in.OwnerKind)
	in.OwnerID = strings.TrimSpace(in.OwnerID)
	in.Stability = strings.TrimSpace(in.Stability)
	if in.OwnerKind == "" {
		in.OwnerKind = PortOwnerIngressBinding
	}
	if in.OwnerKind != PortOwnerIngressBinding {
		return fmt.Errorf("%w: owner_kind must be ingress_binding", ErrPortAllocationInputInvalid)
	}
	if in.OwnerID == "" {
		return fmt.Errorf("%w: owner_id required", ErrPortAllocationInputInvalid)
	}
	if in.Stability == "" {
		in.Stability = PortStabilityEphemeral
	}
	if in.Stability != PortStabilityEphemeral && in.Stability != PortStabilityStable {
		return fmt.Errorf("%w: invalid stability", ErrPortAllocationInputInvalid)
	}
	return nil
}

func isPortAllocUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed")
}

func scanPortAllocation(row scanner) (*PortAllocation, error) {
	var pa PortAllocation
	var projectID, gatewayID, releasedAt, releaseReason sql.NullString
	var created string
	if err := row.Scan(
		&pa.AllocationID, &pa.Port, &pa.Purpose, &pa.Stability, &pa.OwnerKind, &pa.OwnerID,
		&projectID, &gatewayID, &created, &releasedAt, &releaseReason,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if projectID.Valid {
		v := projectID.String
		pa.ProjectID = &v
	}
	if gatewayID.Valid {
		v := gatewayID.String
		pa.GatewayID = &v
	}
	pa.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if releasedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, releasedAt.String)
		pa.ReleasedAt = &t
	}
	if releaseReason.Valid {
		v := releaseReason.String
		pa.ReleaseReason = &v
	}
	return &pa, nil
}

func (s *SQLiteStore) getActivePortAllocationByOwnerDB(ctx context.Context, db queryRowContext, ownerKind, ownerID, purpose string) (*PortAllocation, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+portAllocSelectCols+` FROM port_allocations
		WHERE owner_kind = ? AND owner_id = ? AND purpose = ? AND released_at IS NULL`,
		ownerKind, ownerID, purpose)
	return scanPortAllocation(row)
}

type queryRowContext interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *SQLiteStore) AllocateIngressListenPort(ctx context.Context, in AllocateIngressListenPortInput) (*PortAllocation, error) {
	return withRetry(func() (*PortAllocation, error) {
		if err := normalizeAllocateIngressInput(&in); err != nil {
			return nil, err
		}
		existing, err := s.getActivePortAllocationByOwnerDB(ctx, s.db, in.OwnerKind, in.OwnerID, PortPurposeIngressListen)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
		minP, maxP := s.ingressPortBounds()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for port := minP; port <= maxP; port++ {
			pa := &PortAllocation{
				AllocationID: uuid.NewString(),
				Port:         port,
				Purpose:      PortPurposeIngressListen,
				Stability:    in.Stability,
				OwnerKind:    in.OwnerKind,
				OwnerID:      in.OwnerID,
				ProjectID:    in.ProjectID,
				GatewayID:    in.GatewayID,
				CreatedAt:    time.Now().UTC(),
			}
			_, err := s.db.ExecContext(ctx, `
				INSERT INTO port_allocations (
					allocation_id, port, purpose, stability, owner_kind, owner_id,
					project_id, gateway_id, created_at, released_at, release_reason
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
				pa.AllocationID, pa.Port, pa.Purpose, pa.Stability, pa.OwnerKind, pa.OwnerID,
				nullStr(pa.ProjectID), nullStr(pa.GatewayID), now)
			if err != nil {
				if isPortAllocUniqueViolation(err) {
					continue
				}
				return nil, err
			}
			pa.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
			return pa, nil
		}
		return nil, ErrPortPoolExhausted
	})
}

func (s *SQLiteStore) ReserveStablePort(ctx context.Context, in ReserveStablePortInput) (*PortAllocation, error) {
	return withRetry(func() (*PortAllocation, error) {
		in.OwnerKind = strings.TrimSpace(in.OwnerKind)
		in.OwnerID = strings.TrimSpace(in.OwnerID)
		if in.OwnerKind == "" {
			in.OwnerKind = PortOwnerIngressBinding
		}
		if in.OwnerKind != PortOwnerIngressBinding {
			return nil, fmt.Errorf("%w: owner_kind must be ingress_binding", ErrPortAllocationInputInvalid)
		}
		if in.OwnerID == "" {
			return nil, fmt.Errorf("%w: owner_id required", ErrPortAllocationInputInvalid)
		}
		minP, maxP := s.ingressPortBounds()
		if err := validateIngressPort(in.Port, minP, maxP); err != nil {
			return nil, err
		}

		byOwner, err := s.getActivePortAllocationByOwnerDB(ctx, s.db, in.OwnerKind, in.OwnerID, PortPurposeIngressListen)
		if err != nil {
			return nil, err
		}
		if byOwner != nil {
			if byOwner.Port == in.Port {
				return byOwner, nil
			}
			return nil, ErrPortConflict
		}

		row := s.db.QueryRowContext(ctx, `
			SELECT `+portAllocSelectCols+` FROM port_allocations
			WHERE port = ? AND released_at IS NULL`, in.Port)
		onPort, err := scanPortAllocation(row)
		if err != nil {
			return nil, err
		}
		if onPort != nil && onPort.OwnerID != in.OwnerID {
			return nil, ErrPortConflict
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		pa := &PortAllocation{
			AllocationID: uuid.NewString(),
			Port:         in.Port,
			Purpose:      PortPurposeIngressListen,
			Stability:    PortStabilityStable,
			OwnerKind:    in.OwnerKind,
			OwnerID:      in.OwnerID,
			ProjectID:    in.ProjectID,
			GatewayID:    in.GatewayID,
		}
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO port_allocations (
				allocation_id, port, purpose, stability, owner_kind, owner_id,
				project_id, gateway_id, created_at, released_at, release_reason
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
			pa.AllocationID, pa.Port, pa.Purpose, pa.Stability, pa.OwnerKind, pa.OwnerID,
			nullStr(pa.ProjectID), nullStr(pa.GatewayID), now)
		if err != nil {
			if isPortAllocUniqueViolation(err) {
				return nil, ErrPortConflict
			}
			return nil, err
		}
		pa.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
		return pa, nil
	})
}

func (s *SQLiteStore) ReleasePort(ctx context.Context, in ReleasePortInput) error {
	return withRetryErr(func() error {
		in.AllocationID = strings.TrimSpace(in.AllocationID)
		in.OwnerKind = strings.TrimSpace(in.OwnerKind)
		in.OwnerID = strings.TrimSpace(in.OwnerID)
		now := time.Now().UTC().Format(time.RFC3339Nano)

		if in.AllocationID != "" {
			res, err := s.db.ExecContext(ctx, `
				UPDATE port_allocations SET released_at = ?, release_reason = ?
				WHERE allocation_id = ? AND released_at IS NULL`,
				now, nullStrNonEmpty(in.ReleaseReason), in.AllocationID)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return ErrPortAllocationNotFound
			}
			return nil
		}
		if in.OwnerKind == "" || in.OwnerID == "" {
			return fmt.Errorf("%w: allocation_id or owner required", ErrPortAllocationInputInvalid)
		}
		res, err := s.db.ExecContext(ctx, `
			UPDATE port_allocations SET released_at = ?, release_reason = ?
			WHERE owner_kind = ? AND owner_id = ? AND purpose = ? AND released_at IS NULL`,
			now, nullStrNonEmpty(in.ReleaseReason), in.OwnerKind, in.OwnerID, PortPurposeIngressListen)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrPortAllocationNotFound
		}
		return nil
	})
}

func nullStrNonEmpty(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func (s *SQLiteStore) GetActivePortAllocationByOwner(ctx context.Context, ownerKind, ownerID string) (*PortAllocation, error) {
	return withRetry(func() (*PortAllocation, error) {
		return s.getActivePortAllocationByOwnerDB(ctx, s.db, strings.TrimSpace(ownerKind), strings.TrimSpace(ownerID), PortPurposeIngressListen)
	})
}

func (s *SQLiteStore) ListActivePortAllocations(ctx context.Context, purpose string) ([]PortAllocation, error) {
	return withRetry(func() ([]PortAllocation, error) {
		purpose = strings.TrimSpace(purpose)
		if purpose == PortPurposeCelldUpstream {
			return nil, ErrPortPurposeNotSupported
		}
		if purpose == "" {
			purpose = PortPurposeIngressListen
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT `+portAllocSelectCols+` FROM port_allocations
			WHERE purpose = ? AND released_at IS NULL ORDER BY port`, purpose)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []PortAllocation
		for rows.Next() {
			pa, err := scanPortAllocation(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, *pa)
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) allocateIngressListenPortInTx(ctx context.Context, tx *sql.Tx, in AllocateIngressListenPortInput) (*PortAllocation, error) {
	if err := normalizeAllocateIngressInput(&in); err != nil {
		return nil, err
	}
	existing, err := s.getActivePortAllocationByOwnerDB(ctx, tx, in.OwnerKind, in.OwnerID, PortPurposeIngressListen)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	minP, maxP := s.ingressPortBounds()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for port := minP; port <= maxP; port++ {
		pa := &PortAllocation{
			AllocationID: uuid.NewString(),
			Port:         port,
			Purpose:      PortPurposeIngressListen,
			Stability:    in.Stability,
			OwnerKind:    in.OwnerKind,
			OwnerID:      in.OwnerID,
			ProjectID:    in.ProjectID,
			GatewayID:    in.GatewayID,
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO port_allocations (
				allocation_id, port, purpose, stability, owner_kind, owner_id,
				project_id, gateway_id, created_at, released_at, release_reason
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
			pa.AllocationID, pa.Port, pa.Purpose, pa.Stability, pa.OwnerKind, pa.OwnerID,
			nullStr(pa.ProjectID), nullStr(pa.GatewayID), now)
		if err != nil {
			if isPortAllocUniqueViolation(err) {
				continue
			}
			return nil, err
		}
		pa.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
		return pa, nil
	}
	return nil, ErrPortPoolExhausted
}

func (s *SQLiteStore) releasePortInTx(ctx context.Context, tx *sql.Tx, in ReleasePortInput) error {
	in.AllocationID = strings.TrimSpace(in.AllocationID)
	in.OwnerKind = strings.TrimSpace(in.OwnerKind)
	in.OwnerID = strings.TrimSpace(in.OwnerID)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if in.AllocationID != "" {
		res, err := tx.ExecContext(ctx, `
			UPDATE port_allocations SET released_at = ?, release_reason = ?
			WHERE allocation_id = ? AND released_at IS NULL`,
			now, nullStrNonEmpty(in.ReleaseReason), in.AllocationID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrPortAllocationNotFound
		}
		return nil
	}
	if in.OwnerKind == "" || in.OwnerID == "" {
		return fmt.Errorf("%w: allocation_id or owner required", ErrPortAllocationInputInvalid)
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE port_allocations SET released_at = ?, release_reason = ?
		WHERE owner_kind = ? AND owner_id = ? AND purpose = ? AND released_at IS NULL`,
		now, nullStrNonEmpty(in.ReleaseReason), in.OwnerKind, in.OwnerID, PortPurposeIngressListen)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPortAllocationNotFound
	}
	return nil
}

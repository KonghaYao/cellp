package registry

import (
	"context"
	"database/sql"
)

// AttachIngressListenPort allocates ingress_listen ledger row and sets binding.listen_port in one transaction (R-PORT-LEDGER).
// P5b orchestrator ready path should call this instead of bare UpsertIngressBinding with listen_port.
func (s *SQLiteStore) AttachIngressListenPort(ctx context.Context, binding IngressBinding, in AllocateIngressListenPortInput) error {
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if in.OwnerID == "" {
			in.OwnerID = binding.BindingID
		}
		if in.ProjectID == nil && binding.ProjectID != "" {
			pid := binding.ProjectID
			in.ProjectID = &pid
		}
		if in.GatewayID == nil {
			in.GatewayID = binding.OwnerGatewayID
		}

		pa, err := s.allocateIngressListenPortInTx(ctx, tx, in)
		if err != nil {
			return err
		}
		port := pa.Port
		binding.ListenPort = &port
		if pa.GatewayID != nil {
			binding.OwnerGatewayID = pa.GatewayID
		} else if in.GatewayID != nil {
			binding.OwnerGatewayID = in.GatewayID
		}
		if err := validateIngressBinding(&binding); err != nil {
			return err
		}
		if err := upsertIngressBindingExec(ctx, tx, binding); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// DetachIngressListenPort releases the ledger row for binding_id and clears listen_port on the binding.
func (s *SQLiteStore) DetachIngressListenPort(ctx context.Context, bindingID, releaseReason string) error {
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		releaseErr := s.releasePortInTx(ctx, tx, ReleasePortInput{
			OwnerKind:     PortOwnerIngressBinding,
			OwnerID:       bindingID,
			ReleaseReason: releaseReason,
		})

		b, err := getIngressBindingTx(ctx, tx, bindingID)
		if err != nil {
			return err
		}
		if b == nil {
			if releaseErr == ErrPortAllocationNotFound {
				return ErrPortAllocationNotFound
			}
			if releaseErr != nil {
				return releaseErr
			}
			return tx.Commit()
		}
		if b.Host != nil {
			b.ListenPort = nil
		} else {
			b.Active = false
		}
		if err := upsertIngressBindingExec(ctx, tx, *b); err != nil {
			return err
		}
		if releaseErr != nil && releaseErr != ErrPortAllocationNotFound {
			return releaseErr
		}
		return tx.Commit()
	})
}

type execContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func upsertIngressBindingExec(ctx context.Context, db execContext, b IngressBinding) error {
	if err := validateIngressBinding(&b); err != nil {
		return err
	}
	active := 0
	if b.Active {
		active = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO ingress_bindings (
			binding_id, project_id, version_id, role, host, listen_port, synthetic_host, owner_gateway_id, active
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(binding_id) DO UPDATE SET
			project_id = excluded.project_id,
			version_id = excluded.version_id,
			role = excluded.role,
			host = excluded.host,
			listen_port = excluded.listen_port,
			synthetic_host = excluded.synthetic_host,
			owner_gateway_id = excluded.owner_gateway_id,
			active = excluded.active`,
		b.BindingID, b.ProjectID, nullStr(b.VersionID), b.Role,
		nullStr(b.Host), nullInt(b.ListenPort), b.SyntheticHost, nullStr(b.OwnerGatewayID), active)
	if isIngressUniqueViolation(err) {
		return ErrIngressBindingConflict
	}
	return err
}

func getIngressBindingTx(ctx context.Context, tx queryRowContext, bindingID string) (*IngressBinding, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT `+ingressSelectCols+` FROM ingress_bindings WHERE binding_id = ?`, bindingID)
	return scanIngressBinding(row)
}

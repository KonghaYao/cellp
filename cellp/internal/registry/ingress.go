package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
)

var (
	// ErrIngressBindingInvalid is returned when binding fields fail validation.
	ErrIngressBindingInvalid = errors.New("ingress binding invalid")
	// ErrIngressBindingConflict is returned on unique constraint violation (host, synthetic_host, listen_port).
	ErrIngressBindingConflict = errors.New("ingress binding conflict")
)

// NormalizeIngressHost lowercases Host authority and strips a trailing :port if present.
func NormalizeIngressHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

func validateIngressBinding(b *IngressBinding) error {
	if b == nil {
		return fmt.Errorf("%w: nil binding", ErrIngressBindingInvalid)
	}
	b.BindingID = strings.TrimSpace(b.BindingID)
	b.ProjectID = strings.TrimSpace(b.ProjectID)
	b.Role = strings.TrimSpace(b.Role)
	b.SyntheticHost = NormalizeIngressHost(b.SyntheticHost)
	if b.Host != nil {
		n := NormalizeIngressHost(*b.Host)
		if n == "" {
			b.Host = nil
		} else {
			b.Host = &n
		}
	}
	if b.BindingID == "" || b.ProjectID == "" || b.SyntheticHost == "" {
		return fmt.Errorf("%w: binding_id, project_id, synthetic_host required", ErrIngressBindingInvalid)
	}
	if b.Role != IngressRolePreview && b.Role != IngressRoleProd {
		return fmt.Errorf("%w: role must be preview or prod", ErrIngressBindingInvalid)
	}
	if b.Host == nil && b.ListenPort == nil {
		return fmt.Errorf("%w: host or listen_port required", ErrIngressBindingInvalid)
	}
	if b.Role == IngressRolePreview {
		if b.VersionID == nil || strings.TrimSpace(*b.VersionID) == "" {
			return fmt.Errorf("%w: preview requires version_id", ErrIngressBindingInvalid)
		}
		v := strings.TrimSpace(*b.VersionID)
		b.VersionID = &v
	}
	if b.ListenPort != nil && *b.ListenPort <= 0 {
		return fmt.Errorf("%w: listen_port must be positive", ErrIngressBindingInvalid)
	}
	if b.OwnerGatewayID != nil {
		o := strings.TrimSpace(*b.OwnerGatewayID)
		if o == "" {
			b.OwnerGatewayID = nil
		} else {
			b.OwnerGatewayID = &o
		}
	}
	return nil
}

func isIngressUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed")
}

func scanIngressBinding(row scanner) (*IngressBinding, error) {
	var b IngressBinding
	var versionID, host, owner sql.NullString
	var listenPort sql.NullInt64
	var active int
	if err := row.Scan(
		&b.BindingID, &b.ProjectID, &versionID, &b.Role, &host, &listenPort,
		&b.SyntheticHost, &owner, &active,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if versionID.Valid {
		v := versionID.String
		b.VersionID = &v
	}
	if host.Valid {
		h := host.String
		b.Host = &h
	}
	if listenPort.Valid {
		p := int(listenPort.Int64)
		b.ListenPort = &p
	}
	if owner.Valid {
		o := owner.String
		b.OwnerGatewayID = &o
	}
	b.Active = active == 1
	return &b, nil
}

type scanner interface {
	Scan(dest ...any) error
}

const ingressSelectCols = `binding_id, project_id, version_id, role, host, listen_port, synthetic_host, owner_gateway_id, active`

func (s *SQLiteStore) UpsertIngressBinding(ctx context.Context, b IngressBinding) error {
	if err := validateIngressBinding(&b); err != nil {
		return err
	}
	return withRetryErr(func() error {
		active := 0
		if b.Active {
			active = 1
		}
		_, err := s.db.ExecContext(ctx, `
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
	})
}

func (s *SQLiteStore) GetIngressBinding(ctx context.Context, bindingID string) (*IngressBinding, error) {
	return withRetry(func() (*IngressBinding, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT `+ingressSelectCols+` FROM ingress_bindings WHERE binding_id = ?`, bindingID)
		return scanIngressBinding(row)
	})
}

func (s *SQLiteStore) LookupIngressByHost(ctx context.Context, host string) (*IngressBinding, error) {
	h := NormalizeIngressHost(host)
	if h == "" {
		return nil, nil
	}
	return withRetry(func() (*IngressBinding, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT `+ingressSelectCols+` FROM ingress_bindings WHERE active = 1 AND host = ?`, h)
		return scanIngressBinding(row)
	})
}

func (s *SQLiteStore) LookupIngressByListenPort(ctx context.Context, listenPort int, ownerGatewayID string) (*IngressBinding, error) {
	ownerGatewayID = strings.TrimSpace(ownerGatewayID)
	if listenPort <= 0 || ownerGatewayID == "" {
		return nil, nil
	}
	return withRetry(func() (*IngressBinding, error) {
		row := s.db.QueryRowContext(ctx,
			`SELECT `+ingressSelectCols+` FROM ingress_bindings
			 WHERE active = 1 AND listen_port = ? AND owner_gateway_id = ?`,
			listenPort, ownerGatewayID)
		return scanIngressBinding(row)
	})
}

func (s *SQLiteStore) SetIngressBindingActive(ctx context.Context, bindingID string, active bool) error {
	return withRetryErr(func() error {
		activeInt := 0
		if active {
			activeInt = 1
		}
		res, err := s.db.ExecContext(ctx,
			`UPDATE ingress_bindings SET active = ? WHERE binding_id = ?`, activeInt, bindingID)
		if err != nil {
			if isIngressUniqueViolation(err) {
				return ErrIngressBindingConflict
			}
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("ingress binding not found")
		}
		return nil
	})
}

func (s *SQLiteStore) ListActiveIngressBindings(ctx context.Context) ([]IngressBinding, error) {
	return withRetry(func() ([]IngressBinding, error) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+ingressSelectCols+` FROM ingress_bindings WHERE active = 1 ORDER BY project_id, binding_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanIngressBindings(rows)
	})
}

func (s *SQLiteStore) ListIngressBindingsByVersion(ctx context.Context, projectID, versionID string) ([]IngressBinding, error) {
	return withRetry(func() ([]IngressBinding, error) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+ingressSelectCols+` FROM ingress_bindings
			 WHERE project_id = ? AND version_id = ? ORDER BY binding_id`,
			projectID, versionID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanIngressBindings(rows)
	})
}

func scanIngressBindings(rows *sql.Rows) ([]IngressBinding, error) {
	var out []IngressBinding
	for rows.Next() {
		b, err := scanIngressBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func nullInt(n *int) interface{} {
	if n == nil {
		return nil
	}
	return *n
}

package registry

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func bumpRouteRevisionInTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE control_plane_meta SET route_revision = route_revision + 1 WHERE id = 1`)
	return err
}

func parseRFC3339NanoRequired(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("timestamp required")
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp corrupt")
	}
	return t.UTC(), nil
}

func assignmentLeaseEqual(a, b time.Time) bool {
	return a.UTC().Format(time.RFC3339Nano) == b.UTC().Format(time.RFC3339Nano)
}

// routingLKGValidUntil is the gateway-facing lease horizon: earliest of node, assignment, and endpoint expiry.
func routingLKGValidUntil(nodeExpiry, assignUntil, endpointUntil time.Time) time.Time {
	lkg := nodeExpiry
	if assignUntil.Before(lkg) {
		lkg = assignUntil
	}
	if endpointUntil.Before(lkg) {
		lkg = endpointUntil
	}
	return lkg.UTC()
}

func ingressBindingRoutingEqual(a, b *IngressBinding) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.BindingID != b.BindingID || a.ProjectID != b.ProjectID || a.Role != b.Role ||
		a.SyntheticHost != b.SyntheticHost || a.Active != b.Active {
		return false
	}
	if !nullStringEqual(a.VersionID, b.VersionID) || !nullStringEqual(a.Host, b.Host) ||
		!nullIntEqual(a.ListenPort, b.ListenPort) || !nullStringEqual(a.OwnerGatewayID, b.OwnerGatewayID) {
		return false
	}
	return true
}

func nullStringEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func nullIntEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

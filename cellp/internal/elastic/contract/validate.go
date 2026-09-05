package contract

import (
	"fmt"
	"strings"
	"time"
)

// ValidateServingPolicy enforces min/max and fail-closed background unknown.
func ValidateServingPolicy(p ServingPolicy) error {
	if p.MinReplicas < 0 || p.MaxReplicas < 0 {
		return fmt.Errorf("replica bounds must be non-negative")
	}
	if p.MaxReplicas < p.MinReplicas {
		return fmt.Errorf("max_replicas < min_replicas")
	}
	switch p.BackgroundMode {
	case BackgroundModeNone, BackgroundModeResidentRequired, BackgroundModeUnknown:
	default:
		return fmt.Errorf("invalid background_mode: %q", p.BackgroundMode)
	}
	if p.BackgroundMode == BackgroundModeUnknown {
		return fmt.Errorf("background_mode unknown is fail-closed")
	}
	return nil
}

// ValidateRouteSnapshot rejects empty first snapshot and revision regression.
func ValidateRouteSnapshot(prevRevision int64, snap RouteSnapshot, now time.Time) error {
	if snap.Revision <= 0 {
		return fmt.Errorf("revision must be positive")
	}
	if prevRevision > 0 && snap.Revision <= prevRevision {
		return fmt.Errorf("revision regression: %d after %d", snap.Revision, prevRevision)
	}
	for _, es := range snap.EndpointSets {
		for _, ep := range es.Endpoints {
			if ep.State == EndpointReady {
				if ep.ValidUntil != nil && !ep.ValidUntil.After(now) {
					return fmt.Errorf("expired ready endpoint %s", ep.ReplicaID)
				}
			}
			if strings.TrimSpace(ep.Address) == "" {
				return fmt.Errorf("empty endpoint address")
			}
		}
	}
	return nil
}

// ValidateWorkloadManifest fail-closed on unknown capabilities.
func ValidateWorkloadManifest(m WorkloadManifest) error {
	for name, cap := range m.Capabilities {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("empty capability name")
		}
		switch cap {
		case CapabilityNone, CapabilityResidentRequired, CapabilityUnknown:
			if cap == CapabilityUnknown {
				return fmt.Errorf("capability %q is unknown", name)
			}
		default:
			return fmt.Errorf("capability %q invalid value %q", name, cap)
		}
	}
	return nil
}

// ValidateCommandScope ensures fencing fields are present for mutating actions.
func ValidateCommandScope(s CommandScope) error {
	if strings.TrimSpace(s.NodeID) == "" || strings.TrimSpace(s.ProjectID) == "" || strings.TrimSpace(s.VersionID) == "" {
		return fmt.Errorf("node_id, project_id, version_id required")
	}
	if s.Generation <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	if s.LeaseExpiry.IsZero() {
		return fmt.Errorf("lease_expiry required")
	}
	if strings.TrimSpace(s.Nonce) == "" {
		return fmt.Errorf("nonce required")
	}
	return nil
}

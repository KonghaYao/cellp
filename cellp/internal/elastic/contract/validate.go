package contract

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	MaxCommandNonceLen            = 128
	MaxCommandFieldLen            = 128
	MaxCommandActionLen           = 64
	MaxRuntimeNodeZoneLen         = 128
	MaxRuntimeNodeAgentBaseURLLen = 512
	MaxRuntimeNodeIdentityURILen  = 512
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

func validateSPIFFEIdentityURI(identity string) error {
	u, err := url.Parse(strings.TrimSpace(identity))
	if err != nil || u.Scheme != "spiffe" || u.Host == "" || u.Path == "" || u.Path == "/" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid spiffe identity uri")
	}
	if len(identity) > MaxRuntimeNodeIdentityURILen {
		return fmt.Errorf("identity uri too long")
	}
	return nil
}

// AgentBaseURLPolicy tunes agent base URL validation (production defaults reject loopback literals).
type AgentBaseURLPolicy struct {
	AllowLoopback bool
}

// ProductionAgentBaseURLPolicy is the default fail-closed SSRF posture for remote nodes.
var ProductionAgentBaseURLPolicy = AgentBaseURLPolicy{}

func validateAgentBaseURL(raw string) error {
	return validateAgentBaseURLWithPolicy(raw, ProductionAgentBaseURLPolicy)
}

func validateAgentBaseURLWithPolicy(raw string, policy AgentBaseURLPolicy) error {
	if len(raw) > MaxRuntimeNodeAgentBaseURLLen {
		return fmt.Errorf("agent base url too long")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("https agent base url required")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("agent base url components forbidden")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return fmt.Errorf("agent base url host required")
	}
	if err := validateAgentBaseURLHost(host, policy.AllowLoopback); err != nil {
		return err
	}
	return nil
}

// ValidateRuntimeNodeActivationMetadataWithPolicy requires complete node metadata under the given URL policy.
func ValidateRuntimeNodeActivationMetadataWithPolicy(n RuntimeNode, policy AgentBaseURLPolicy) error {
	zone := strings.TrimSpace(n.Zone)
	if zone == "" {
		return fmt.Errorf("zone required")
	}
	if len(zone) > MaxRuntimeNodeZoneLen {
		return fmt.Errorf("zone too long")
	}
	if err := validateAgentBaseURLWithPolicy(n.AgentBaseURL, policy); err != nil {
		return err
	}
	if err := validateSPIFFEIdentityURI(n.IdentityURI); err != nil {
		return err
	}
	return nil
}

// ValidateRuntimeNodeActivationMetadata requires complete production node metadata.
func ValidateRuntimeNodeActivationMetadata(n RuntimeNode) error {
	return ValidateRuntimeNodeActivationMetadataWithPolicy(n, ProductionAgentBaseURLPolicy)
}

// ValidateRuntimeNodeMetadataIfPresent rejects invalid metadata when any field is set (legacy all-empty rows allowed).
// URL host policy is lenient here so internal registry/test fixtures may use loopback agent listeners.
func ValidateRuntimeNodeMetadataIfPresent(n RuntimeNode) error {
	if strings.TrimSpace(n.AgentBaseURL) == "" && strings.TrimSpace(n.IdentityURI) == "" && strings.TrimSpace(n.Zone) == "" {
		return nil
	}
	zone := strings.TrimSpace(n.Zone)
	if zone == "" {
		return fmt.Errorf("zone required")
	}
	if len(zone) > MaxRuntimeNodeZoneLen {
		return fmt.Errorf("zone too long")
	}
	if err := validateAgentBaseURLLenient(n.AgentBaseURL); err != nil {
		return err
	}
	if err := validateSPIFFEIdentityURI(n.IdentityURI); err != nil {
		return err
	}
	return nil
}

func validateAgentBaseURLLenient(raw string) error {
	if len(raw) > MaxRuntimeNodeAgentBaseURLLen {
		return fmt.Errorf("agent base url too long")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("https agent base url required")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("agent base url components forbidden")
	}
	return nil
}

func boundedCommandID(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s required", field)
	}
	if len(value) > MaxCommandFieldLen {
		return fmt.Errorf("%s too long", field)
	}
	return nil
}

// ValidateCommandScope ensures fencing fields are present for mutating actions.
func ValidateCommandScope(s CommandScope) error {
	if err := boundedCommandID("node_id", s.NodeID); err != nil {
		return err
	}
	if err := boundedCommandID("project_id", s.ProjectID); err != nil {
		return err
	}
	if err := boundedCommandID("version_id", s.VersionID); err != nil {
		return err
	}
	if rid := strings.TrimSpace(s.ReplicaID); rid != "" && len(rid) > MaxCommandFieldLen {
		return fmt.Errorf("replica_id too long")
	}
	if s.Generation <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	if s.LeaseExpiry.IsZero() {
		return fmt.Errorf("lease_expiry required")
	}
	nonce := strings.TrimSpace(s.Nonce)
	if nonce == "" {
		return fmt.Errorf("nonce required")
	}
	if len(nonce) > MaxCommandNonceLen {
		return fmt.Errorf("nonce too long")
	}
	if s.Action != "" {
		if len(string(s.Action)) > MaxCommandActionLen {
			return fmt.Errorf("action too long")
		}
		switch s.Action {
		case ActionStartReplica, ActionProbeReplica, ActionDrainReplica, ActionStopReplica, ActionListReplicas:
		default:
			return fmt.Errorf("invalid action")
		}
	}
	return nil
}

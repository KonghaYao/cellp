package config

import (
	"fmt"
	"strings"
)

// Tier B ingress modes (AD-12 / INGRESS-PORT-DEPLOYMENT §2).
const (
	IngressTierHost           = "host"
	IngressTierDedicatedPort  = "dedicated_port"
	IngressTierProdPort       = "prod_port"
	IngressTierExternalMap    = "external_map"
)

var validIngressTierB = map[string]struct{}{
	IngressTierHost:          {},
	IngressTierDedicatedPort: {},
	IngressTierProdPort:      {},
	IngressTierExternalMap:   {},
}

// ValidateIngressTierB rejects unknown tier strings (empty is invalid for global config).
func ValidateIngressTierB(s string) error {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return fmt.Errorf("ingress tier B empty")
	}
	if _, ok := validIngressTierB[t]; !ok {
		return fmt.Errorf("invalid CELLP_INGRESS_TIER_B %q", s)
	}
	return nil
}

// ValidateIngressTierBOptional validates a project override; empty means inherit global.
func ValidateIngressTierBOptional(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return ValidateIngressTierB(s)
}

// TierBOrDefault returns normalized global tier (defaults to host).
func (ic IngressConfig) TierBOrDefault() string {
	t := strings.ToLower(strings.TrimSpace(ic.TierB))
	if t == "" {
		return IngressTierHost
	}
	return t
}

// EffectiveIngressTierB resolves project override against global config.
func EffectiveIngressTierB(global string, projectOverride *string) string {
	if projectOverride != nil {
		if t := strings.ToLower(strings.TrimSpace(*projectOverride)); t != "" {
			return t
		}
	}
	return strings.ToLower(strings.TrimSpace(global))
}

// Validate checks ingress-related configuration (fail-fast at cellpd boot).
func (ic IngressConfig) Validate() error {
	return ValidateIngressTierB(ic.TierBOrDefault())
}

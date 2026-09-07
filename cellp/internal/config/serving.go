package config

import (
	"os"
	"strings"

	"github.com/cellp/cellp/internal/elastic/contract"
)

const (
	envServingDefaultMinReplicas = "CELLP_SERVING_DEFAULT_MIN_REPLICAS"
	envServingMaxReplicas        = "CELLP_SERVING_MAX_REPLICAS"
	envServingIdleAfterDeploy    = "CELLP_SERVING_IDLE_AFTER_DEPLOY"
	envServingProdScaleToZero    = "CELLP_SERVING_PROD_SCALE_TO_ZERO"
	envServingScaleToZero        = "CELLP_SERVING_SCALE_TO_ZERO"
	envServingDefaultBackground  = "CELLP_SERVING_DEFAULT_BACKGROUND"
)

const (
	defaultServingMinReplicas = 0
	defaultServingMaxReplicas = 8
)

// ServingDefaults are platform-wide defaults for auto-enrolled ServingPolicy rows and deploy idle behavior.
// Per-version overrides use UpsertServingPolicy (API/dashboard future).
type ServingDefaults struct {
	DefaultMinReplicas int
	DefaultMaxReplicas int
	// IdleAfterDeploy CASes desired replicas to zero after successful elastic qualification when policy allows.
	IdleAfterDeploy bool
	// ProdScaleToZero allows prod versions to transition ready→deploy_ready when desired=0 (cold prod).
	ProdScaleToZero bool
	// ScaleToZeroEnabled when false forces default min replicas to at least 1 (platform-wide scale-to-zero off).
	ScaleToZeroEnabled bool
	DefaultBackground contract.BackgroundMode
}

// LoadServingDefaults reads serving knobs from the environment (serverless-aligned defaults).
func LoadServingDefaults() ServingDefaults {
	max := envInt(envServingMaxReplicas, defaultServingMaxReplicas)
	if max < 1 {
		max = defaultServingMaxReplicas
	}
	min := envInt(envServingDefaultMinReplicas, defaultServingMinReplicas)
	if min < 0 {
		min = defaultServingMinReplicas
	}
	scaleToZero := envBoolDefaultTrue(envServingScaleToZero)
	if !scaleToZero && min < 1 {
		min = 1
	}
	bg := parseServingBackground(envOr(envServingDefaultBackground, string(contract.BackgroundModeNone)))
	if bg == contract.BackgroundModeResidentRequired && min < 1 {
		min = 1
	}
	return ServingDefaults{
		DefaultMinReplicas: min,
		DefaultMaxReplicas: max,
		IdleAfterDeploy:    envBoolDefaultTrue(envServingIdleAfterDeploy),
		ProdScaleToZero:    envBool(envServingProdScaleToZero, false),
		ScaleToZeroEnabled: scaleToZero,
		DefaultBackground:  bg,
	}
}

// EffectiveDefaultMinReplicas returns min replicas for a new auto policy (respects scale-to-zero and background).
func (d ServingDefaults) EffectiveDefaultMinReplicas() int {
	min := d.DefaultMinReplicas
	if !d.ScaleToZeroEnabled && min < 1 {
		min = 1
	}
	if d.DefaultBackground == contract.BackgroundModeResidentRequired && min < 1 {
		min = 1
	}
	return min
}

func parseServingBackground(raw string) contract.BackgroundMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return contract.BackgroundModeNone
	case "resident_required", "resident", "always_on":
		return contract.BackgroundModeResidentRequired
	default:
		return contract.BackgroundModeNone
	}
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envBoolDefaultTrue(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

package contract

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const EnvElasticRuntime = "CELLP_ELASTIC_RUNTIME"

var errElasticRuntimeRequired = errors.New("elastic runtime is required")

// ParseElasticRuntimeEnv interprets CELLP_ELASTIC_RUNTIME.
// Unset defaults to enabled. Known on values enable. Explicit off values error.
// Any other non-empty value is an invalid value error.
func ParseElasticRuntimeEnv() (enabled bool, err error) {
	v := strings.TrimSpace(os.Getenv(EnvElasticRuntime))
	if v == "" {
		return true, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, fmt.Errorf("%w (%s=%q)", errElasticRuntimeRequired, EnvElasticRuntime, v)
	default:
		return false, fmt.Errorf("%s: invalid value %q", EnvElasticRuntime, v)
	}
}

// ElasticRuntimeEnabled reports whether cellpd uses the AD-15 elastic control plane.
func ElasticRuntimeEnabled() bool {
	return true
}

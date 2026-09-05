package contract

import (
	"os"
	"strings"
)

const EnvElasticRuntime = "CELLP_ELASTIC_RUNTIME"

// ElasticRuntimeEnabled is true only when CELLP_ELASTIC_RUNTIME is a truthy value.
// Default (unset or 0/false) keeps legacy AD-1 behavior.
func ElasticRuntimeEnabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvElasticRuntime))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Config holds cellpd runtime configuration from environment.
type Config struct {
	RegistryDB    string
	DeployToken   string
	AdminToken    string
	APIPort       int
	GatewayPort   int
	CelldBasePort int
	GatewayURL    string

	OffshootStore string

	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string

	ArtifactsBucket string
	CelldBucket     string

	ArtifactsDir string

	Ingress IngressConfig
}

// APIAddr returns the API listen address.
func (c Config) APIAddr() string {
	return fmt.Sprintf(":%d", c.APIPort)
}

// GatewayAddr returns the gateway listen address.
func (c Config) GatewayAddr() string {
	return fmt.Sprintf(":%d", c.GatewayPort)
}

func Load() Config {
	cfg := Config{
		RegistryDB:      envOr("CELLP_REGISTRY_DB", "./dev/data/cellp-registry.sqlite"),
		DeployToken:     envOr("CELLP_DEPLOY_TOKEN", envOr("PLATFORM_TOKEN", "dev-local-token")),
		AdminToken:      envOr("CELLP_ADMIN_TOKEN", envOr("PLATFORM_TOKEN", "dev-local-token")),
		APIPort:         envInt("PLATFORM_PORT", 8790),
		GatewayPort:     envInt("GATEWAY_PORT", 8787),
		CelldBasePort:   envInt("CELLD_PORT", 8792),
		GatewayURL:      envOr("GATEWAY_URL", "http://127.0.0.1:8787"),
		OffshootStore:   envOr("OFFSHOOT_STORE", "./dev/data/offshoot-store"),
		S3Endpoint:      envOr("S3_ENDPOINT", "http://127.0.0.1:9000"),
		S3Region:        envOr("AWS_REGION", "us-east-1"),
		S3AccessKey:     envOr("AWS_ACCESS_KEY_ID", envOr("RUSTFS_ACCESS_KEY", "rustfsadmin")),
		S3SecretKey:     envOr("AWS_SECRET_ACCESS_KEY", envOr("RUSTFS_SECRET_KEY", "rustfsadmin")),
		ArtifactsBucket: envOr("CELLP_ARTIFACTS_BUCKET", "cellp-artifacts"),
		CelldBucket:     envOr("CELLD_BUCKET", "s3://cellp-celld/demo-app"),
		ArtifactsDir:    envOr("ARTIFACTS_DIR", "./dev/data/artifacts"),
	}
	cfg.Ingress = loadIngressConfig()
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

var workerEnvKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const (
	maxWorkerEnvKeys  = 64
	maxWorkerEnvValue = 8192
)

// StripPlatformEnv removes keys the platform owns (DESIGN §12).
func StripPlatformEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if isPlatformEnvKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}

func isPlatformEnvKey(k string) bool {
	upper := strings.ToUpper(strings.TrimSpace(k))
	if upper == "" {
		return true
	}
	if strings.HasPrefix(upper, "CELLP_") || strings.HasPrefix(upper, "CELLD_") {
		return true
	}
	switch upper {
	case "PROJECT_ID", "VERSION_ID", "CELLD_REGISTRY":
		return true
	default:
		return false
	}
}

// NormalizeWorkerEnv strips platform keys and rejects invalid names/values.
func NormalizeWorkerEnv(env map[string]string) (map[string]string, error) {
	stripped := StripPlatformEnv(env)
	if len(stripped) > maxWorkerEnvKeys {
		return nil, fmt.Errorf("too many env keys (max %d)", maxWorkerEnvKeys)
	}
	out := make(map[string]string, len(stripped))
	for k, v := range stripped {
		if !workerEnvKeyRe.MatchString(k) {
			return nil, fmt.Errorf("invalid env key %q", k)
		}
		if strings.ContainsAny(v, "\n\x00") {
			return nil, fmt.Errorf("invalid env value for %q", k)
		}
		if len(v) > maxWorkerEnvValue {
			return nil, fmt.Errorf("env value for %q is too long", k)
		}
		out[k] = v
	}
	return out, nil
}

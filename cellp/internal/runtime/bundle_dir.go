package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NativeHTTPProfileV1 is the explicit celld execution profile for Native Component HTTP.
const NativeHTTPProfileV1 = "native-http-v1"

type wranglerCellp struct {
	Execution string `json:"execution"`
	Component string `json:"component"`
}

type wranglerExecution struct {
	Cellp *wranglerCellp `json:"cellp"`
}

// ResolveVersionBundleDir picks the directory passed to celld deploy for a version.
// When the fetched artifact contains wrangler config, that tree is used as-is so
// native component bytes and wrangler metadata reach celld unchanged.
func ResolveVersionBundleDir(artifactsDir, projectID, versionID string) (string, error) {
	destDir := filepath.Join(artifactsDir, projectID, versionID)
	bundleDir := filepath.Join("dev", "examples", "counter")
	if hasWranglerConfig(destDir) {
		bundleDir = destDir
	} else if alt := filepath.Join(artifactsDir, "..", "examples", "counter"); alt != bundleDir {
		if hasWranglerConfig(alt) {
			bundleDir = alt
		}
	}
	abs, err := filepath.Abs(bundleDir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func hasWranglerConfig(dir string) bool {
	for _, name := range []string{"wrangler.jsonc", "wrangler.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// WranglerExecutionProfile returns the explicit cellp.execution value when present.
// An empty string means the bundle follows the historical implicit JS/V8 profile.
func WranglerExecutionProfile(bundleDir string) (string, error) {
	_, raw, err := readWranglerConfigFile(bundleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var cfg wranglerExecution
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse wrangler: %w", err)
	}
	if cfg.Cellp == nil {
		return "", nil
	}
	return strings.TrimSpace(cfg.Cellp.Execution), nil
}

// NativeComponentPath returns the project-relative component path declared in wrangler.
func NativeComponentPath(bundleDir string) (string, error) {
	_, raw, err := readWranglerConfigFile(bundleDir)
	if err != nil {
		return "", err
	}
	var cfg wranglerExecution
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse wrangler: %w", err)
	}
	if cfg.Cellp == nil || strings.TrimSpace(cfg.Cellp.Component) == "" {
		return "", fmt.Errorf("native-http-v1 requires cellp.component")
	}
	return cfg.Cellp.Component, nil
}

// ValidateDeployBundle fail-closes Native bundles before celld deploy when the
// declared component artifact is missing from the bundle directory.
func ValidateDeployBundle(bundleDir string) error {
	profile, err := WranglerExecutionProfile(bundleDir)
	if err != nil {
		return err
	}
	if profile == "" || profile != NativeHTTPProfileV1 {
		return nil
	}
	component, err := NativeComponentPath(bundleDir)
	if err != nil {
		return err
	}
	if strings.Contains(component, "..") {
		return fmt.Errorf("cellp.component must stay inside the bundle directory")
	}
	path := filepath.Join(bundleDir, filepath.FromSlash(component))
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return fmt.Errorf("native component artifact missing: %s", component)
	}
	return nil
}

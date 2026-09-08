package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// DeployPreflight selects how celld is validated before deploy.
type DeployPreflight int

const (
	// DeployPreflightFullFleet runs celld diagnose with full fleet enumeration (initial deploy).
	DeployPreflightFullFleet DeployPreflight = iota
	// DeployPreflightConfiguredUpstream verifies only the version's configured upstream celld
	// (cron redeploy after prod change). Unrelated fleet peers must not block deploy.
	DeployPreflightConfiguredUpstream
)

// DeployForCronReconcile redeploys a ready version's bundle without a full-fleet diagnose.
// The configured route upstream must be healthy (fail-closed).
func (m *Manager) DeployForCronReconcile(ctx context.Context, project, version, exampleDir string, includeCrons bool, upstreamHost string, upstreamPort int) error {
	return m.deployWithPreflight(ctx, project, version, exampleDir, includeCrons, DeployPreflightConfiguredUpstream, upstreamHost, upstreamPort)
}

func (m *Manager) verifyConfiguredUpstream(ctx context.Context, host string, port int) error {
	if strings.TrimSpace(host) == "" || port <= 0 {
		return fmt.Errorf("configured celld upstream missing")
	}
	if !m.Health(ctx, host, port) {
		return fmt.Errorf("configured celld upstream %s:%d unreachable or unhealthy", host, port)
	}
	return nil
}

func (m *Manager) runDeployPreflight(ctx context.Context, project, version string, mode DeployPreflight, upstreamHost string, upstreamPort int) error {
	if os.Getenv("CELLP_SKIP_CELLD_DIAGNOSE") == "1" {
		return nil
	}
	if !CelldInstalled() {
		return nil
	}
	switch mode {
	case DeployPreflightFullFleet:
		return m.Diagnose(ctx, project, version)
	case DeployPreflightConfiguredUpstream:
		return m.verifyConfiguredUpstream(ctx, upstreamHost, upstreamPort)
	default:
		return fmt.Errorf("unknown deploy preflight mode %d", mode)
	}
}

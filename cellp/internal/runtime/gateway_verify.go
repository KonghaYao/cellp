package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VerifyGatewayRoute is deprecated (AD-12 path routing removed). Use VerifyGatewayRouteHost.
func VerifyGatewayRoute(ctx context.Context, gatewayURL, project, version string) error {
	return fmt.Errorf("path gateway verify removed (AD-12): use VerifyGatewayRouteHost with preview Host for %s/%s", project, version)
}

// VerifyGatewayRouteHost checks Host-based gateway routing to celld health.
func VerifyGatewayRouteHost(ctx context.Context, gatewayURL, host string) error {
	base := strings.TrimRight(gatewayURL, "/")
	url := base + "/.well-known/celld/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Host = host
	req.Header.Set("Host", host)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway route probe: status %d for Host %s", resp.StatusCode, host)
	}
	return nil
}

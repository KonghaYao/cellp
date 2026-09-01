package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VerifyGatewayRoute checks path-based gateway routing (legacy AD-2).
func VerifyGatewayRoute(ctx context.Context, gatewayURL, project, version string) error {
	base := strings.TrimRight(gatewayURL, "/")
	url := fmt.Sprintf("%s/%s/%s/.well-known/celld/health", base, project, version)
	return probeGatewayHealth(ctx, url, "")
}

// VerifyGatewayRouteHost checks Host-based ingress (AD-12).
func VerifyGatewayRouteHost(ctx context.Context, gatewayURL, host string) error {
	base := strings.TrimRight(gatewayURL, "/")
	url := base + "/.well-known/celld/health"
	return probeGatewayHealth(ctx, url, host)
}

func probeGatewayHealth(ctx context.Context, url, host string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if host != "" {
		req.Host = host
		req.Header.Set("Host", host)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway route probe: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway route probe: status %d for %s", resp.StatusCode, url)
	}
	return nil
}

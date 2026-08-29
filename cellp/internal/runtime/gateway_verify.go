package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VerifyGatewayRoute checks that the gateway forwards to the version's celld health endpoint.
func VerifyGatewayRoute(ctx context.Context, gatewayURL, project, version string) error {
	base := strings.TrimRight(gatewayURL, "/")
	url := fmt.Sprintf("%s/%s/%s/.well-known/celld/health", base, project, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway route probe: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway route probe: status %d for %s", resp.StatusCode, url)
	}
	return nil
}

package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VerifyGatewayRoutePreviewURL probes dedicated listen port routing (Tier B) with synthetic upstream Host.
func VerifyGatewayRoutePreviewURL(ctx context.Context, previewURL, syntheticHost, verifyBase string) error {
	u, err := url.Parse(strings.TrimSpace(previewURL))
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid preview url %q", previewURL)
	}
	if vb, err := url.Parse(strings.TrimRight(strings.TrimSpace(verifyBase), "/")); err == nil && vb.Host != "" {
		u.Scheme = vb.Scheme
		u.Host = vb.Host
	}
	probe := u.Scheme + "://" + u.Host + "/.well-known/celld/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe, nil)
	if err != nil {
		return err
	}
	req.Host = syntheticHost
	req.Header.Set("Host", syntheticHost)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway route probe: status %d for %s Host %s", resp.StatusCode, probe, syntheticHost)
	}
	return nil
}

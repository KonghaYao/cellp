package gateway

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/cellp/cellp/internal/registry"
)

type localListenPortKey struct{}

// WithLocalListenPort attaches the gateway listener port for dedicated_port resolution (tests).
func WithLocalListenPort(ctx context.Context, port int) context.Context {
	return context.WithValue(ctx, localListenPortKey{}, port)
}

func (g *Gateway) resolveIngressBinding(ctx context.Context, r *http.Request) (*registry.IngressBinding, error) {
	localPort := listenerLocalPort(r)
	tierB := strings.ToLower(strings.TrimSpace(g.cfg.IngressTierB))

	if tierB == "dedicated_port" && localPort != 0 && localPort != g.cfg.GatewayPort {
		b, err := g.store.LookupIngressByListenPort(ctx, localPort, g.cfg.GatewayID)
		if err != nil {
			return nil, err
		}
		if b != nil && b.Active {
			return b, nil
		}
	}

	host := g.effectiveHost(r)
	if host == "" {
		return nil, nil
	}
	return g.lookupIngressByHost(ctx, host)
}

func listenerLocalPort(r *http.Request) int {
	if r == nil {
		return 0
	}
	if v := r.Context().Value(localListenPortKey{}); v != nil {
		if p, ok := v.(int); ok {
			return p
		}
	}
	return 0
}

func (g *Gateway) lookupIngressByHost(ctx context.Context, host string) (*registry.IngressBinding, error) {
	host = registry.NormalizeIngressHost(host)
	if host == "" {
		return nil, nil
	}
	if g.cache != nil {
		if b, hit, found := g.cache.GetIngressHost(host); hit {
			if !found {
				return nil, nil
			}
			copyB := *b
			return &copyB, nil
		}
	}
	b, err := g.store.LookupIngressByHost(ctx, host)
	if err != nil {
		return nil, err
	}
	found := b != nil
	if g.cache != nil {
		g.cache.SetIngressHost(host, b, found)
	}
	return b, nil
}

func (g *Gateway) versionForBinding(ctx context.Context, b *registry.IngressBinding) (projectID, versionID string, ok bool) {
	if b == nil {
		return "", "", false
	}
	projectID = b.ProjectID
	if b.Role == registry.IngressRoleProd {
		vid, found := g.lookupProdVersion(ctx, projectID)
		return projectID, vid, found && vid != ""
	}
	if b.VersionID != nil && *b.VersionID != "" {
		return projectID, *b.VersionID, true
	}
	return "", "", false
}

func parseLocalPort(addr string) int {
	if addr == "" {
		return 0
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return p
}

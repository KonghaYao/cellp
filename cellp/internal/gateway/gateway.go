package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/metrics"
	"github.com/cellp/cellp/internal/registry"
	"github.com/go-chi/chi/v5"
)

// Gateway is the cellpd built-in reverse proxy (DESIGN §2.3, AD-12 Host ingress).
type Gateway struct {
	store       registry.Store
	cache       *RouteCache
	router      chi.Router
	cfg         GatewayConfig
	lastTouchMu sync.Mutex
	lastTouchAt map[string]time.Time
}

// New creates a gateway with config from the environment.
func New(store registry.Store) *Gateway {
	return NewWithConfig(store, ConfigFromEnv())
}

// NewWithConfig creates a gateway with explicit AD-12 settings (tests).
func NewWithConfig(store registry.Store, cfg GatewayConfig) *Gateway {
	g := &Gateway{
		store:       store,
		cache:       NewRouteCache(),
		cfg:         cfg,
		lastTouchAt: make(map[string]time.Time),
	}
	g.router = chi.NewRouter()
	g.routes()
	return g
}

// InvalidateRoute clears cached route data for a project/version pair.
func (g *Gateway) InvalidateRoute(projectID, versionID string) {
	if g.cache != nil {
		g.cache.InvalidateRoute(projectID, versionID)
	}
}

// InvalidateProd clears cached prod version data for a project.
func (g *Gateway) InvalidateProd(projectID string) {
	if g.cache != nil {
		g.cache.InvalidateProd(projectID)
	}
}

// InvalidateIngressHost clears cached ingress binding for a host.
func (g *Gateway) InvalidateIngressHost(host string) {
	if g.cache != nil {
		g.cache.InvalidateIngressHost(registry.NormalizeIngressHost(host))
	}
}

// LookupRoute resolves a route via cache then registry. Exported for tests.
func (g *Gateway) LookupRoute(ctx context.Context, projectID, versionID string) (*registry.Route, bool) {
	return g.lookupRoute(ctx, projectID, versionID)
}

// LookupProdVersion resolves prod version via cache then registry. Exported for tests.
func (g *Gateway) LookupProdVersion(ctx context.Context, projectID string) (string, bool) {
	return g.lookupProdVersion(ctx, projectID)
}

// RouteCacheForTest exposes the route cache for test configuration.
func (g *Gateway) RouteCacheForTest() *RouteCache {
	return g.cache
}

// Config returns the gateway configuration (listeners, tests).
func (g *Gateway) Config() GatewayConfig {
	return g.cfg
}

func (g *Gateway) Handler() http.Handler {
	return corsMiddleware(MetricsMiddleware(g.router))
}

func (g *Gateway) routes() {
	g.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway ok"))
	})
	g.router.Get("/health/deep", g.handleHealthDeep)
	g.router.Handle("/*", http.HandlerFunc(g.handleIngress))
}

func (g *Gateway) handleIngress(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/health/") {
		http.NotFound(w, r)
		return
	}

	binding, err := g.resolveIngressBinding(r.Context(), r)
	if err != nil {
		http.Error(w, "ingress lookup failed", http.StatusInternalServerError)
		return
	}
	if binding == nil || !binding.Active {
		http.Error(w, "ingress_unknown", http.StatusNotFound)
		return
	}

	projectID, versionID, ok := g.versionForBinding(r.Context(), binding)
	if !ok {
		http.Error(w, "ingress_unknown", http.StatusNotFound)
		return
	}

	route, ok := g.lookupRoute(r.Context(), projectID, versionID)
	if !ok || route == nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	if !route.Active {
		if g.versionInactiveBody(r.Context(), projectID, versionID) == "version_archived" {
			http.Error(w, "version_archived", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "route draining", http.StatusServiceUnavailable)
		return
	}

	g.proxyIngress(w, r, route, binding, projectID, versionID)
}

func (g *Gateway) versionInactiveBody(ctx context.Context, projectID, versionID string) string {
	v, err := g.store.GetVersion(ctx, projectID, versionID)
	if err != nil || v == nil {
		return "route draining"
	}
	if v.Status == registry.StatusArchived {
		return "version_archived"
	}
	return "route draining"
}

func (g *Gateway) lookupRoute(ctx context.Context, projectID, versionID string) (*registry.Route, bool) {
	if g.cache != nil {
		if route, hit, found := g.cache.GetRoute(projectID, versionID); hit {
			return route, found
		}
	}

	route, err := g.store.GetRoute(ctx, projectID, versionID)
	found := err == nil && route != nil
	if g.cache != nil {
		g.cache.SetRoute(projectID, versionID, route, found)
	}
	return route, found
}

func (g *Gateway) lookupProdVersion(ctx context.Context, projectID string) (string, bool) {
	if g.cache != nil {
		if versionID, hit, found := g.cache.GetProd(projectID); hit {
			if !found || versionID == nil {
				return "", false
			}
			return *versionID, true
		}
	}

	proj, err := g.store.GetProject(ctx, projectID)
	found := err == nil && proj != nil && proj.ProdVersionID != nil
	var versionID *string
	if found {
		versionID = proj.ProdVersionID
	}
	if g.cache != nil {
		g.cache.SetProd(projectID, versionID, found)
	}
	if !found || versionID == nil {
		return "", false
	}
	return *versionID, true
}

func (g *Gateway) proxyIngress(w http.ResponseWriter, r *http.Request, route *registry.Route, binding *registry.IngressBinding, projectID, versionID string) {
	target, err := url.Parse(fmt.Sprintf("http://%s:%d", route.UpstreamHost, route.UpstreamPort))
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}
	publicProto := g.publicSchemeForRequest(r, binding.Role)
	clientAuth := clientAuthorityForIngress(r, publicProto, g.cfg.GatewayPort)

	proxy := httputil.NewSingleHostReverseProxy(target)
	// Immediate flush: WebSocket 101 and SSE must not wait for the default
	// 50ms / full-response buffer. http.Server here has no WriteTimeout.
	proxy.FlushInterval = -1
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		applyUpstreamHeaders(req, binding, clientAuth, publicProto)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		metrics.RecordGatewayUpstream(resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			g.touchLastAccessThrottled(projectID, versionID)
		}
		ct := resp.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "text/event-stream") {
			resp.Header.Set("X-Accel-Buffering", "no")
			if resp.Header.Get("Cache-Control") == "" {
				resp.Header.Set("Cache-Control", "no-cache")
			}
		}
		return nil
	}
	upgrade := isUpgradeRequest(r)
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, e error) {
		metrics.RecordGatewayUpstream(http.StatusBadGateway)
		if upgrade {
			log.Printf("gateway websocket proxy error class=%s method=%s path=%s host=%s err=%v",
				classifyProxyError(e), req.Method, req.URL.Path, req.Host, e)
		}
		http.Error(rw, "bad gateway", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func (g *Gateway) touchLastAccessThrottled(projectID, versionID string) {
	key := projectID + "/" + versionID
	now := time.Now().UTC()
	g.lastTouchMu.Lock()
	if last, ok := g.lastTouchAt[key]; ok && now.Sub(last) < time.Minute {
		g.lastTouchMu.Unlock()
		return
	}
	g.lastTouchAt[key] = now
	g.lastTouchMu.Unlock()
	go func() {
		_ = g.store.TouchLastAccess(context.Background(), projectID, versionID)
	}()
}

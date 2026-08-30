package gateway

import (
	"context"
	"fmt"
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

// Gateway is the cellpd built-in reverse proxy (DESIGN §2.3).
type Gateway struct {
	store       registry.Store
	cache       *RouteCache
	router      chi.Router
	lastTouchMu sync.Mutex
	lastTouchAt map[string]time.Time
}

// New creates a gateway server with an in-memory route cache.
func New(store registry.Store) *Gateway {
	g := &Gateway{
		store:       store,
		cache:       NewRouteCache(),
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

func (g *Gateway) Handler() http.Handler {
	return corsMiddleware(MetricsMiddleware(g.router))
}

func (g *Gateway) routes() {
	g.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway ok"))
	})
	g.router.Get("/health/deep", g.handleHealthDeep)

	// Version-specific route: /{project}/{version}/*
	g.router.Handle("/{project}/{version}/*", http.HandlerFunc(g.handleVersionRoute))

	// Prod route: /{project}/* (AD-2)
	g.router.Handle("/{project}/*", http.HandlerFunc(g.handleProdRoute))
}

func (g *Gateway) handleVersionRoute(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "project")
	versionID := chi.URLParam(r, "version")
	if projectID == "health" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway ok"))
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

	prefix := fmt.Sprintf("/%s/%s", projectID, versionID)
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	g.proxy(w, r, route.UpstreamHost, route.UpstreamPort, rest, projectID, versionID)
}

func (g *Gateway) handleProdRoute(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "project")
	if projectID == "health" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway ok"))
		return
	}

	versionID, ok := g.lookupProdVersion(r.Context(), projectID)
	if !ok || versionID == "" {
		http.Error(w, "prod not configured", http.StatusNotFound)
		return
	}

	route, ok := g.lookupRoute(r.Context(), projectID, versionID)
	if !ok || route == nil {
		http.Error(w, "prod route not found", http.StatusNotFound)
		return
	}
	if !route.Active {
		http.Error(w, "route draining", http.StatusServiceUnavailable)
		return
	}

	prefix := fmt.Sprintf("/%s", projectID)
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == "" {
		rest = "/"
	}
	g.proxy(w, r, route.UpstreamHost, route.UpstreamPort, rest, projectID, versionID)
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

func (g *Gateway) proxy(w http.ResponseWriter, r *http.Request, host string, port int, path, projectID, versionID string) {
	target, err := url.Parse(fmt.Sprintf("http://%s:%d", host, port))
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	r.URL.Path = path
	r.Host = target.Host
	proxy.ModifyResponse = func(resp *http.Response) error {
		metrics.RecordGatewayUpstream(resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			g.touchLastAccessThrottled(projectID, versionID)
		}
		return nil
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, e error) {
		metrics.RecordGatewayUpstream(http.StatusBadGateway)
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

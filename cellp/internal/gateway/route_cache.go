package gateway

import (
	"container/list"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

const (
	defaultRouteCacheTTL         = 60 * time.Second
	defaultRouteCacheMaxRoutes   = 10_000
	defaultRouteCacheMaxProd     = 5_000
	defaultRouteCacheMaxIngress  = 5_000
)

type cachedRoute struct {
	route *registry.Route
	found bool
}

type cachedProd struct {
	versionID *string
	found     bool
}

type cachedIngress struct {
	binding *registry.IngressBinding
	found   bool
}

type routeCacheEntry struct {
	key       string
	expiresAt time.Time
	value     cachedRoute
}

type prodCacheEntry struct {
	key       string
	expiresAt time.Time
	value     cachedProd
}

type ingressCacheEntry struct {
	key       string
	expiresAt time.Time
	value     cachedIngress
}

// RouteCache is an in-process LRU/TTL cache for gateway route lookups.
type RouteCache struct {
	mu sync.Mutex

	ttl          time.Duration
	maxRoutes    int
	maxProd      int
	maxIngress   int

	routes map[string]*list.Element
	routeL *list.List

	prod map[string]*list.Element
	prodL *list.List

	ingress map[string]*list.Element
	ingressL *list.List
}

// NewRouteCache returns a RouteCache with default TTL and capacity limits.
func NewRouteCache() *RouteCache {
	return &RouteCache{
		ttl:        defaultRouteCacheTTL,
		maxRoutes:  defaultRouteCacheMaxRoutes,
		maxProd:    defaultRouteCacheMaxProd,
		maxIngress: defaultRouteCacheMaxIngress,
		routes:     make(map[string]*list.Element),
		routeL:     list.New(),
		prod:       make(map[string]*list.Element),
		prodL:      list.New(),
		ingress:    make(map[string]*list.Element),
		ingressL:   list.New(),
	}
}

func routeKey(projectID, versionID string) string {
	return projectID + ":" + versionID
}

func (c *RouteCache) touchRoute(el *list.Element) {
	c.routeL.MoveToFront(el)
}

func (c *RouteCache) touchProd(el *list.Element) {
	c.prodL.MoveToFront(el)
}

func (c *RouteCache) touchIngress(el *list.Element) {
	c.ingressL.MoveToFront(el)
}

func (c *RouteCache) evictRoute(el *list.Element) {
	ent := el.Value.(*routeCacheEntry)
	delete(c.routes, ent.key)
	c.routeL.Remove(el)
}

func (c *RouteCache) evictProd(el *list.Element) {
	ent := el.Value.(*prodCacheEntry)
	delete(c.prod, ent.key)
	c.prodL.Remove(el)
}

func (c *RouteCache) evictIngress(el *list.Element) {
	ent := el.Value.(*ingressCacheEntry)
	delete(c.ingress, ent.key)
	c.ingressL.Remove(el)
}

func (c *RouteCache) trimRoutes() {
	for c.routeL.Len() > c.maxRoutes {
		if back := c.routeL.Back(); back != nil {
			c.evictRoute(back)
		}
	}
}

func (c *RouteCache) trimProd() {
	for c.prodL.Len() > c.maxProd {
		if back := c.prodL.Back(); back != nil {
			c.evictProd(back)
		}
	}
}

func (c *RouteCache) trimIngress() {
	for c.ingressL.Len() > c.maxIngress {
		if back := c.ingressL.Back(); back != nil {
			c.evictIngress(back)
		}
	}
}

// GetRoute returns a cached route lookup if present and not expired.
func (c *RouteCache) GetRoute(projectID, versionID string) (*registry.Route, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := routeKey(projectID, versionID)
	el, ok := c.routes[key]
	if !ok {
		return nil, false, false
	}
	ent := el.Value.(*routeCacheEntry)
	if time.Now().After(ent.expiresAt) {
		c.evictRoute(el)
		return nil, false, false
	}
	c.touchRoute(el)
	if !ent.value.found {
		return nil, true, false
	}
	r := *ent.value.route
	return &r, true, true
}

// SetRoute stores a route lookup result (including negative cache).
func (c *RouteCache) SetRoute(projectID, versionID string, route *registry.Route, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := routeKey(projectID, versionID)
	value := cachedRoute{found: found}
	if found && route != nil {
		copyRoute := *route
		value.route = &copyRoute
	}

	if el, ok := c.routes[key]; ok {
		ent := el.Value.(*routeCacheEntry)
		ent.value = value
		ent.expiresAt = time.Now().Add(c.ttl)
		c.touchRoute(el)
		return
	}

	ent := &routeCacheEntry{key: key, expiresAt: time.Now().Add(c.ttl), value: value}
	el := c.routeL.PushFront(ent)
	c.routes[key] = el
	c.trimRoutes()
}

// GetProd returns a cached prod version pointer lookup if present.
func (c *RouteCache) GetProd(projectID string) (*string, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.prod[projectID]
	if !ok {
		return nil, false, false
	}
	ent := el.Value.(*prodCacheEntry)
	if time.Now().After(ent.expiresAt) {
		c.evictProd(el)
		return nil, false, false
	}
	c.touchProd(el)
	if !ent.value.found || ent.value.versionID == nil {
		return nil, true, false
	}
	v := *ent.value.versionID
	return &v, true, true
}

// SetProd stores a prod version lookup result (including negative cache).
func (c *RouteCache) SetProd(projectID string, versionID *string, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value := cachedProd{found: found}
	if found && versionID != nil {
		copyID := *versionID
		value.versionID = &copyID
	}

	if el, ok := c.prod[projectID]; ok {
		ent := el.Value.(*prodCacheEntry)
		ent.value = value
		ent.expiresAt = time.Now().Add(c.ttl)
		c.touchProd(el)
		return
	}

	ent := &prodCacheEntry{key: projectID, expiresAt: time.Now().Add(c.ttl), value: value}
	el := c.prodL.PushFront(ent)
	c.prod[projectID] = el
	c.trimProd()
}

// GetIngressHost returns a cached ingress binding by normalized host.
func (c *RouteCache) GetIngressHost(host string) (*registry.IngressBinding, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.ingress[host]
	if !ok {
		return nil, false, false
	}
	ent := el.Value.(*ingressCacheEntry)
	if time.Now().After(ent.expiresAt) {
		c.evictIngress(el)
		return nil, false, false
	}
	c.touchIngress(el)
	if !ent.value.found {
		return nil, true, false
	}
	b := *ent.value.binding
	return &b, true, true
}

// SetIngressHost stores an ingress host lookup (including negative cache).
func (c *RouteCache) SetIngressHost(host string, binding *registry.IngressBinding, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value := cachedIngress{found: found}
	if found && binding != nil {
		copyB := *binding
		value.binding = &copyB
	}

	if el, ok := c.ingress[host]; ok {
		ent := el.Value.(*ingressCacheEntry)
		ent.value = value
		ent.expiresAt = time.Now().Add(c.ttl)
		c.touchIngress(el)
		return
	}

	ent := &ingressCacheEntry{key: host, expiresAt: time.Now().Add(c.ttl), value: value}
	el := c.ingressL.PushFront(ent)
	c.ingress[host] = el
	c.trimIngress()
}

// InvalidateRoute drops cached data for a specific project/version route.
func (c *RouteCache) InvalidateRoute(projectID, versionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := routeKey(projectID, versionID)
	if el, ok := c.routes[key]; ok {
		c.evictRoute(el)
	}
}

// InvalidateProd drops cached prod version for a project.
func (c *RouteCache) InvalidateProd(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.prod[projectID]; ok {
		c.evictProd(el)
	}
}

// InvalidateIngressHost drops cached ingress binding for a host.
func (c *RouteCache) InvalidateIngressHost(host string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.ingress[host]; ok {
		c.evictIngress(el)
	}
}

// InvalidateProject drops all cached routes and prod pointer for a project.
func (c *RouteCache) InvalidateProject(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.prod[projectID]; ok {
		c.evictProd(el)
	}

	prefix := projectID + ":"
	for key, el := range c.routes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			c.evictRoute(el)
		}
	}
}

// SetTTLForTest overrides cache TTL (tests only).
func (c *RouteCache) SetTTLForTest(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}

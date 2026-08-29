package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/health"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func (s *Server) handleHealthDeep(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	pending, err := s.store.CountPendingJobs(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	max := QueueMax()

	checks := map[string]interface{}{}
	overall := "ok"
	code := http.StatusOK

	// registry (SQLite)
	regStart := time.Now()
	if err := s.store.Ping(ctx); err != nil {
		checks["registry"] = health.CheckResult{Name: "registry", Status: "down", Detail: err.Error()}
		overall = "degraded"
		code = http.StatusServiceUnavailable
	} else {
		checks["registry"] = health.CheckResult{Name: "registry", Status: "ok", LatencyMs: time.Since(regStart).Milliseconds()}
	}

	// RustFS (skip when endpoint not configured, e.g. unit tests)
	if strings.TrimSpace(s.cfg.S3Endpoint) != "" {
		rustfs := health.ProbeRustFS(ctx, s.cfg.S3Endpoint)
		checks["rustfs"] = rustfs
		if rustfs.Status == "down" {
			overall = "degraded"
			code = http.StatusServiceUnavailable
		}
	} else {
		checks["rustfs"] = health.CheckResult{Name: "rustfs", Status: "skipped", Detail: "no endpoint"}
	}

	// celld base install
	if runtime.CelldInstalled() {
		checks["celld"] = health.CheckResult{Name: "celld", Status: "ok", Detail: "installed"}
	} else {
		checks["celld"] = health.CheckResult{Name: "celld", Status: "skipped", Detail: "not on PATH"}
	}

	// active route fleet rollup
	routes, err := s.store.ListAllActiveRoutes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	runtimeHealth := s.runtime.RuntimeHealth(ctx, routes)
	healthy, unhealthy := 0, 0
	for _, rh := range runtimeHealth {
		if rh.Healthy {
			healthy++
		} else {
			unhealthy++
		}
	}
	checks["runtimes"] = map[string]interface{}{
		"active_routes": len(routes),
		"healthy":       healthy,
		"unhealthy":     unhealthy,
	}
	if unhealthy > 0 && runtime.CelldInstalled() {
		overall = "degraded"
		code = http.StatusServiceUnavailable
	}

	checks["queue"] = map[string]interface{}{
		"pending_jobs": pending,
		"queue_max":    max,
	}
	if pending >= max {
		overall = "overloaded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]interface{}{
		"status":   overall,
		"registry": s.cfg.RegistryDB,
		"checks":   checks,
	})
}

func (s *Server) handleRuntimeRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	routes, err := s.store.ListAllActiveRoutes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type routeRow struct {
		ProjectID     string `json:"project_id"`
		VersionID     string `json:"version_id"`
		Active        bool   `json:"active"`
		Upstream      string `json:"upstream"`
		VersionStatus string `json:"version_status,omitempty"`
		CelldHealth   string `json:"celld_health"`
	}

	rows := make([]routeRow, len(routes))
	var wg sync.WaitGroup
	for i, route := range routes {
		wg.Add(1)
		go func(i int, route registry.Route) {
			defer wg.Done()
			status := ""
			if v, err := s.store.GetVersion(ctx, route.ProjectID, route.VersionID); err == nil && v != nil {
				status = v.Status
			}
			healthy := s.runtime.Health(ctx, route.UpstreamHost, route.UpstreamPort)
			celldHealth := "ok"
			if !healthy {
				celldHealth = "down"
			}
			rows[i] = routeRow{
				ProjectID:     route.ProjectID,
				VersionID:     route.VersionID,
				Active:        route.Active,
				Upstream:      formatUpstream(route.UpstreamHost, route.UpstreamPort),
				VersionStatus: status,
				CelldHealth:   celldHealth,
			}
		}(i, route)
	}
	wg.Wait()

	healthy, unhealthy := 0, 0
	for _, row := range rows {
		if row.CelldHealth == "ok" {
			healthy++
		} else {
			unhealthy++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"summary": map[string]int{
			"active_routes": len(rows),
			"healthy":       healthy,
			"unhealthy":     unhealthy,
		},
		"routes": rows,
	})
}

func formatUpstream(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

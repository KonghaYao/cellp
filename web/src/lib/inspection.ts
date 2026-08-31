import type { RuntimeRouteRow, Version } from "@/lib/cellp-api";
import { isInProgressStatus } from "@/lib/status";

export interface VersionFleetSummary {
  total: number;
  ready: number;
  inProgress: number;
  failed: number;
  archived: number;
  other: number;
}

export function summarizeVersionFleet(versions: Version[]): VersionFleetSummary {
  const out: VersionFleetSummary = {
    total: versions.length,
    ready: 0,
    inProgress: 0,
    failed: 0,
    archived: 0,
    other: 0,
  };
  for (const v of versions) {
    if (v.status === "ready") out.ready += 1;
    else if (v.status === "failed") out.failed += 1;
    else if (v.status === "archived") out.archived += 1;
    else if (isInProgressStatus(v.status)) out.inProgress += 1;
    else if (v.status === "destroyed" || v.status === "draining") continue;
    else out.other += 1;
  }
  return out;
}

export function routesForProject(
  routes: RuntimeRouteRow[],
  projectId: string,
): RuntimeRouteRow[] {
  return routes.filter((r) => r.project_id === projectId);
}

export function routeForVersion(
  routes: RuntimeRouteRow[],
  projectId: string,
  versionId: string,
): RuntimeRouteRow | undefined {
  return routes.find(
    (r) => r.project_id === projectId && r.version_id === versionId,
  );
}

export function isCelldRouteHealthy(celldHealth: string): boolean {
  return celldHealth === "ok" || celldHealth === "healthy";
}

export function countUnhealthyRoutes(rows: RuntimeRouteRow[]): number {
  return rows.filter((r) => !isCelldRouteHealthy(r.celld_health)).length;
}

export interface PlatformMetricsView {
  pendingJobs: number | null;
  routesActive: number | null;
  celldHealthy: number | null;
  celldUnhealthy: number | null;
  gatewayRequests: number | null;
  gateway5xx: number | null;
  gatewayUpstream5xx: number | null;
}

export function pickPlatformMetrics(
  gauges: Record<string, number>,
): PlatformMetricsView {
  const n = (key: string) =>
    typeof gauges[key] === "number" ? gauges[key] : null;
  return {
    pendingJobs: n("cellp_pending_jobs"),
    routesActive: n("cellp_routes_active"),
    celldHealthy: n("cellp_celld_healthy"),
    celldUnhealthy: n("cellp_celld_unhealthy"),
    gatewayRequests: n("cellp_gateway_requests_total"),
    gateway5xx: n("cellp_gateway_errors_5xx"),
    gatewayUpstream5xx: n("cellp_gateway_upstream_5xx"),
  };
}

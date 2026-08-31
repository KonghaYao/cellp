import { describe, it, expect } from "vitest";
import {
  countUnhealthyRoutes,
  pickPlatformMetrics,
  routesForProject,
  routeForVersion,
  summarizeVersionFleet,
} from "@/lib/inspection";
import type { RuntimeRouteRow, Version } from "@/lib/cellp-api";

describe("inspection helpers", () => {
  it("summarizeVersionFleet counts statuses", () => {
    const versions = [
      { status: "ready" },
      { status: "ready" },
      { status: "deploying" },
      { status: "failed" },
    ] as Version[];
    expect(summarizeVersionFleet(versions)).toMatchObject({
      total: 4,
      ready: 2,
      inProgress: 1,
      failed: 1,
    });
  });

  it("routesForProject filters rows", () => {
    const rows: RuntimeRouteRow[] = [
      {
        project_id: "a",
        version_id: "v1",
        active: true,
        upstream: "http://x",
        celld_health: "ok",
      },
      {
        project_id: "b",
        version_id: "v2",
        active: true,
        upstream: "http://y",
        celld_health: "ok",
      },
    ];
    expect(routesForProject(rows, "a")).toHaveLength(1);
    expect(routeForVersion(rows, "a", "v1")?.upstream).toBe("http://x");
  });

  it("pickPlatformMetrics reads gauge keys", () => {
    expect(
      pickPlatformMetrics({
        cellp_pending_jobs: 2,
        cellp_gateway_errors_5xx: 1,
      }),
    ).toEqual({
      pendingJobs: 2,
      routesActive: null,
      celldHealthy: null,
      celldUnhealthy: null,
      gatewayRequests: null,
      gateway5xx: 1,
      gatewayUpstream5xx: null,
    });
  });

  it("countUnhealthyRoutes", () => {
    const rows: RuntimeRouteRow[] = [
      {
        project_id: "a",
        version_id: "v1",
        active: true,
        upstream: "",
        celld_health: "ok",
      },
      {
        project_id: "a",
        version_id: "v2",
        active: true,
        upstream: "",
        celld_health: "down",
      },
    ];
    expect(countUnhealthyRoutes(rows)).toBe(1);
  });
});

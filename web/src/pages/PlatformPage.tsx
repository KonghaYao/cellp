import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Activity, RefreshCw } from "lucide-react";
import {
  CellpApiError,
  fetchMetricsGauges,
  getGatewayHealthDeep,
  getHealthDeep,
  getRuntimeRoutes,
  type DeepHealth,
  type RuntimeRoutesResponse,
} from "@/lib/cellp-api";
import { Breadcrumbs } from "@/components/breadcrumbs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { pickPlatformMetrics } from "@/lib/inspection";
import { versionHref } from "@/lib/routes";
import { cn } from "@/lib/utils";

const REFRESH_MS = 15_000;

function statusVariant(status: string): "default" | "secondary" | "destructive" {
  if (status === "ok") return "default";
  if (status === "degraded" || status === "overloaded") return "secondary";
  return "destructive";
}

function StatCard({
  label,
  value,
  hint,
}: {
  label: string;
  value: string | number;
  hint?: string;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
        {hint ? (
          <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function HealthBadge({ status }: { status: string }) {
  return (
    <Badge variant={statusVariant(status)} className="capitalize">
      {status}
    </Badge>
  );
}

function checkStatus(checks: Record<string, unknown> | undefined, key: string): string {
  const raw = checks?.[key];
  if (raw && typeof raw === "object" && raw !== null && "status" in raw) {
    return String((raw as { status: unknown }).status);
  }
  if (raw && typeof raw === "object" && raw !== null && "unhealthy" in raw) {
    const n = Number((raw as { unhealthy: unknown }).unhealthy);
    return n > 0 ? "degraded" : "ok";
  }
  return "—";
}

export function PlatformPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const projectFilter = searchParams.get("project")?.trim() ?? "";
  const unhealthyOnly = searchParams.get("unhealthy") === "1";

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [apiHealth, setApiHealth] = useState<DeepHealth | null>(null);
  const [gatewayHealth, setGatewayHealth] = useState<DeepHealth | null>(null);
  const [routes, setRoutes] = useState<RuntimeRoutesResponse | null>(null);
  const [metrics, setMetrics] = useState<Record<string, number>>({});

  const platformMetrics = useMemo(() => pickPlatformMetrics(metrics), [metrics]);

  const visibleRoutes = useMemo(() => {
    if (!routes?.routes) return [];
    return routes.routes.filter((row) => {
      if (projectFilter && row.project_id !== projectFilter) return false;
      if (unhealthyOnly && row.celld_health === "ok") return false;
      return true;
    });
  }, [routes, projectFilter, unhealthyOnly]);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [deep, runtime, gauges, gw] = await Promise.all([
        getHealthDeep(),
        getRuntimeRoutes(),
        fetchMetricsGauges(),
        getGatewayHealthDeep().catch(() => ({
          status: "unavailable",
          checks: {},
        })),
      ]);
      setApiHealth(deep);
      setRoutes(runtime);
      setMetrics(gauges);
      setGatewayHealth(gw);
    } catch (e) {
      setError(e instanceof CellpApiError ? e.message : "Failed to load platform health");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const id = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const runtimes = apiHealth?.checks?.runtimes as
    | { active_routes?: number; healthy?: number; unhealthy?: number }
    | undefined;

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Breadcrumbs items={[{ label: "Platform" }]} />
          <h1 className="mt-2 text-heading-24 font-semibold tracking-tight">
            Platform health
          </h1>
          <p className="mt-1 text-copy-14 text-muted-foreground">
            cellpd API, gateway, and celld fleet status. Refreshes every 15s.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
          <RefreshCw className={cn("mr-2 size-4", loading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {error ? (
        <div className="rounded-md border border-destructive/40 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        {loading && !apiHealth ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 rounded-lg" />
          ))
        ) : (
          <>
            <StatCard
              label="Pending jobs"
              value={metrics.cellp_pending_jobs ?? "—"}
            />
            <StatCard
              label="Active routes"
              value={routes?.summary.active_routes ?? runtimes?.active_routes ?? "—"}
            />
            <StatCard
              label="Healthy celld"
              value={routes?.summary.healthy ?? runtimes?.healthy ?? metrics.cellp_celld_healthy ?? "—"}
              hint={
                (routes?.summary.unhealthy ?? runtimes?.unhealthy ?? metrics.cellp_celld_unhealthy)
                  ? `${routes?.summary.unhealthy ?? runtimes?.unhealthy ?? metrics.cellp_celld_unhealthy} unhealthy`
                  : undefined
              }
            />
            <StatCard
              label="Gateway requests"
              value={platformMetrics.gatewayRequests ?? "—"}
            />
            <StatCard
              label="Gateway 5xx"
              value={platformMetrics.gateway5xx ?? "—"}
              hint={
                platformMetrics.gatewayUpstream5xx != null
                  ? `upstream ${platformMetrics.gatewayUpstream5xx}`
                  : undefined
              }
            />
          </>
        )}
      </div>

      <div className="flex flex-wrap items-end gap-4 rounded-lg border border-border bg-muted/20 px-4 py-3">
        <label className="flex min-w-[12rem] flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Filter project</span>
          <input
            className="h-9 rounded-md border border-border bg-background px-3 font-mono text-sm"
            value={projectFilter}
            onChange={(e) => {
              const next = new URLSearchParams(searchParams);
              const v = e.target.value.trim();
              if (v) next.set("project", v);
              else next.delete("project");
              setSearchParams(next, { replace: true });
            }}
            placeholder="demo-app"
          />
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={unhealthyOnly}
            onChange={(e) => {
              const next = new URLSearchParams(searchParams);
              if (e.target.checked) next.set("unhealthy", "1");
              else next.delete("unhealthy");
              setSearchParams(next, { replace: true });
            }}
          />
          Unhealthy celld only
        </label>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-base">API (cellpd)</CardTitle>
            {apiHealth ? <HealthBadge status={apiHealth.status} /> : <Skeleton className="h-5 w-14" />}
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label="Registry" value={checkStatus(apiHealth?.checks, "registry")} />
            <Row label="RustFS" value={checkStatus(apiHealth?.checks, "rustfs")} />
            <Row label="celld" value={checkStatus(apiHealth?.checks, "celld")} />
            <Row label="Fleet" value={checkStatus(apiHealth?.checks, "runtimes")} />
            <Row
              label="Queue"
              value={
                typeof apiHealth?.checks?.queue === "object" &&
                apiHealth.checks.queue !== null &&
                "pending_jobs" in (apiHealth.checks.queue as object)
                  ? `${(apiHealth.checks.queue as { pending_jobs: number }).pending_jobs} pending`
                  : "—"
              }
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-base">Gateway</CardTitle>
            {gatewayHealth ? (
              <HealthBadge status={gatewayHealth.status} />
            ) : (
              <Skeleton className="h-5 w-14" />
            )}
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label="Registry" value={checkStatus(gatewayHealth?.checks, "registry")} />
            <Row
              label="Routes"
              value={
                typeof gatewayHealth?.checks?.routes === "object" &&
                gatewayHealth.checks.routes !== null &&
                "active" in (gatewayHealth.checks.routes as object)
                  ? String((gatewayHealth.checks.routes as { active: number }).active)
                  : "—"
              }
            />
            <Row label="Sample upstream" value={checkStatus(gatewayHealth?.checks, "sample_upstream")} />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center gap-2 pb-2">
          <Activity className="size-4 text-muted-foreground" />
          <CardTitle className="text-base">Runtime routes</CardTitle>
        </CardHeader>
        <CardContent className="p-0 pb-2">
          {loading && !routes ? (
            <div className="space-y-2 p-6">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          ) : visibleRoutes.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Project</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Upstream</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>celld</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visibleRoutes.map((row) => (
                  <TableRow key={`${row.project_id}/${row.version_id}`}>
                    <TableCell className="font-mono text-xs">{row.project_id}</TableCell>
                    <TableCell className="font-mono text-xs">
                      <Link
                        to={versionHref(row.project_id, row.version_id)}
                        className="hover:underline"
                      >
                        {row.version_id}
                      </Link>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {row.upstream}
                    </TableCell>
                    <TableCell className="text-xs capitalize">
                      {row.version_status || "—"}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={row.celld_health === "ok" ? "default" : "destructive"}
                        className="text-xs capitalize"
                      >
                        {row.celld_health}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <p className="px-6 py-4 text-sm text-muted-foreground">
              {routes && routes.routes.length > 0
                ? "No routes match the current filters."
                : "No active routes."}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono text-xs capitalize">{value}</span>
    </div>
  );
}

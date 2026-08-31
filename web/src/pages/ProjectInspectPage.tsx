import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Activity, AlertTriangle, RefreshCw } from "lucide-react";
import {
  CellpApiError,
  getBindings,
  getProject,
  getHealthDeep,
  getRuntimeRoutes,
  listVersions,
  fetchMetricsGauges,
  type Bindings,
  type DeepHealth,
  type ProjectDetail,
  type RuntimeRouteRow,
  type Version,
} from "@/lib/cellp-api";
import {
  countUnhealthyRoutes,
  pickPlatformMetrics,
  routesForProject,
  summarizeVersionFleet,
} from "@/lib/inspection";
import {
  deploymentsHref,
  platformHref,
  storageHref,
  versionHref,
} from "@/lib/routes";
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
import { cn } from "@/lib/utils";
import { isInProgressStatus, statusLabel } from "@/lib/status";

const REFRESH_MS = 20_000;

function bindingCounts(b: Bindings | null) {
  if (!b) return null;
  return {
    d1: b.d1.length,
    kv: b.kv.length,
    queues: b.queues.length,
    workflows: b.workflows.length,
    r2: b.r2.length,
    crons: b.crons.length,
  };
}

export function ProjectInspectPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [project, setProject] = useState<ProjectDetail | null>(null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [routeRows, setRouteRows] = useState<RuntimeRouteRow[]>([]);
  const [deepHealth, setDeepHealth] = useState<DeepHealth | null>(null);
  const [metrics, setMetrics] = useState(pickPlatformMetrics({}));
  const [prodBindings, setProdBindings] = useState<Bindings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [proj, verPage, runtime, deep, gauges] = await Promise.all([
        getProject(id),
        listVersions(id, { limit: 100 }),
        getRuntimeRoutes(),
        getHealthDeep(),
        fetchMetricsGauges(),
      ]);
      setProject(proj);
      setVersions(
        verPage.versions.filter(
          (v) => v.status !== "destroyed" && v.status !== "draining",
        ),
      );
      setRouteRows(routesForProject(runtime.routes, id));
      setDeepHealth(deep);
      setMetrics(pickPlatformMetrics(gauges));

      if (proj.prod_version_id) {
        const prod = verPage.versions.find((v) => v.id === proj.prod_version_id);
        if (prod?.status === "ready") {
          try {
            setProdBindings(await getBindings(id, proj.prod_version_id));
          } catch {
            setProdBindings(null);
          }
        } else {
          setProdBindings(null);
        }
      } else {
        setProdBindings(null);
      }
    } catch (e) {
      setError(e instanceof CellpApiError ? e.message : "Failed to load inspection data");
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(timer);
  }, [load]);

  const fleet = useMemo(() => summarizeVersionFleet(versions), [versions]);
  const unhealthyRoutes = countUnhealthyRoutes(routeRows);
  const bindings = bindingCounts(prodBindings);

  const attention = useMemo(() => {
    const rows: Version[] = [];
    for (const v of versions) {
      if (v.status === "failed") rows.push(v);
      else if (isInProgressStatus(v.status)) rows.push(v);
    }
    const routeByVersion = new Map(routeRows.map((r) => [r.version_id, r]));
    for (const v of versions) {
      if (v.status !== "ready") continue;
      const r = routeByVersion.get(v.id);
      if (r && r.celld_health !== "ok" && r.celld_health !== "healthy") {
        if (!rows.some((x) => x.id === v.id)) rows.push(v);
      }
    }
    return rows.slice(0, 8);
  }, [versions, routeRows]);

  const queuePending =
    deepHealth?.checks?.queue &&
    typeof deepHealth.checks.queue === "object" &&
    deepHealth.checks.queue !== null &&
    "pending_jobs" in deepHealth.checks.queue
      ? Number((deepHealth.checks.queue as { pending_jobs: number }).pending_jobs)
      : metrics.pendingJobs;

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Breadcrumbs
            items={[
              { label: "Projects", to: "/" },
              { label: id, to: `/projects/${id}` },
              { label: "Inspect" },
            ]}
          />
          <h1 className="mt-2 text-heading-24 font-semibold tracking-tight">
            Inspect
          </h1>
          <p className="mt-1 text-copy-14 text-muted-foreground">
            Fleet status, gateway routes, and prod bindings for this project. Refreshes
            every 20s. Platform-wide metrics:{" "}
            <Link to={platformHref(id)} className="underline underline-offset-2">
              Platform health
            </Link>
            .
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

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {loading && !project ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 rounded-lg" />
          ))
        ) : (
          <>
            <MetricCard label="Ready versions" value={fleet.ready} />
            <MetricCard
              label="In progress"
              value={fleet.inProgress}
              warn={fleet.inProgress > 0}
            />
            <MetricCard
              label="Unhealthy routes"
              value={unhealthyRoutes}
              warn={unhealthyRoutes > 0}
            />
            <MetricCard
              label="Orchestrator queue"
              value={queuePending ?? "—"}
              warn={typeof queuePending === "number" && queuePending > 0}
            />
          </>
        )}
      </div>

      {attention.length > 0 ? (
        <Card className="border-amber-500/30">
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base">
              <AlertTriangle className="size-4 text-amber-600" />
              Needs attention
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {attention.map((v) => (
              <div
                key={v.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border/60 px-3 py-2 text-sm"
              >
                <Link
                  to={versionHref(id, v.id)}
                  className="font-mono font-medium hover:underline"
                >
                  {v.id}
                </Link>
                <Badge variant="secondary" className="capitalize">
                  {statusLabel(v.status)}
                </Badge>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Production</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row
              label="prod_version_id"
              value={project?.prod_version_id ?? "—"}
            />
            {project?.prod_version_id ? (
              <Link
                to={versionHref(id, project.prod_version_id)}
                className="inline-block text-xs text-primary hover:underline"
              >
                Open version →
              </Link>
            ) : null}
            {bindings ? (
              <p className="text-xs text-muted-foreground">
                Bindings (prod): D1 {bindings.d1} · KV {bindings.kv} · Queues{" "}
                {bindings.queues} · Workflows {bindings.workflows} · R2 {bindings.r2} ·
                Cron {bindings.crons}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                No ready prod bindings snapshot.
              </p>
            )}
            {project?.prod_version_id ? (
              <Link
                to={storageHref(id)}
                className="inline-block text-xs text-muted-foreground hover:text-foreground hover:underline"
              >
                Storage hub →
              </Link>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Platform signals</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label="Global celld unhealthy" value={metrics.celldUnhealthy ?? "—"} />
            <Row label="Gateway 5xx (cellp)" value={metrics.gateway5xx ?? "—"} />
            <Row label="Gateway upstream 5xx" value={metrics.gatewayUpstream5xx ?? "—"} />
            <p className="pt-1 text-xs text-muted-foreground">
              Scraped from <span className="font-mono">/metrics</span>. Use Grafana for
              history — see observability docs.
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center gap-2 pb-2">
          <Activity className="size-4 text-muted-foreground" />
          <CardTitle className="text-base">Runtime routes (this project)</CardTitle>
        </CardHeader>
        <CardContent className="p-0 pb-2">
          {loading && routeRows.length === 0 ? (
            <Skeleton className="mx-6 mb-4 h-24" />
          ) : routeRows.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Version</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>celld</TableHead>
                  <TableHead>Upstream</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {routeRows.map((row) => (
                  <TableRow key={row.version_id}>
                    <TableCell>
                      <Link
                        to={versionHref(id, row.version_id)}
                        className="font-mono text-xs hover:underline"
                      >
                        {row.version_id}
                      </Link>
                    </TableCell>
                    <TableCell className="text-xs capitalize">
                      {row.version_status || "—"}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          row.celld_health === "ok" ? "default" : "destructive"
                        }
                        className="text-xs capitalize"
                      >
                        {row.celld_health}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[14rem] truncate font-mono text-xs text-muted-foreground">
                      {row.upstream}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <p className="px-6 py-4 text-sm text-muted-foreground">
              No active gateway routes for this project.{" "}
              <Link to={deploymentsHref(id)} className="underline">
                Deployments
              </Link>
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function MetricCard({
  label,
  value,
  warn,
}: {
  label: string;
  value: string | number;
  warn?: boolean;
}) {
  return (
    <Card className={warn ? "border-amber-500/40" : undefined}>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
      </CardContent>
    </Card>
  );
}

function Row({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono text-xs">{value}</span>
    </div>
  );
}

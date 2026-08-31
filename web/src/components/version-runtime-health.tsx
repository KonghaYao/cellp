import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  CellpApiError,
  getRuntimeRoutes,
  type RuntimeRouteRow,
} from "@/lib/cellp-api";
import { isCelldRouteHealthy, routeForVersion } from "@/lib/inspection";
import { inspectHref } from "@/lib/routes";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";

interface VersionRuntimeHealthProps {
  projectId: string;
  versionId: string;
  versionStatus: string;
}

export function VersionRuntimeHealth({
  projectId,
  versionId,
  versionStatus,
}: VersionRuntimeHealthProps) {
  const [row, setRow] = useState<RuntimeRouteRow | null | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await getRuntimeRoutes();
        if (cancelled) return;
        const match = routeForVersion(data.routes, projectId, versionId);
        setRow(match ?? null);
        setError(null);
      } catch (e) {
        if (!cancelled) {
          setRow(null);
          setError(
            e instanceof CellpApiError ? e.message : "Runtime routes unavailable",
          );
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, versionId]);

  if (row === undefined) {
    return (
      <div
        className="rounded-lg border border-border bg-muted/20 px-4 py-3"
        data-testid="version-runtime-health"
      >
        <Skeleton className="h-5 w-48" />
      </div>
    );
  }

  const onGateway = row !== null;
  const healthy = row ? isCelldRouteHealthy(row.celld_health) : false;

  return (
    <div
      className="rounded-lg border border-border bg-muted/20 px-4 py-3 text-sm"
      data-testid="version-runtime-health"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-medium text-foreground">Runtime inspection</p>
        <Link
          to={inspectHref(projectId)}
          className="text-xs text-muted-foreground hover:text-foreground hover:underline"
        >
          Project inspect →
        </Link>
      </div>
      <dl className="mt-2 grid gap-2 sm:grid-cols-2">
        <div>
          <dt className="text-xs text-muted-foreground">Gateway route</dt>
          <dd className="mt-0.5 font-mono text-xs">
            {onGateway ? "active" : versionStatus === "ready" ? "not routed" : "—"}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">celld upstream</dt>
          <dd className="mt-0.5">
            {row ? (
              <Badge variant={healthy ? "default" : "destructive"} className="text-xs capitalize">
                {row.celld_health}
              </Badge>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </dd>
        </div>
        {row?.upstream ? (
          <div className="sm:col-span-2">
            <dt className="text-xs text-muted-foreground">Upstream</dt>
            <dd className="mt-0.5 break-all font-mono text-xs text-muted-foreground">
              {row.upstream}
            </dd>
          </div>
        ) : null}
      </dl>
      {error ? (
        <p className="mt-2 text-xs text-destructive" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

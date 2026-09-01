import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { Layers } from "lucide-react";
import {
  getProject,
  getBindings,
  listVersions,
  hasAnyBindings,
  CellpApiError,
  type Bindings,
  type Version,
} from "@/lib/cellp-api";
import {
  storageBrowserHref,
  storageKvHref,
  storageQueuesHref,
  storageWorkflowsHref,
  versionHref,
} from "@/lib/routes";
import { Ad7Banner } from "@/components/bindings/ad7-banner";
import { EmptyState } from "@/components/empty-state";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip } from "@/components/ui/tooltip";
import { StatusIndicator } from "@/components/status-indicator";
import { cn } from "@/lib/utils";

const R2_TOOLTIP =
  "No celld r2 CLI — object browser is not available. Preview objects inherit parent bucket via branch.";
const CRON_TOOLTIP_PREFIX = "Cron is triggered by celld; no run-once action.";

type VersionRow = {
  version: Version;
  bindings: Bindings;
};

function BindingBadge({
  type,
  count,
  href,
  tooltip,
}: {
  type: string;
  count: number;
  href?: string;
  tooltip?: string;
}) {
  if (count <= 0) return null;
  const label = `${type} ${count}`;
  const badge = (
    <Badge
      variant="outline"
      data-binding-type={type}
      className={cn(
        "normal-case tracking-normal font-mono",
        href && "hover:border-foreground/40 hover:text-foreground",
      )}
    >
      {label}
    </Badge>
  );

  const inner = href ? (
    <Link to={href} className="inline-flex">
      {badge}
    </Link>
  ) : (
    badge
  );

  if (tooltip) {
    return <Tooltip content={tooltip}>{inner}</Tooltip>;
  }
  return inner;
}

export function StoragePage() {
  const { id = "" } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const highlightVersion = searchParams.get("version");
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [rows, setRows] = useState<VersionRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const project = await getProject(id);
        const all: Version[] = [];
        let cursor: string | null = null;
        do {
          const page = await listVersions(id, { cursor });
          all.push(...page.versions);
          cursor = page.next_cursor;
        } while (cursor);
        const ready = all.filter((v) => v.status === "ready");
        const withBindings: VersionRow[] = [];
        const bindingErrors: string[] = [];
        for (const version of ready) {
          try {
            const bindings = await getBindings(id, version.id);
            withBindings.push({ version, bindings });
          } catch (e) {
            const msg =
              e instanceof CellpApiError
                ? `${version.id}: ${e.message}`
                : `${version.id}: failed to load bindings`;
            bindingErrors.push(msg);
          }
        }
        if (cancelled) return;
        setProdVersionId(project.prod_version_id);
        setRows(withBindings);
        if (bindingErrors.length > 0 && withBindings.length === 0) {
          setError(bindingErrors.join(" · "));
        } else if (bindingErrors.length > 0) {
          setError(`Some bindings failed: ${bindingErrors.join(" · ")}`);
        } else {
          setError(null);
        }
      } catch (e) {
        if (!cancelled) {
          setError(
            e instanceof CellpApiError
              ? e.message
              : "Failed to load storage",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id]);

  const hasPreview = useMemo(
    () => rows.some((row) => row.version.parent_version_id != null),
    [rows],
  );

  const anyBindings = useMemo(
    () => rows.some((row) => hasAnyBindings(row.bindings)),
    [rows],
  );

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-heading-24 font-semibold tracking-tight">Storage</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          Bindings for ready deployments in{" "}
          <span className="font-mono">{id}</span>
        </p>
      </div>

      {hasPreview && <Ad7Banner />}

      {loading && <Skeleton className="h-48 w-full" />}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && !error && rows.length === 0 && (
        <EmptyState
          title="No ready deployments"
          description="Deploy a version to inspect its wrangler bindings."
          icon={<Layers className="size-6 text-muted-foreground" />}
        />
      )}

      {!loading && !error && rows.length > 0 && !anyBindings && (
        <EmptyState
          title="Ready deployments have no wrangler bindings"
          description="This project’s ready versions do not declare D1, KV, Queue, Workflow, R2, or Cron bindings."
          icon={<Layers className="size-6 text-muted-foreground" />}
        />
      )}

      {!loading && !error && rows.length > 0 && anyBindings && (
        <div className="overflow-hidden rounded-md border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="text-label-12">Deployment</TableHead>
                <TableHead className="text-label-12">Bindings</TableHead>
                <TableHead className="text-right text-label-12">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map(({ version, bindings }) => {
                const isProd = prodVersionId === version.id;
                const isPreview = version.parent_version_id != null;
                const cronTooltip =
                  bindings.crons.length > 0
                    ? `${CRON_TOOLTIP_PREFIX} ${bindings.crons.join(", ")}`
                    : CRON_TOOLTIP_PREFIX;
                return (
                  <TableRow
                    key={version.id}
                    id={`version-${version.id}`}
                    className={cn(
                      "hover:bg-muted/40",
                      highlightVersion === version.id && "bg-accent/40",
                    )}
                  >
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Link
                          to={versionHref(id, version.id)}
                          className="font-mono text-sm hover:underline"
                        >
                          {version.id}
                        </Link>
                        {isProd && <Badge variant="prod">Production</Badge>}
                      </div>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {version.git_ref || "—"}
                      </p>
                      <div className="mt-1">
                        <StatusIndicator status={version.status} />
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1.5">
                        <BindingBadge
                          type="d1"
                          count={bindings.d1.length}
                          href={storageBrowserHref(id, version.id)}
                        />
                        <BindingBadge
                          type="kv"
                          count={bindings.kv.length}
                          href={storageKvHref(id, version.id)}
                        />
                        <BindingBadge
                          type="queue"
                          count={bindings.queues.length}
                          href={storageQueuesHref(id, version.id)}
                        />
                        <BindingBadge
                          type="workflow"
                          count={bindings.workflows.length}
                          href={storageWorkflowsHref(id, version.id)}
                        />
                        <BindingBadge
                          type="r2"
                          count={bindings.r2.length}
                          tooltip={R2_TOOLTIP}
                        />
                        <BindingBadge
                          type="cron"
                          count={bindings.crons.length}
                          tooltip={cronTooltip}
                        />
                      </div>
                      {isPreview && (
                        <p className="mt-2 text-xs text-muted-foreground">
                          Preview KV / Queue branch from parent. Workflow / Cron start empty.
                        </p>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-col items-end gap-1">
                        {bindings.d1.length > 0 && (
                          <Link
                            to={storageBrowserHref(id, version.id)}
                            className="text-sm font-medium hover:underline"
                          >
                            Open D1
                          </Link>
                        )}
                        {bindings.kv.length > 0 && (
                          <Link
                            to={storageKvHref(id, version.id)}
                            className="text-sm font-medium hover:underline"
                          >
                            Open KV
                          </Link>
                        )}
                        {bindings.queues.length > 0 && (
                          <Link
                            to={storageQueuesHref(id, version.id)}
                            className="text-sm font-medium hover:underline"
                          >
                            Open queues
                          </Link>
                        )}
                        {bindings.workflows.length > 0 && (
                          <Link
                            to={storageWorkflowsHref(id, version.id)}
                            className="text-sm font-medium hover:underline"
                          >
                            Open workflows
                          </Link>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}

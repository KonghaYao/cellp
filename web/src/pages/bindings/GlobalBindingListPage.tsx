import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { RefreshCw } from "lucide-react";
import {
  BINDING_KIND_LABEL,
  loadBindingRegistry,
  type BindingHealth,
  type BindingInstanceRow,
  type BindingKind,
} from "@/lib/binding-registry";
import { CellpApiError } from "@/lib/cellp-api";
import { projectOverviewHref, versionHref } from "@/lib/routes";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

const HEALTH_LABEL: Record<BindingHealth, string> = {
  ok: "Active",
  configured: "Configured",
  error: "Error",
};

function HealthBadge({ health, message }: { health: BindingHealth; message?: string }) {
  const variant =
    health === "ok" ? "default" : health === "error" ? "destructive" : "secondary";
  return (
    <span title={message} className="inline-flex flex-col items-start gap-0.5">
      <Badge variant={variant}>{HEALTH_LABEL[health]}</Badge>
      {message ? (
        <span className="max-w-[14rem] text-xs text-muted-foreground">{message}</span>
      ) : null}
    </span>
  );
}

export function GlobalBindingListPage({
  kind,
  emptyDescription,
}: {
  kind: BindingKind;
  emptyDescription: string;
}) {
  const [rows, setRows] = useState<BindingInstanceRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await loadBindingRegistry(kind, { probe: true });
      setRows(data);
    } catch (e) {
      setRows([]);
      setError(
        e instanceof CellpApiError ? e.message : `Failed to load ${BINDING_KIND_LABEL[kind]} instances`,
      );
    } finally {
      setLoading(false);
    }
  }, [kind]);

  useEffect(() => {
    void load();
  }, [load]);

  const label = BINDING_KIND_LABEL[kind];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          {rows.length} {label} instance{rows.length === 1 ? "" : "s"} across ready
          deployments
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void load()}
          disabled={loading}
        >
          <RefreshCw className={cn("mr-2 size-3.5", loading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {loading && <Skeleton className="h-48 w-full" />}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && !error && rows.length === 0 && (
        <EmptyState
          title={`No ${label} instances`}
          description={emptyDescription}
        />
      )}

      {!loading && !error && rows.length > 0 && (
        <div className="overflow-hidden rounded-md border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead>Project</TableHead>
                <TableHead>Version</TableHead>
                <TableHead>Binding</TableHead>
                <TableHead>Instance</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Open</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.key} className="hover:bg-muted/40">
                  <TableCell>
                    <Link
                      to={projectOverviewHref(row.projectId)}
                      className="font-mono text-sm hover:underline"
                    >
                      {row.projectId}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Link
                        to={versionHref(row.projectId, row.versionId)}
                        className="font-mono text-sm hover:underline"
                      >
                        {row.versionId}
                      </Link>
                      {row.isProd && <Badge variant="prod">Production</Badge>}
                    </div>
                  </TableCell>
                  <TableCell className="font-mono text-sm">{row.binding}</TableCell>
                  <TableCell>
                    <div className="font-mono text-sm">{row.name}</div>
                    {row.detail ? (
                      <div className="text-xs text-muted-foreground">{row.detail}</div>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <HealthBadge health={row.health} message={row.healthMessage} />
                  </TableCell>
                  <TableCell className="text-right">
                    {row.manageHref ? (
                      <Link
                        to={row.manageHref}
                        className="text-sm font-medium hover:underline"
                      >
                        Manage
                      </Link>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}

import { useMemo, useState, useTransition } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Database, Trash2 } from "lucide-react";
import {
  destroyVersion,
  CellpApiError,
  type Version,
} from "@/lib/cellp-api";
import { formatRelativeTime } from "@/lib/format";
import { StatusIndicator } from "@/components/status-indicator";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { storageBrowserHref } from "@/lib/routes";

interface BranchListProps {
  projectId: string;
  currentVersionId: string;
  prodVersionId: string | null;
  versions: Version[];
  onRefresh?: () => void | Promise<void>;
}

function sortVersionsNewestFirst(versions: Version[]): Version[] {
  return [...versions].sort((a, b) => {
    const aTime = a.created_at ? new Date(a.created_at).getTime() : 0;
    const bTime = b.created_at ? new Date(b.created_at).getTime() : 0;
    if (aTime !== bTime) return bTime - aTime;
    return b.id.localeCompare(a.id);
  });
}

function hasChildBranches(versionId: string, versions: Version[]): boolean {
  return versions.some(
    (v) =>
      v.parent_version_id === versionId &&
      v.status !== "destroyed" &&
      v.status !== "draining",
  );
}

export function BranchList({
  projectId,
  currentVersionId,
  prodVersionId,
  versions,
  onRefresh,
}: BranchListProps) {
  const navigate = useNavigate();
  const [pending, startTransition] = useTransition();
  const [destroyTarget, setDestroyTarget] = useState<Version | null>(null);
  const [error, setError] = useState<string | null>(null);

  const sorted = useMemo(() => sortVersionsNewestFirst(versions), [versions]);
  const active = sorted.filter(
    (v) => v.status !== "destroyed" && v.status !== "draining",
  );

  function handleDestroy() {
    if (!destroyTarget) return;
    setError(null);
    startTransition(async () => {
      try {
        await destroyVersion(projectId, destroyTarget.id);
        setDestroyTarget(null);
        if (destroyTarget.id === currentVersionId) {
          const fallback =
            active.find((v) => v.id === prodVersionId)?.id ??
            active.find((v) => v.id !== destroyTarget.id)?.id;
          if (fallback) {
            navigate(storageBrowserHref(projectId, fallback));
          }
        }
        await onRefresh?.();
      } catch (e) {
        setError(
          e instanceof CellpApiError ? e.message : "Failed to delete branch",
        );
      }
    });
  }

  const destroyBlocked =
    destroyTarget != null &&
    (prodVersionId === destroyTarget.id ||
      hasChildBranches(destroyTarget.id, versions));

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Branch</TableHead>
              <TableHead>Parent</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {active.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="py-8 text-center text-sm text-muted-foreground"
                >
                  No branches yet
                </TableCell>
              </TableRow>
            ) : (
              active.map((version) => {
                const isProd = prodVersionId === version.id;
                const isCurrent = currentVersionId === version.id;
                const canDelete =
                  !isProd &&
                  !hasChildBranches(version.id, versions) &&
                  version.status !== "destroyed" &&
                  version.status !== "draining";

                return (
                  <TableRow
                    key={version.id}
                    className={isCurrent ? "bg-accent/30" : undefined}
                  >
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-2">
                        <Link
                          to={storageBrowserHref(projectId, version.id)}
                          className="font-mono text-sm font-medium hover:underline"
                        >
                          {version.id}
                        </Link>
                        {isProd && <Badge variant="prod">Production</Badge>}
                        {isCurrent && (
                          <Badge variant="outline">Current</Badge>
                        )}
                      </div>
                      <p className="mt-0.5 font-mono text-xs text-muted-foreground">
                        {version.data_branch || "—"}
                      </p>
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {version.parent_version_id ? (
                        <Link
                          to={storageBrowserHref(
                            projectId,
                            version.parent_version_id,
                          )}
                          className="hover:underline"
                        >
                          {version.parent_version_id}
                        </Link>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusIndicator status={version.status} />
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {version.created_at
                        ? formatRelativeTime(version.created_at)
                        : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        {!isCurrent && version.status === "ready" && (
                          <Link
                            to={storageBrowserHref(projectId, version.id)}
                            className="inline-flex h-8 items-center gap-2 rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
                          >
                            <Database className="size-3.5" />
                            Switch
                          </Link>
                        )}
                        {canDelete && (
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => setDestroyTarget(version)}
                            disabled={pending}
                          >
                            <Trash2 className="size-3.5" />
                            Delete
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>

      {error && (
        <p className="text-sm text-destructive">{error}</p>
      )}

      <ConfirmDialog
        open={destroyTarget != null}
        title={destroyBlocked ? "Cannot delete branch" : "Delete branch?"}
        description={
          destroyBlocked
            ? destroyTarget && prodVersionId === destroyTarget.id
              ? `${destroyTarget.id} is the production branch and cannot be deleted.`
              : `${destroyTarget?.id} has child branches. Delete children first.`
            : `Branch ${destroyTarget?.id} will be destroyed. This cannot be undone.`
        }
        confirmLabel={destroyBlocked ? "OK" : "Delete"}
        destructive={!destroyBlocked}
        loading={pending && !destroyBlocked}
        onConfirm={
          destroyBlocked ? () => setDestroyTarget(null) : handleDestroy
        }
        onCancel={() => setDestroyTarget(null)}
      />
    </div>
  );
}

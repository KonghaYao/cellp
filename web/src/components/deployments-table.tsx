import { useMemo, useState, useTransition } from "react";
import { Link } from "react-router-dom";
import { ExternalLink, GitBranch, MoreHorizontal, Rocket } from "lucide-react";
import type { Version } from "@/lib/cellp-api";
import { promoteVersion, CellpApiError } from "@/lib/cellp-api";
import { deriveProdUrl, formatRelativeTime, truncateSha } from "@/lib/format";
import { StatusIndicator } from "@/components/status-indicator";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfirmDialog } from "@/components/confirm-dialog";

interface DeploymentsTableProps {
  projectId: string;
  prodVersionId: string | null;
  versions: Version[];
  prodUrl: string;
  onRefresh?: () => void | Promise<void>;
  dense?: boolean;
}

function sortVersionsNewestFirst(versions: Version[]): Version[] {
  return [...versions].sort((a, b) => {
    const aTime = a.created_at ? new Date(a.created_at).getTime() : 0;
    const bTime = b.created_at ? new Date(b.created_at).getTime() : 0;
    if (aTime !== bTime) return bTime - aTime;
    return b.id.localeCompare(a.id);
  });
}

function versionTimestamp(v: Version): string {
  return v.created_at ?? v.ready_at ?? v.updated_at ?? "";
}

export function DeploymentsTable({
  projectId,
  prodVersionId,
  versions,
  prodUrl,
  onRefresh,
  dense = false,
}: DeploymentsTableProps) {
  const sorted = useMemo(() => sortVersionsNewestFirst(versions), [versions]);

  return (
    <>
      <div className="hidden overflow-hidden rounded-md border border-border bg-card md:block">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className={dense ? "text-label-12" : undefined}>
                Deployment
              </TableHead>
              <TableHead className={dense ? "text-label-12" : undefined}>
                Status
              </TableHead>
              <TableHead className={dense ? "text-label-12" : undefined}>
                Git
              </TableHead>
              <TableHead className={dense ? "text-label-12" : undefined}>
                Created
              </TableHead>
              <TableHead
                className={dense ? "text-right text-label-12" : "text-right"}
              >
                Actions
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((version) => {
              const isProd = prodVersionId === version.id;
              return (
                <TableRow
                  key={version.id}
                  className={dense ? "hover:bg-muted/40" : undefined}
                >
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Link
                        to={`/projects/${projectId}/versions/${version.id}`}
                        className="font-mono text-sm font-medium hover:underline"
                      >
                        {version.id}
                      </Link>
                      {isProd && <Badge variant="prod">Production</Badge>}
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusIndicator status={version.status} />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                      <GitBranch className="size-3.5 shrink-0" />
                      <span>{version.git_ref || "—"}</span>
                      <span className="font-mono text-xs text-foreground/70">
                        {truncateSha(version.git_sha)}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatRelativeTime(versionTimestamp(version))}
                  </TableCell>
                  <TableCell>
                    <DeploymentActions
                      projectId={projectId}
                      version={version}
                      isProd={isProd}
                      prodUrl={prodUrl}
                      onRefresh={onRefresh}
                    />
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>

      <div className="space-y-3 md:hidden">
        {sorted.map((version) => {
          const isProd = prodVersionId === version.id;
          return (
            <DeploymentCard
              key={version.id}
              projectId={projectId}
              version={version}
              isProd={isProd}
              prodUrl={prodUrl}
              onRefresh={onRefresh}
            />
          );
        })}
      </div>
    </>
  );
}

function DeploymentCard({
  projectId,
  version,
  isProd,
  prodUrl,
  onRefresh,
}: {
  projectId: string;
  version: Version;
  isProd: boolean;
  prodUrl: string;
  onRefresh?: () => void | Promise<void>;
}) {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="rounded-md border border-border bg-card p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <Link
              to={`/projects/${projectId}/versions/${version.id}`}
              className="font-mono text-sm font-medium hover:underline"
            >
              {version.id}
            </Link>
            {isProd && <Badge variant="prod">Production</Badge>}
          </div>
          <StatusIndicator status={version.status} />
          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <GitBranch className="size-3.5 shrink-0" />
            <span>{version.git_ref || "—"}</span>
            <span className="font-mono text-xs text-foreground/70">
              {truncateSha(version.git_sha)}
            </span>
          </div>
          <p className="text-sm text-muted-foreground">
            {formatRelativeTime(versionTimestamp(version))}
          </p>
        </div>
        <div className="relative shrink-0">
          <button
            type="button"
            aria-label="Deployment actions"
            aria-expanded={menuOpen}
            className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            onClick={() => setMenuOpen((open) => !open)}
          >
            <MoreHorizontal className="size-4" />
          </button>
          {menuOpen && (
            <>
              <button
                type="button"
                aria-label="Close actions menu"
                className="fixed inset-0 z-40"
                onClick={() => setMenuOpen(false)}
              />
              <div className="absolute right-0 z-50 mt-1 min-w-[10rem] rounded-md border border-border bg-card p-1 shadow-lg">
                <DeploymentActions
                  projectId={projectId}
                  version={version}
                  isProd={isProd}
                  prodUrl={prodUrl}
                  onRefresh={onRefresh}
                  stacked
                  onAction={() => setMenuOpen(false)}
                />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function DeploymentActions({
  projectId,
  version,
  isProd,
  prodUrl,
  onRefresh,
  stacked = false,
  onAction,
}: {
  projectId: string;
  version: Version;
  isProd: boolean;
  prodUrl: string;
  onRefresh?: () => void | Promise<void>;
  stacked?: boolean;
  onAction?: () => void;
}) {
  const [pending, startTransition] = useTransition();
  const [promoteOpen, setPromoteOpen] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const canPromote = version.status === "ready" && !isProd;
  const hasPreview = Boolean(version.preview_url);

  function handlePromote() {
    setError(null);
    startTransition(async () => {
      try {
        await promoteVersion(projectId, version.id);
        setPromoteOpen(false);
        setFeedback("Promoted to production");
        onAction?.();
        await onRefresh?.();
        setTimeout(() => setFeedback(null), 3000);
      } catch (e) {
        setError(e instanceof CellpApiError ? e.message : "Promote failed");
      }
    });
  }

  const layoutClass = stacked
    ? "flex flex-col items-stretch gap-1"
    : "flex items-center justify-end gap-1";

  const actionClass = stacked
    ? "inline-flex h-8 w-full items-center justify-start gap-2 rounded-md px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    : "inline-flex h-8 items-center gap-1 rounded-md px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground";

  return (
    <div className={layoutClass}>
      {feedback && (
        <span
          className={
            stacked
              ? "px-3 text-xs text-emerald-400"
              : "mr-2 text-xs text-emerald-400"
          }
        >
          {feedback}
        </span>
      )}
      {error && (
        <span
          className={
            stacked ? "px-3 text-xs text-destructive" : "mr-2 text-xs text-destructive"
          }
        >
          {error}
        </span>
      )}
      {hasPreview && (
        <a
          href={version.preview_url}
          target="_blank"
          rel="noreferrer"
          className={actionClass}
          onClick={onAction}
        >
          Preview
          <ExternalLink className="size-3" />
        </a>
      )}
      {isProd && (
        <a
          href={prodUrl}
          target="_blank"
          rel="noreferrer"
          className={actionClass}
          onClick={onAction}
        >
          Visit
          <ExternalLink className="size-3" />
        </a>
      )}
      {canPromote && (
        <>
          <Button
            variant={stacked ? "ghost" : "outline"}
            size="sm"
            className={stacked ? "w-full justify-start px-3" : undefined}
            onClick={() => {
              setPromoteOpen(true);
              onAction?.();
            }}
            disabled={pending}
          >
            <Rocket className="size-3" />
            Promote
          </Button>
          <ConfirmDialog
            open={promoteOpen}
            title="Promote to production?"
            description={`Version ${version.id} will become the production cutover for ${projectId}.`}
            confirmLabel="Promote"
            loading={pending}
            onConfirm={handlePromote}
            onCancel={() => setPromoteOpen(false)}
          />
        </>
      )}
    </div>
  );
}

/** Re-export for pages that need prod URL derivation */
export { deriveProdUrl };

import { useMemo, useState, useTransition } from "react";
import { Link } from "react-router-dom";
import { ExternalLink, GitBranch, Rocket } from "lucide-react";
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
    <div className="overflow-hidden rounded-md border border-border bg-card">
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
  );
}

function DeploymentActions({
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
        await onRefresh?.();
        setTimeout(() => setFeedback(null), 3000);
      } catch (e) {
        setError(e instanceof CellpApiError ? e.message : "Promote failed");
      }
    });
  }

  return (
    <div className="flex items-center justify-end gap-1">
      {feedback && (
        <span className="mr-2 text-xs text-emerald-400">{feedback}</span>
      )}
      {error && (
        <span className="mr-2 text-xs text-destructive">{error}</span>
      )}
      {hasPreview && (
        <a
          href={version.preview_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-8 items-center gap-1 rounded-md px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
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
          className="inline-flex h-8 items-center gap-1 rounded-md px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          Visit
          <ExternalLink className="size-3" />
        </a>
      )}
      {canPromote && (
        <>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPromoteOpen(true)}
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

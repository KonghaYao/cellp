import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ExternalLink, GitBranch, Globe, Info } from "lucide-react";
import {
  getProject,
  listVersions,
  checkDatabaseAvailability,
  CellpApiError,
  type DatabaseAvailability,
  type ProjectDetail,
  type Version,
} from "@/lib/cellp-api";
import { resolveProdUrl, formatRelativeTime, truncateSha, ingressDisplayUrl, isAppInternalPath } from "@/lib/format";
import { deploymentsHref, inspectHref, storageBrowserHref } from "@/lib/routes";
import { CopyButton } from "@/components/copy-button";
import { DeploymentsTable } from "@/components/deployments-table";
import { CommerceStorefrontEmbed } from "@/components/commerce-storefront-embed";
import { IngressAccessHint } from "@/components/ingress-access-hint";
import { OperatorChecklist } from "@/components/operator-checklist";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";

function sortVersionsNewestFirst(versions: Version[]): Version[] {
  return [...versions].sort((a, b) => {
    const aTime = a.created_at ? new Date(a.created_at).getTime() : 0;
    const bTime = b.created_at ? new Date(b.created_at).getTime() : 0;
    if (aTime !== bTime) return bTime - aTime;
    return b.id.localeCompare(a.id);
  });
}

function versionTimestamp(v: Version): string {
  return v.ready_at ?? v.created_at ?? v.updated_at ?? "";
}

export function ProjectOverviewPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [project, setProject] = useState<ProjectDetail | null>(null);
  const [prodVersion, setProdVersion] = useState<Version | null>(null);
  const [recentVersions, setRecentVersions] = useState<Version[]>([]);
  const [activeVersions, setActiveVersions] = useState<Version[]>([]);
  const [prodDatabase, setProdDatabase] = useState<DatabaseAvailability | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await getProject(id);
        const versionsPage = await listVersions(id, { limit: 50 });
        const activeVersions = versionsPage.versions.filter(
          (v) => v.status !== "destroyed" && v.status !== "draining",
        );
        let prod: Version | null = null;
        let dbAvailability: DatabaseAvailability | null = null;
        if (data.prod_version_id) {
          prod =
            activeVersions.find((v) => v.id === data.prod_version_id) ??
            null;
          dbAvailability = await checkDatabaseAvailability(
            id,
            data.prod_version_id,
          );
        }
        if (cancelled) return;
        setProject(data);
        setProdVersion(prod);
        const sorted = sortVersionsNewestFirst(activeVersions);
        setActiveVersions(sorted);
        setRecentVersions(sorted.slice(0, 5));
        setProdDatabase(dbAvailability);
        setError(null);
        setNotFound(false);
      } catch (e) {
        if (!cancelled) {
          if (e instanceof CellpApiError && e.status === 404) {
            setNotFound(true);
          } else {
            setError(
              e instanceof CellpApiError
                ? `${e.message} (${e.status})`
                : "Failed to load project",
            );
          }
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id]);

  const prodOpenUrl = resolveProdUrl(
    id,
    project?.prod_url,
    prodVersion?.preview_url,
  );
  const prodDisplayUrl = ingressDisplayUrl(
    id,
    undefined,
    project?.prod_url ?? undefined,
  );
  const deploymentCount = project?.version_count ?? recentVersions.length;
  const prodDbAvailable = prodDatabase?.available === true;

  const recentForTable = useMemo(
    () => recentVersions,
    [recentVersions],
  );

  if (notFound) {
    return (
      <div className="space-y-4 py-16 text-center">
        <h1 className="text-2xl font-semibold">Project not found</h1>
        <p>
          <Link to="/" className="text-sm hover:underline">
            Back to projects
          </Link>
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-heading-24 font-semibold tracking-tight">{id}</h1>
          {project?.git_remote && (
            <div className="mt-2 flex items-center gap-2 text-sm text-muted-foreground">
              <GitBranch className="size-3.5" />
              <span className="font-mono text-xs">{project.git_remote}</span>
              <CopyButton value={project.git_remote} label="Copy git remote" />
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Link
            to={inspectHref(id)}
            className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
          >
            Inspect
          </Link>
          {project?.prod_version_id && prodDbAvailable && (
            <Link
              to={storageBrowserHref(id, project.prod_version_id)}
              className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
            >
              Open storage
            </Link>
          )}
        </div>
      </div>

      {!loading && (
        <OperatorChecklist
          projectId={id}
          prodVersionId={project?.prod_version_id ?? null}
          versions={activeVersions}
        />
      )}

      {loading ? (
        <Skeleton className="h-40 w-full rounded-md" />
      ) : (
        <div className="rounded-md border border-border bg-card p-6">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-label-14 font-medium">Production</h2>
                {project?.prod_version_id && (
                  <Badge variant="prod">Live</Badge>
                )}
              </div>
              {project?.prod_version_id && prodVersion ? (
                <>
                  <p className="font-mono text-xl font-semibold tracking-tight">
                    {project.prod_version_id}
                  </p>
                  <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
                    <span className="inline-flex items-center gap-1.5">
                      <GitBranch className="size-3.5" />
                      {prodVersion.git_ref || "—"}
                      <span className="font-mono text-xs text-foreground/70">
                        {truncateSha(prodVersion.git_sha)}
                      </span>
                    </span>
                    <span>
                      Deployed {formatRelativeTime(versionTimestamp(prodVersion))}
                    </span>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm break-all">{prodDisplayUrl}</span>
                    <CopyButton value={prodDisplayUrl} label="Copy production URL" />
                  </div>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">
                  No production deployment yet. Promote a ready deployment to go
                  live.
                </p>
              )}
            </div>
            {project?.prod_version_id && (
            isAppInternalPath(prodOpenUrl) ? (
              <Link
                to={prodOpenUrl}
                className="inline-flex h-8 items-center gap-2 rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
              >
                <Globe className="size-3.5" />
                {id === "commerce-store" ? "Open storefront" : "Visit"}
              </Link>
            ) : (
            <a
              href={prodOpenUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-8 items-center gap-2 rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
            >
              <Globe className="size-3.5" />
              {id === "commerce-store" ? "Open storefront" : "Visit"}
              <ExternalLink className="size-3 text-muted-foreground" />
            </a>
            )
          )}
          </div>
        </div>
      )}

      {!loading && project?.prod_version_id && (
        <IngressAccessHint
          projectId={id}
          versionId={project.prod_version_id}
          prodUrl={project.prod_url}
        />
      )}

      {!loading && id === "commerce-store" && project?.prod_version_id && (
        <CommerceStorefrontEmbed
          projectId={id}
          versionId={project.prod_version_id}
          prodUrl={project.prod_url}
          isProd
        />
      )}

      {loading ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          <OverviewCard label="Total deployments">
            <span className="text-2xl font-semibold tabular-nums">
              {deploymentCount}
            </span>
          </OverviewCard>
          <OverviewCard label="Project created">
            {project?.created_at
              ? formatRelativeTime(project.created_at)
              : "—"}
          </OverviewCard>
        </div>
      )}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && recentForTable.length > 0 && (
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-4">
            <h2 className="text-label-14 font-medium">Recent deployments</h2>
            <Link
              to={deploymentsHref(id)}
              className="text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              View all
            </Link>
          </div>
          <DeploymentsTable
            projectId={id}
            prodVersionId={project?.prod_version_id ?? null}
            versions={recentForTable}
            prodUrl={prodOpenUrl}
            dense
          />
        </div>
      )}

      {project?.prod_version_id &&
        prodDatabase &&
        !prodDatabase.available && (
          <div className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm">
            <div className="flex items-start gap-2">
              <Info className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div>
                <p className="font-medium">Production database unavailable</p>
                <p className="mt-1 text-muted-foreground">
                  {prodDatabase.message}. Attach a D1 database to your
                  deployment to browse storage from the dashboard.{" "}
                  <a
                    href="https://github.com/cursor/cellp#readme"
                    target="_blank"
                    rel="noreferrer"
                    className="font-medium text-foreground hover:underline"
                  >
                    Read the cellp docs
                  </a>
                </p>
              </div>
            </div>
          </div>
        )}

      <div className="rounded-md border border-border bg-card p-6">
        <h2 className="text-label-14 font-medium">Quick links</h2>
        <div className="mt-4 flex flex-wrap gap-2">
          <Link
            to={deploymentsHref(id)}
            className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
          >
            View deployments
          </Link>
          {project?.prod_version_id && prodDbAvailable && (
            <Link
              to={storageBrowserHref(id, project.prod_version_id)}
              className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
            >
              Browse storage bindings
            </Link>
          )}
          {id === "commerce-store" && project?.prod_version_id && (
            <a
              href={prodOpenUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
            >
              Storefront UI
            </a>
          )}
        </div>
      </div>
    </div>
  );
}

function OverviewCard({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-md border border-border bg-card px-4 py-4">
      <p className="text-label-13 text-muted-foreground">{label}</p>
      <div className="mt-2 text-sm">{children}</div>
    </div>
  );
}

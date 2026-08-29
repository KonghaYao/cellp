import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ExternalLink, GitBranch, Globe } from "lucide-react";
import {
  getProject,
  listVersions,
  CellpApiError,
  type ProjectDetail,
  type Version,
} from "@/lib/cellp-api";
import { deriveProdUrl, formatRelativeTime } from "@/lib/format";
import { deploymentsHref, storageBrowserHref } from "@/lib/routes";
import { CopyButton } from "@/components/copy-button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";

export function ProjectOverviewPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [project, setProject] = useState<ProjectDetail | null>(null);
  const [prodVersion, setProdVersion] = useState<Version | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await getProject(id);
        let prod: Version | null = null;
        if (data.prod_version_id) {
          const page = await listVersions(id, { limit: 50 });
          prod = page.versions.find((v) => v.id === data.prod_version_id) ?? null;
        }
        if (cancelled) return;
        setProject(data);
        setProdVersion(prod);
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

  const prodUrl = deriveProdUrl(id, prodVersion?.preview_url);
  const deploymentCount = project?.version_count ?? 0;

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
          {project?.prod_version_id && (
            <Link
              to={storageBrowserHref(id, project.prod_version_id)}
              className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
            >
              Open storage
            </Link>
          )}
          <a
            href={prodUrl}
            target="_blank"
            rel="noreferrer"
            className="inline-flex h-8 items-center gap-2 rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
          >
            <Globe className="size-3.5" />
            Visit
            <ExternalLink className="size-3 text-muted-foreground" />
          </a>
        </div>
      </div>

      {loading ? (
        <div className="grid gap-4 sm:grid-cols-3">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-3">
          <OverviewCard label="Production deployment">
            {project?.prod_version_id ? (
              <div className="flex items-center gap-2">
                <span className="font-mono text-lg font-semibold">
                  {project.prod_version_id}
                </span>
                <Badge variant="prod">Production</Badge>
              </div>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </OverviewCard>
          <OverviewCard label="Deployments">
            <span className="text-2xl font-semibold tabular-nums">
              {deploymentCount}
            </span>
          </OverviewCard>
          <OverviewCard label="Created">
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

      <div className="rounded-md border border-border bg-card p-6">
        <h2 className="text-label-14 font-medium">Quick links</h2>
        <div className="mt-4 flex flex-wrap gap-2">
          <Link
            to={deploymentsHref(id)}
            className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
          >
            View deployments
          </Link>
          {project?.prod_version_id && (
            <Link
              to={storageBrowserHref(id, project.prod_version_id)}
              className="inline-flex h-8 items-center rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
            >
              Browse production database
            </Link>
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

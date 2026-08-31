import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  getProject,
  listVersions,
  CellpApiError,
  type ProjectDetail,
  type Version,
} from "@/lib/cellp-api";
import { resolveProdUrl } from "@/lib/format";
import { DeploymentsStatusSummary } from "@/components/deployments-status-summary";
import { DeploymentsTable } from "@/components/deployments-table";
import { DeploymentsFilterBar } from "@/components/deployments-filter-bar";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

export function DeploymentsPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const [project, setProject] = useState<ProjectDetail | null>(null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  const branchFilter = searchParams.get("branch") ?? "";
  const statusFilter = searchParams.get("status") ?? "";
  const hideDestroyed = searchParams.get("hide_destroyed") !== "0";

  const loadVersions = useCallback(
    async (cursor?: string | null) => {
      const isInitial = cursor == null;
      if (!isInitial) setLoadingMore(true);
      try {
        const page = await listVersions(id, { cursor });
        setVersions((prev) =>
          isInitial ? page.versions : [...prev, ...page.versions],
        );
        setNextCursor(page.next_cursor);
      } finally {
        if (!isInitial) setLoadingMore(false);
      }
    },
    [id],
  );

  const loadProject = useCallback(async () => {
    const data = await getProject(id);
    setProject(data);
    await loadVersions();
    setError(null);
    setNotFound(false);
  }, [id, loadVersions]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await getProject(id);
        if (cancelled) return;
        setProject(data);
        const page = await listVersions(id);
        if (cancelled) return;
        setVersions(page.versions);
        setNextCursor(page.next_cursor);
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
                : "Failed to load deployments",
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

  const filtered = useMemo(() => {
    return versions.filter((v) => {
      if (hideDestroyed && (v.status === "destroyed" || v.status === "draining")) {
        return false;
      }
      if (branchFilter && !v.git_ref.toLowerCase().includes(branchFilter.toLowerCase())) {
        return false;
      }
      if (statusFilter && v.status !== statusFilter) {
        return false;
      }
      return true;
    });
  }, [versions, branchFilter, statusFilter, hideDestroyed]);

  const branches = useMemo(
    () => [...new Set(versions.map((v) => v.git_ref).filter(Boolean))].sort(),
    [versions],
  );

  const hasActiveFilters =
    Boolean(branchFilter) || Boolean(statusFilter) || !hideDestroyed;

  function clearFilters() {
    setSearchParams(new URLSearchParams(), { replace: true });
  }

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

  const prodVersion = versions.find((v) => v.id === project?.prod_version_id);
  const prodUrl = resolveProdUrl(
    id,
    project?.prod_url,
    prodVersion?.preview_url,
  );

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-heading-24 font-semibold tracking-tight">Versions</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          Immutable deployment versions for{" "}
          <span className="font-mono">{id}</span> — not Git branch deployments.
          Each row is a <span className="font-mono">version ID</span> with its own
          preview URL; production is the promoted version only.
        </p>
      </div>

      {!loading && versions.length > 0 ? (
        <DeploymentsStatusSummary
          projectId={id}
          versions={versions.filter(
            (v) => v.status !== "destroyed" && v.status !== "draining",
          )}
          prodVersionId={project?.prod_version_id ?? null}
        />
      ) : null}

      <DeploymentsFilterBar
        branches={branches}
        branchFilter={branchFilter}
        statusFilter={statusFilter}
        hideDestroyed={hideDestroyed}
        onBranchChange={(value) => {
          const next = new URLSearchParams(searchParams);
          if (value) next.set("branch", value);
          else next.delete("branch");
          setSearchParams(next, { replace: true });
        }}
        onStatusChange={(value) => {
          const next = new URLSearchParams(searchParams);
          if (value) next.set("status", value);
          else next.delete("status");
          setSearchParams(next, { replace: true });
        }}
        onHideDestroyedChange={(value) => {
          const next = new URLSearchParams(searchParams);
          if (value) next.delete("hide_destroyed");
          else next.set("hide_destroyed", "0");
          setSearchParams(next, { replace: true });
        }}
      />

      {loading && <Skeleton className="h-64 w-full" />}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && !error && filtered.length === 0 && versions.length === 0 && (
        <EmptyState
          title="No deployments yet"
          description="Deploy a version with the cellp CLI or your CI pipeline to see it here."
        />
      )}

      {!loading && !error && filtered.length === 0 && versions.length > 0 && (
        <div className="space-y-4">
          <EmptyState
            title="No deployments match your filters"
            description="Try adjusting branch or status filters, or show destroyed deployments."
          />
          {hasActiveFilters && (
            <div className="flex justify-center">
              <Button variant="outline" size="sm" onClick={clearFilters}>
                Clear filters
              </Button>
            </div>
          )}
        </div>
      )}

      {!loading && !error && filtered.length > 0 && project && (
        <div className="space-y-3">
          <DeploymentsTable
            projectId={id}
            prodVersionId={project.prod_version_id}
            versions={filtered}
            prodUrl={prodUrl}
            onRefresh={loadProject}
            dense
          />
          {nextCursor && (
            <div className="flex justify-center pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => void loadVersions(nextCursor)}
                disabled={loadingMore}
              >
                {loadingMore ? "Loading…" : "Load more"}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

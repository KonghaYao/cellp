import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  getProject,
  getVersion,
  CellpApiError,
  type Version,
} from "@/lib/cellp-api";
import { Breadcrumbs } from "@/components/breadcrumbs";
import { VersionDetailView } from "@/components/version-detail-view";
import { Skeleton } from "@/components/ui/skeleton";

export function VersionPage() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const [version, setVersion] = useState<Version | null>(null);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  const loadData = useCallback(async () => {
    const [project, v] = await Promise.all([
      getProject(id),
      getVersion(id, vid),
    ]);
    setVersion(v);
    setProdVersionId(project.prod_version_id);
    setError(null);
    setNotFound(false);
  }, [id, vid]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const [project, v] = await Promise.all([
          getProject(id),
          getVersion(id, vid),
        ]);
        if (!cancelled) {
          setVersion(v);
          setProdVersionId(project.prod_version_id);
          setError(null);
          setNotFound(false);
        }
      } catch (e) {
        if (!cancelled) {
          if (e instanceof CellpApiError && e.status === 404) {
            setNotFound(true);
          } else {
            setError(
              e instanceof CellpApiError
                ? `${e.message} (${e.status})`
                : "Failed to load version",
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
  }, [id, vid]);

  const handleRefresh = useCallback(async () => {
    try {
      await loadData();
    } catch {
      /* keep current state on transient refresh errors */
    }
  }, [loadData]);

  if (notFound) {
    return (
      <div className="space-y-4 text-center py-16">
        <h1 className="text-2xl font-semibold">Version not found</h1>
        <p className="text-muted-foreground">
          <Link to={`/projects/${id}`} className="hover:underline">
            Back to project
          </Link>
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <Breadcrumbs
        items={[
          { label: "Projects", href: "/" },
          { label: id, href: `/projects/${id}` },
          { label: vid },
        ]}
      />

      {loading && (
        <div className="space-y-4">
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-48 w-full" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {version && (
        <VersionDetailView
          projectId={id}
          versionId={vid}
          initialVersion={version}
          prodVersionId={prodVersionId}
          onRefresh={handleRefresh}
        />
      )}
    </div>
  );
}

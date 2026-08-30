import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, BookOpen, Settings } from "lucide-react";
import {
  getBindings,
  getProject,
  hasAnyBindings,
  CellpApiError,
  type Bindings,
} from "@/lib/cellp-api";
import {
  projectOverviewHref,
  storageBrowserHref,
  storageHref,
  storageKvHref,
  storageQueuesHref,
  storageWorkflowsHref,
} from "@/lib/routes";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";

export function SettingsPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [bindings, setBindings] = useState<Bindings | null>(null);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const project = await getProject(id);
        if (!project.prod_version_id) {
          if (!cancelled) {
            setProdVersionId(null);
            setBindings(null);
            setError(null);
          }
          return;
        }
        const b = await getBindings(id, project.prod_version_id);
        if (!cancelled) {
          setProdVersionId(project.prod_version_id);
          setBindings(b);
          setError(null);
        }
      } catch (e) {
        if (!cancelled) {
          setBindings(null);
          setError(
            e instanceof CellpApiError ? e.message : "Failed to load project settings",
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

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <Link
          to={projectOverviewHref(id)}
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-3.5" />
          Back to overview
        </Link>
        <h1 className="text-heading-24 font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          Project configuration for <span className="font-mono">{id}</span>
        </p>
      </div>

      {loading && <Skeleton className="h-40 w-full" />}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && prodVersionId && bindings && hasAnyBindings(bindings) && (
        <div className="rounded-md border border-border bg-card p-6">
          <div className="flex items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-md border border-border bg-muted">
              <Settings className="size-5 text-muted-foreground" />
            </div>
            <div className="min-w-0 flex-1">
              <h2 className="text-label-14 font-medium">Production bindings</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Wrangler bindings for <span className="font-mono">{prodVersionId}</span>
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                {bindings.d1.length > 0 && (
                  <Link to={storageBrowserHref(id, prodVersionId)}>
                    <Badge variant="outline">D1 × {bindings.d1.length}</Badge>
                  </Link>
                )}
                {bindings.kv.length > 0 && (
                  <Link to={storageKvHref(id, prodVersionId)}>
                    <Badge variant="outline">KV × {bindings.kv.length}</Badge>
                  </Link>
                )}
                {bindings.queues.length > 0 && (
                  <Link to={storageQueuesHref(id, prodVersionId)}>
                    <Badge variant="outline">Queue × {bindings.queues.length}</Badge>
                  </Link>
                )}
                {bindings.workflows.length > 0 && (
                  <Link to={storageWorkflowsHref(id, prodVersionId)}>
                    <Badge variant="outline">Workflow × {bindings.workflows.length}</Badge>
                  </Link>
                )}
                {bindings.r2.length > 0 && (
                  <Badge variant="outline">R2 × {bindings.r2.length}</Badge>
                )}
                {bindings.crons.length > 0 && (
                  <Badge variant="outline">Cron × {bindings.crons.length}</Badge>
                )}
              </div>
              <p className="mt-4">
                <Link
                  to={storageHref(id)}
                  className="text-sm font-medium hover:underline"
                >
                  Open storage hub →
                </Link>
              </p>
            </div>
          </div>
        </div>
      )}

      {!loading && !prodVersionId && !error && (
        <div className="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">
          No production deployment yet. Promote a ready version to configure bindings.
        </div>
      )}

      <div className="rounded-md border border-dashed border-border bg-card/50 p-6">
        <h2 className="text-label-14 font-medium">Coming soon</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Environment variables, custom domains, and Git integration are planned for a
          future release.
        </p>
      </div>

      <p className="text-sm text-muted-foreground">
        Need help today?{" "}
        <a
          href="https://github.com/KonghaYao/cellp#readme"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 font-medium text-foreground hover:underline"
        >
          <BookOpen className="size-3.5" />
          Read the cellp docs
        </a>
      </p>
    </div>
  );
}

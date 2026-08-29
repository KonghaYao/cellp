import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Layers } from "lucide-react";
import {
  listProjects,
  CellpApiError,
  type ProjectSummary,
} from "@/lib/cellp-api";
import { formatRelativeTime } from "@/lib/format";
import { Breadcrumbs } from "@/components/breadcrumbs";
import { EmptyState } from "@/components/empty-state";
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
import { Skeleton } from "@/components/ui/skeleton";

export function ProjectsPage() {
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadPage = useCallback(async (cursor?: string | null) => {
    const isInitial = cursor == null;
    if (isInitial) {
      setLoading(true);
    } else {
      setLoadingMore(true);
    }
    try {
      const page = await listProjects({ cursor });
      setProjects((prev) =>
        isInitial ? page.projects : [...prev, ...page.projects],
      );
      setNextCursor(page.next_cursor);
      setError(null);
    } catch (e) {
      setError(
        e instanceof CellpApiError
          ? `${e.message} (${e.status})`
          : "Failed to load projects",
      );
    } finally {
      if (isInitial) setLoading(false);
      else setLoadingMore(false);
    }
  }, []);

  useEffect(() => {
    void loadPage();
  }, [loadPage]);

  return (
    <div className="space-y-8">
      <div>
        <Breadcrumbs items={[{ label: "Projects" }]} className="mb-4" />
        <h1 className="text-2xl font-semibold tracking-tight">Projects</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage deployments across your cellp projects.
        </p>
      </div>

      {loading && (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && !error && projects.length === 0 && (
        <EmptyState
          title="No projects yet"
          description="Projects appear here once you deploy via the cellp API. Push an artifact and POST /v1/projects/{id}/versions to get started."
          icon={<Layers className="size-6 text-muted-foreground" />}
        />
      )}

      {!loading && !error && projects.length > 0 && (
        <div className="space-y-4">
          <div className="overflow-hidden rounded-lg border border-border bg-card">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>Project</TableHead>
                  <TableHead>Deployments</TableHead>
                  <TableHead>Production</TableHead>
                  <TableHead>Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {projects.map((project) => (
                  <TableRow key={project.id}>
                    <TableCell>
                      <Link
                        to={`/projects/${project.id}`}
                        className="font-medium hover:underline"
                      >
                        {project.id}
                      </Link>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {project.version_count}{" "}
                      {project.version_count === 1
                        ? "deployment"
                        : "deployments"}
                    </TableCell>
                    <TableCell>
                      {project.prod_version_id ? (
                        <Badge variant="prod">{project.prod_version_id}</Badge>
                      ) : (
                        <span className="text-sm text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatRelativeTime(project.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {nextCursor && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                onClick={() => void loadPage(nextCursor)}
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

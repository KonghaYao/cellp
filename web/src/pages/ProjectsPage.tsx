import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Layers, Search } from "lucide-react";
import {
  listProjects,
  CellpApiError,
  projectMatchesQuery,
  type ProjectSummary,
} from "@/lib/cellp-api";
import { formatRelativeTime } from "@/lib/format";
import { CreateProjectDialog } from "@/components/create-project-dialog";
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

type SortKey = "name" | "created" | "has_production";

function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}

function sortProjects(
  projects: ProjectSummary[],
  sortKey: SortKey,
): ProjectSummary[] {
  const sorted = [...projects];
  switch (sortKey) {
    case "name":
      sorted.sort((a, b) => a.id.localeCompare(b.id));
      break;
    case "created":
      sorted.sort((a, b) => {
        const aTime = a.created_at ? new Date(a.created_at).getTime() : 0;
        const bTime = b.created_at ? new Date(b.created_at).getTime() : 0;
        if (Number.isNaN(aTime) && Number.isNaN(bTime)) return 0;
        if (Number.isNaN(aTime)) return 1;
        if (Number.isNaN(bTime)) return -1;
        return bTime - aTime;
      });
      break;
    case "has_production":
      sorted.sort((a, b) => {
        const aProd = a.prod_version_id ? 1 : 0;
        const bProd = b.prod_version_id ? 1 : 0;
        if (aProd !== bProd) return bProd - aProd;
        return a.id.localeCompare(b.id);
      });
      break;
  }
  return sorted;
}

export function ProjectsPage() {
  const [projects, setProjects] = useState<ProjectSummary[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("has_production");
  const debouncedSearch = useDebouncedValue(search, 300);
  const searchQuery = debouncedSearch.trim();

  const loadPage = useCallback(
    async (cursor?: string | null, q?: string) => {
      const isInitial = cursor == null;
      if (isInitial) {
        setLoading(true);
      } else {
        setLoadingMore(true);
      }
      try {
        const page = await listProjects({
          cursor,
          q: q || undefined,
        });
        setProjects((prev) =>
          isInitial ? page.projects : [...prev, ...page.projects],
        );
        setNextCursor(page.next_cursor);
        setError(null);
        return page;
      } catch (e) {
        setError(
          e instanceof CellpApiError
            ? `${e.message} (${e.status})`
            : "Failed to load projects",
        );
        return null;
      } finally {
        if (isInitial) setLoading(false);
        else setLoadingMore(false);
      }
    },
    [],
  );

  useEffect(() => {
    void loadPage(null, searchQuery);
  }, [loadPage, searchQuery]);

  const filtered = useMemo(() => {
    if (!searchQuery) return projects;
    return projects.filter((p) => projectMatchesQuery(p, searchQuery));
  }, [projects, searchQuery]);

  const sorted = useMemo(
    () => sortProjects(filtered, sortKey),
    [filtered, sortKey],
  );

  const searchBackendIgnored = useMemo(() => {
    if (!searchQuery || projects.length === 0) return false;
    return projects.some((p) => !projectMatchesQuery(p, searchQuery));
  }, [projects, searchQuery]);

  const searchExhausted =
    searchQuery && sorted.length === 0 && !nextCursor && !loading && !loadingMore;

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Breadcrumbs items={[{ label: "Projects" }]} className="mb-4" />
          <h1 className="text-2xl font-semibold tracking-tight">Projects</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage deployments across your cellp projects.
          </p>
        </div>
        <CreateProjectDialog onCreated={() => loadPage(null, searchQuery)} />
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

      {!loading && !error && projects.length === 0 && !searchQuery && (
        <EmptyState
          title="No projects yet"
          description="Projects show up here after your first deploy. Use the cellp CLI or your CI pipeline to push an artifact and create a deployment."
          icon={<Layers className="size-6 text-muted-foreground" />}
        />
      )}

      {!loading && !error && (projects.length > 0 || searchQuery) && (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-card p-2">
            <label className="relative flex min-w-[12rem] flex-1 items-center">
              <Search className="pointer-events-none absolute left-2.5 size-3.5 text-muted-foreground" />
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search projects…"
                className="h-8 w-full rounded-md border border-border bg-background pl-8 pr-3 text-sm"
              />
            </label>
            <label className="flex items-center gap-2 text-label-13 text-muted-foreground">
              <span className="hidden sm:inline">Sort by</span>
              <select
                value={sortKey}
                onChange={(e) => setSortKey(e.target.value as SortKey)}
                className="h-8 rounded-md border border-border bg-background px-2 text-sm"
              >
                <option value="has_production">Has production</option>
                <option value="name">Name</option>
                <option value="created">Created</option>
              </select>
            </label>
          </div>

          {projects.length > 0 && (
            <p className="text-sm text-muted-foreground">
              {searchQuery ? (
                searchBackendIgnored ? (
                  <>
                    Showing {sorted.length} of {projects.length} loaded matching
                    &ldquo;{searchQuery}&rdquo; (filtered locally — restart cellpd
                    for server search)
                  </>
                ) : (
                  <>
                    Showing {sorted.length} matching &ldquo;{searchQuery}&rdquo;
                  </>
                )
              ) : (
                <>Showing {projects.length} loaded</>
              )}
              {nextCursor ? " — more available" : ""}
            </p>
          )}

          {loadingMore && (
            <p className="text-center text-sm text-muted-foreground">
              Loading more projects…
            </p>
          )}

          {sorted.length === 0 ? (
            <div className="space-y-3 rounded-lg border border-border bg-card px-4 py-10 text-center text-sm text-muted-foreground">
              {searchExhausted ? (
                <p>No projects match &ldquo;{searchQuery}&rdquo;</p>
              ) : loading ? (
                <p>Searching…</p>
              ) : (
                <p>No projects match &ldquo;{searchQuery}&rdquo;</p>
              )}
            </div>
          ) : (
            <div className="overflow-hidden rounded-lg border border-border bg-card">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>Project</TableHead>
                    <TableHead>Deployments</TableHead>
                    <TableHead>Production</TableHead>
                    <TableHead>Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sorted.map((project) => (
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
          )}

          {nextCursor && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                onClick={() => void loadPage(nextCursor, searchQuery)}
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

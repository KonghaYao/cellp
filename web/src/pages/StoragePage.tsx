import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Database, ExternalLink } from "lucide-react";
import {
  getProject,
  listVersions,
  CellpApiError,
  type Version,
} from "@/lib/cellp-api";
import { storageBrowserHref } from "@/lib/routes";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusIndicator } from "@/components/status-indicator";

export function StoragePage() {
  const { id = "" } = useParams<{ id: string }>();
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const project = await getProject(id);
        const all: Version[] = [];
        let cursor: string | null = null;
        do {
          const page = await listVersions(id, { cursor });
          all.push(...page.versions);
          cursor = page.next_cursor;
        } while (cursor);
        if (cancelled) return;
        setProdVersionId(project.prod_version_id);
        setVersions(all.filter((v) => v.status === "ready"));
        setError(null);
      } catch (e) {
        if (!cancelled) {
          setError(
            e instanceof CellpApiError
              ? e.message
              : "Failed to load storage",
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
    <div className="space-y-6">
      <div>
        <h1 className="text-heading-24 font-semibold tracking-tight">Storage</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          D1 databases attached to deployments in{" "}
          <span className="font-mono">{id}</span>
        </p>
      </div>

      {loading && <Skeleton className="h-48 w-full" />}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!loading && !error && (
        <div className="overflow-hidden rounded-md border border-border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="text-label-12">Database</TableHead>
                <TableHead className="text-label-12">Deployment</TableHead>
                <TableHead className="text-label-12">Branch</TableHead>
                <TableHead className="text-label-12">Status</TableHead>
                <TableHead className="text-right text-label-12">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {versions.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                    No ready deployments with databases
                  </TableCell>
                </TableRow>
              ) : (
                versions.map((version) => {
                  const isProd = prodVersionId === version.id;
                  return (
                    <TableRow key={version.id} className="hover:bg-muted/40">
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Database className="size-4 text-muted-foreground" />
                          <span className="font-mono text-sm">main</span>
                        </div>
                        <p className="mt-0.5 font-mono text-xs text-muted-foreground">
                          {version.data_branch || `${id}/${version.id}`}
                        </p>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-sm">{version.id}</span>
                          {isProd && <Badge variant="prod">Production</Badge>}
                        </div>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {version.git_ref || "—"}
                      </TableCell>
                      <TableCell>
                        <StatusIndicator status={version.status} />
                      </TableCell>
                      <TableCell className="text-right">
                        <Link
                          to={storageBrowserHref(id, version.id)}
                          className="inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
                        >
                          Open browser
                          <ExternalLink className="size-3" />
                        </Link>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}

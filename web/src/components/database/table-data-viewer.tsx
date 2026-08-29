import { useCallback, useEffect, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import {
  getDatabaseTableRows,
  CellpApiError,
  LIST_PAGE_SIZE,
} from "@/lib/cellp-api";
import { QueryResults } from "@/components/database/query-results";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface TableDataViewerProps {
  projectId: string;
  versionId: string;
  tableName: string | null;
  pageSize?: number;
  className?: string;
}

export function TableDataViewer({
  projectId,
  versionId,
  tableName,
  pageSize = LIST_PAGE_SIZE,
  className,
}: TableDataViewerProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [columns, setColumns] = useState<
    Awaited<ReturnType<typeof getDatabaseTableRows>>["columns"]
  >([]);
  const [rows, setRows] = useState<
    Awaited<ReturnType<typeof getDatabaseTableRows>>["rows"]
  >([]);

  const loadRows = useCallback(
    async (nextOffset: number) => {
      if (!tableName) return;
      setLoading(true);
      try {
        const data = await getDatabaseTableRows(
          projectId,
          versionId,
          tableName,
          { limit: pageSize, offset: nextOffset },
        );
        setColumns(data.columns);
        setRows(data.rows);
        setTotal(data.total);
        setOffset(data.offset);
        setError(null);
      } catch (e) {
        setError(
          e instanceof CellpApiError
            ? e.message
            : "Failed to load table rows",
        );
      } finally {
        setLoading(false);
      }
    },
    [pageSize, projectId, tableName, versionId],
  );

  useEffect(() => {
    setOffset(0);
    setTotal(0);
    setColumns([]);
    setRows([]);
    setError(null);
    if (tableName) {
      void loadRows(0);
    }
  }, [tableName, loadRows]);

  if (!tableName) {
    return (
      <Card className={className}>
        <CardContent className="py-12 text-center text-sm text-muted-foreground">
          Select a table to view its data
        </CardContent>
      </Card>
    );
  }

  const start = total === 0 ? 0 : offset + 1;
  const end = Math.min(offset + rows.length, total);
  const hasPrevious = offset > 0;
  const hasNext = offset + rows.length < total;

  return (
    <Card className={cn("overflow-hidden", className)}>
      <CardHeader className="flex flex-row items-center justify-between gap-4 space-y-0 pb-3">
        <div>
          <CardTitle className="font-mono text-base">{tableName}</CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            {loading ? "Loading rows…" : `${total.toLocaleString()} rows total`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={!hasPrevious || loading}
            onClick={() => void loadRows(Math.max(0, offset - pageSize))}
          >
            <ChevronLeft className="size-4" />
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasNext || loading}
            onClick={() => void loadRows(offset + pageSize)}
          >
            Next
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && (
          <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}

        {loading && rows.length === 0 ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-48 w-full" />
          </div>
        ) : (
          <QueryResults
            columns={columns}
            rows={rows}
            emptyMessage="This table has no rows"
          />
        )}

        {!loading && total > 0 && (
          <p className="text-xs text-muted-foreground">
            Showing {start.toLocaleString()}–{end.toLocaleString()} of{" "}
            {total.toLocaleString()}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

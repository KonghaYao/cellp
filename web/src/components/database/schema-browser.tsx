import { useEffect, useState } from "react";
import { Table2 } from "lucide-react";
import {
  listDatabaseTables,
  CellpApiError,
  type DatabaseTable,
} from "@/lib/cellp-api";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface SchemaBrowserProps {
  projectId: string;
  versionId: string;
  selectedTable: string | null;
  onSelectTable: (tableName: string) => void;
  className?: string;
}

function formatRowCount(count: number): string {
  return count.toLocaleString();
}

export function SchemaBrowser({
  projectId,
  versionId,
  selectedTable,
  onSelectTable,
  className,
}: SchemaBrowserProps) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tables, setTables] = useState<DatabaseTable[]>([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await listDatabaseTables(projectId, versionId);
        if (cancelled) return;
        setTables(data.tables);
        setError(null);
      } catch (e) {
        if (!cancelled) {
          setError(
            e instanceof CellpApiError
              ? e.message
              : "Failed to load tables",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, versionId]);

  if (loading) {
    return (
      <div className={cn("space-y-2", className)}>
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div
        className={cn(
          "rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive",
          className,
        )}
      >
        {error}
      </div>
    );
  }

  return (
    <nav
      aria-label="Database tables"
      className={cn(
        "rounded-lg border border-border bg-card",
        className,
      )}
    >
      <div className="border-b border-border px-3 py-2">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Tables
        </p>
        <p className="text-xs text-muted-foreground">{tables.length} total</p>
      </div>
      {tables.length === 0 ? (
        <p className="px-3 py-6 text-center text-sm text-muted-foreground">
          No tables found
        </p>
      ) : (
        <ul className="max-h-[28rem] overflow-y-auto p-1">
          {tables.map((table) => {
            const isSelected = selectedTable === table.name;
            return (
              <li key={table.name}>
                <button
                  type="button"
                  onClick={() => onSelectTable(table.name)}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors",
                    isSelected
                      ? "bg-accent text-accent-foreground"
                      : "hover:bg-muted/60",
                  )}
                >
                  <Table2 className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate font-mono">
                    {table.name}
                  </span>
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                    {formatRowCount(table.row_count)}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </nav>
  );
}

import type { DatabaseColumn } from "@/lib/cellp-api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

interface QueryResultsProps {
  columns: DatabaseColumn[];
  rows: Record<string, unknown>[];
  emptyMessage?: string;
  className?: string;
}

function formatCellValue(value: unknown): string {
  if (value == null) return "NULL";
  if (typeof value === "object") {
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  return String(value);
}

export function QueryResults({
  columns,
  rows,
  emptyMessage = "No rows returned",
  className,
}: QueryResultsProps) {
  if (columns.length === 0) {
    return (
      <div
        className={cn(
          "rounded-lg border border-dashed border-border bg-card px-4 py-10 text-center text-sm text-muted-foreground",
          className,
        )}
      >
        {emptyMessage}
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div
        className={cn(
          "rounded-lg border border-dashed border-border bg-card px-4 py-10 text-center text-sm text-muted-foreground",
          className,
        )}
      >
        {emptyMessage}
      </div>
    );
  }

  return (
    <div
      className={cn(
        "overflow-hidden rounded-lg border border-border bg-card",
        className,
      )}
    >
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {columns.map((column) => (
              <TableHead key={column.name} className="whitespace-nowrap">
                <div>{column.name}</div>
                <div className="mt-0.5 font-normal normal-case tracking-normal text-muted-foreground/80">
                  {column.type}
                </div>
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row, rowIndex) => (
            <TableRow key={rowIndex}>
              {columns.map((column) => {
                const value = row[column.name];
                const display = formatCellValue(value);
                return (
                  <TableCell
                    key={column.name}
                    className="max-w-xs truncate font-mono text-xs"
                    title={display}
                  >
                    {display}
                  </TableCell>
                );
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

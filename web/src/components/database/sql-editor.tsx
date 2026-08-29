import { useState, useTransition } from "react";
import { Play } from "lucide-react";
import {
  queryDatabase,
  CellpApiError,
  type DatabaseQueryResponse,
} from "@/lib/cellp-api";
import { QueryResults } from "@/components/database/query-results";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface SqlEditorProps {
  projectId: string;
  versionId: string;
  initialSql?: string;
  className?: string;
}

const DEFAULT_SQL = "SELECT name FROM sqlite_master WHERE type = 'table';";

export function SqlEditor({
  projectId,
  versionId,
  initialSql = DEFAULT_SQL,
  className,
}: SqlEditorProps) {
  const [sql, setSql] = useState(initialSql);
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<DatabaseQueryResponse | null>(null);

  function handleRun() {
    const trimmed = sql.trim();
    if (!trimmed) {
      setError("Enter a SQL statement to run");
      return;
    }

    setError(null);
    startTransition(async () => {
      try {
        const response = await queryDatabase(projectId, versionId, trimmed);
        setResult(response);
      } catch (e) {
        setResult(null);
        setError(
          e instanceof CellpApiError ? e.message : "Query failed",
        );
      }
    });
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      handleRun();
    }
  }

  return (
    <Card className={cn("overflow-hidden", className)}>
      <CardHeader className="flex flex-row items-center justify-between gap-4 space-y-0 pb-3">
        <div>
          <CardTitle className="text-base">SQL editor</CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            Run read/write queries against this version&apos;s database
          </p>
        </div>
        <Button onClick={handleRun} disabled={pending} size="sm">
          <Play className="size-3.5" />
          {pending ? "Running…" : "Run"}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <textarea
          value={sql}
          onChange={(event) => setSql(event.target.value)}
          onKeyDown={handleKeyDown}
          spellCheck={false}
          rows={8}
          className="w-full resize-y rounded-lg border border-border bg-background px-3 py-2 font-mono text-sm leading-relaxed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          placeholder="SELECT * FROM ..."
          aria-label="SQL query"
        />
        <p className="text-xs text-muted-foreground">
          Press ⌘/Ctrl + Enter to run
        </p>

        {error && (
          <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 font-mono text-sm text-destructive">
            {error}
          </div>
        )}

        {result && (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
              <span>
                {result.rows.length.toLocaleString()} row
                {result.rows.length === 1 ? "" : "s"}
              </span>
              <span aria-hidden>·</span>
              <span>{result.duration_ms.toLocaleString()} ms</span>
              {result.rows_affected != null && (
                <>
                  <span aria-hidden>·</span>
                  <span>
                    {result.rows_affected.toLocaleString()} rows affected
                  </span>
                </>
              )}
            </div>
            <QueryResults
              columns={result.columns}
              rows={result.rows}
              emptyMessage="Query completed with no rows"
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

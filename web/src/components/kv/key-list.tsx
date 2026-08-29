import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import {
  CellpApiError,
  LIST_PAGE_SIZE,
  listKvKeys,
  type KvKey,
} from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

const fieldClass =
  "h-8 rounded-md border border-border bg-background px-3 font-mono text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

interface KeyListProps {
  projectId: string;
  versionId: string;
  ns: string;
  selectedKey: string | null;
  onSelectKey: (key: string) => void;
  refreshToken: number;
  pageSize?: number;
}

export function KeyList({
  projectId,
  versionId,
  ns,
  selectedKey,
  onSelectKey,
  refreshToken,
  pageSize = LIST_PAGE_SIZE,
}: KeyListProps) {
  const [prefixInput, setPrefixInput] = useState("");
  const [prefix, setPrefix] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [keys, setKeys] = useState<KvKey[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [history, setHistory] = useState<(string | undefined)[]>([]);
  const loadGen = useRef(0);

  const loadPage = useCallback(
    async (pageCursor: string | undefined) => {
      const gen = ++loadGen.current;
      setLoading(true);
      try {
        const data = await listKvKeys(projectId, versionId, ns, {
          prefix: prefix || undefined,
          cursor: pageCursor,
          limit: pageSize,
        });
        if (gen !== loadGen.current) return;
        setKeys(data.keys);
        setNextCursor(data.cursor);
        setCursor(pageCursor);
        setError(null);
      } catch (e) {
        if (gen !== loadGen.current) return;
        setKeys([]);
        setNextCursor(undefined);
        setError(
          e instanceof CellpApiError ? e.message : "Failed to list keys",
        );
      } finally {
        if (gen === loadGen.current) setLoading(false);
      }
    },
    [ns, pageSize, prefix, projectId, versionId],
  );

  useEffect(() => {
    setHistory([]);
    void loadPage(undefined);
  }, [loadPage, refreshToken]);

  function applyPrefix(event: React.FormEvent) {
    event.preventDefault();
    const next = prefixInput.trim();
    if (next === prefix) {
      setHistory([]);
      void loadPage(undefined);
      return;
    }
    setPrefix(next);
  }

  function goPrevious() {
    const previous = history[history.length - 1];
    setHistory((h) => h.slice(0, -1));
    void loadPage(previous);
  }

  function goNext() {
    if (!nextCursor) return;
    setHistory((h) => [...h, cursor]);
    void loadPage(nextCursor);
  }

  const hasPrevious = history.length > 0;
  const hasNext = Boolean(nextCursor);
  const emptyMessage = prefix
    ? "No keys match this prefix"
    : "This namespace has no keys";

  return (
    <Card className="overflow-hidden" data-testid="kv-key-list">
      <CardHeader className="flex flex-col gap-3 space-y-0 pb-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle className="text-base">Keys</CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            {loading
              ? "Loading keys…"
              : `${keys.length.toLocaleString()} on this page`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!hasPrevious || loading}
            onClick={goPrevious}
          >
            <ChevronLeft className="size-4" />
            Previous
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!hasNext || loading}
            onClick={goNext}
          >
            Next
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <form
          onSubmit={applyPrefix}
          className="flex flex-wrap items-end gap-2"
        >
          <label className="min-w-[12rem] flex-1 space-y-1 text-sm">
            <span className="text-muted-foreground">Prefix</span>
            <input
              type="text"
              value={prefixInput}
              onChange={(e) => setPrefixInput(e.target.value)}
              placeholder="Filter by prefix"
              aria-label="Prefix"
              className={cn("w-full", fieldClass)}
            />
          </label>
          <Button type="submit" variant="outline" size="sm" disabled={loading}>
            Filter
          </Button>
        </form>

        {error && (
          <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}

        {loading && keys.length === 0 ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        ) : keys.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
            {emptyMessage}
          </div>
        ) : (
          <div className="overflow-hidden rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>Key</TableHead>
                  <TableHead className="w-28">Expiration</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((key) => {
                  const isSelected = selectedKey === key.name;
                  return (
                    <TableRow
                      key={key.name}
                      className={cn(isSelected && "bg-accent/50")}
                    >
                      <TableCell className="font-mono text-xs">
                        <button
                          type="button"
                          onClick={() => onSelectKey(key.name)}
                          className="text-left hover:underline"
                        >
                          {key.name}
                        </button>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {key.expiration ?? "—"}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

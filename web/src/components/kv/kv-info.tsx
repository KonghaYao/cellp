import { useEffect, useState } from "react";
import { CellpApiError, getKvInfo, type KvInfo } from "@/lib/cellp-api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

interface KvInfoCardProps {
  projectId: string;
  versionId: string;
  ns: string;
  refreshToken: number;
}

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function KvInfoCard({
  projectId,
  versionId,
  ns,
  refreshToken,
}: KvInfoCardProps) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<KvInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await getKvInfo(projectId, versionId, ns);
        if (cancelled) return;
        setInfo(data);
        setError(null);
      } catch (e) {
        if (!cancelled) {
          setInfo(null);
          setError(
            e instanceof CellpApiError ? e.message : "Failed to load KV info",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, versionId, ns, refreshToken]);

  const live = info?.live ?? info?.keys;

  return (
    <Card data-testid="kv-info">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Namespace info</CardTitle>
      </CardHeader>
      <CardContent>
        {loading && <Skeleton className="h-12 w-full" />}
        {error && (
          <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}
        {!loading && !error && info && (
          <dl className="grid gap-3 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-muted-foreground">Live</dt>
              <dd className="mt-0.5 font-mono tabular-nums">
                {live?.toLocaleString() ?? "—"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Bytes</dt>
              <dd className="mt-0.5 font-mono tabular-nums">
                {formatBytes(info.bytes)}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Stored</dt>
              <dd className="mt-0.5 font-mono tabular-nums">
                {info.stored.toLocaleString()}
              </dd>
            </div>
          </dl>
        )}
      </CardContent>
    </Card>
  );
}

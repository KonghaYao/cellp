import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import {
  getTelemetryContext,
  getTelemetryTrace,
  searchTelemetry,
  type TelemetryContext,
  type TelemetrySearchHit,
  type TelemetryTraceResponse,
  CellpApiError,
} from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

function TraceTree({ node, depth = 0 }: { node: TelemetryTraceResponse["tree"]; depth?: number }) {
  return (
    <div className="font-mono text-sm" style={{ marginLeft: depth * 16 }}>
      <div className="py-1">
        <span className="text-muted-foreground">{node.start_time}</span>{" "}
        <span className="font-semibold">{node.name}</span>{" "}
        {node["http.method"] ? <span>{node["http.method"]} </span> : null}
        {node.url ? <span className="text-muted-foreground">{node.url} </span> : null}
        {node["http.status_code"] ? <span>· {node["http.status_code"]} </span> : null}
        <span>· {node.duration_ms.toFixed(1)}ms</span>
      </div>
      {(node.children ?? []).map((child) => (
        <TraceTree key={child.span_id} node={child} depth={depth + 1} />
      ))}
    </div>
  );
}

export function TelemetryPage() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const [ctx, setCtx] = useState<TelemetryContext | null>(null);
  const [hits, setHits] = useState<TelemetrySearchHit[]>([]);
  const [trace, setTrace] = useState<TelemetryTraceResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [traceIdInput, setTraceIdInput] = useState("");

  const windowRange = useMemo(() => {
    const end = new Date();
    const start = new Date(end.getTime() - 60 * 60 * 1000);
    return { start: start.toISOString(), end: end.toISOString() };
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const context = await getTelemetryContext(id, vid);
      setCtx(context);
      if (context.enabled) {
        const rows = await searchTelemetry(id, vid, {
          template: "slow",
          start: windowRange.start,
          end: windowRange.end,
          limit: 50,
          slow_ms: 100,
        });
        setHits(rows);
      } else {
        setHits([]);
      }
    } catch (e) {
      setError(e instanceof CellpApiError ? e.message : "Failed to load telemetry");
    } finally {
      setLoading(false);
    }
  }, [id, vid, windowRange.end, windowRange.start]);

  useEffect(() => {
    void load();
  }, [load]);

  const loadTrace = async () => {
    if (!traceIdInput.trim()) return;
    try {
      const tr = await getTelemetryTrace(id, vid, traceIdInput.trim());
      setTrace(tr);
    } catch (e) {
      setError(e instanceof CellpApiError ? e.message : "Trace lookup failed");
    }
  };

  if (loading) {
    return <Skeleton className="h-48 w-full" />;
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-heading-20 font-semibold">Investigate</h2>
        <p className="text-copy-14 text-muted-foreground">
          Version-scoped traces via cellpd query facade (AD-14). No direct Tempo/Loki access.
        </p>
      </div>

      {error ? <p className="text-copy-14 text-destructive">{error}</p> : null}

      <Card className="p-4 space-y-2">
        <p className="text-label-13 text-muted-foreground">Context</p>
        {ctx ? (
          <dl className="grid gap-1 text-copy-14 sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">Enabled</dt>
              <dd>{ctx.enabled ? "yes" : "no"}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Backend</dt>
              <dd className="font-mono">{ctx.backend}</dd>
            </div>
            {ctx.deep_link ? (
              <div className="sm:col-span-2">
                <dt className="text-muted-foreground">Grafana</dt>
                <dd>
                  <a
                    href={ctx.deep_link}
                    target="_blank"
                    rel="noreferrer"
                    className="text-primary underline"
                  >
                    Open in Grafana Explore
                  </a>
                </dd>
              </div>
            ) : null}
          </dl>
        ) : null}
        <Button type="button" variant="secondary" size="sm" onClick={() => void load()}>
          Refresh
        </Button>
      </Card>

      <Card className="p-4 space-y-3">
        <p className="text-label-13 text-muted-foreground">Slow requests (last hour)</p>
        {!ctx?.enabled ? (
          <p className="text-copy-14 text-muted-foreground">
            Telemetry disabled. Set CELLP_OTEL_BACKEND=memory or run ./dev/scripts/up.sh --profile otel
          </p>
        ) : hits.length === 0 ? (
          <p className="text-copy-14 text-muted-foreground">No slow spans in the selected window.</p>
        ) : (
          <ul className="divide-y text-copy-14">
            {hits.map((h) => (
              <li key={`${h.trace_id}-${h.span_id ?? h.start_time}`} className="py-2 flex flex-wrap gap-2">
                <button
                  type="button"
                  className="font-mono text-primary underline"
                  onClick={() => {
                    setTraceIdInput(h.trace_id);
                    void getTelemetryTrace(id, vid, h.trace_id).then(setTrace).catch(() => undefined);
                  }}
                >
                  {h.trace_id.slice(0, 16)}…
                </button>
                <span>{h.name}</span>
                <span className="text-muted-foreground">{h.duration_ms.toFixed(1)}ms</span>
                {h.url ? <span className="text-muted-foreground truncate">{h.url}</span> : null}
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card className="p-4 space-y-3">
        <p className="text-label-13 text-muted-foreground">Trace lookup</p>
        <div className="flex flex-wrap gap-2">
          <input
            className="flex-1 min-w-[240px] rounded-md border bg-background px-3 py-2 font-mono text-sm"
            placeholder="trace_id"
            value={traceIdInput}
            onChange={(e) => setTraceIdInput(e.target.value)}
          />
          <Button type="button" onClick={() => void loadTrace()}>
            Load trace
          </Button>
        </div>
        {trace ? (
          <div className="space-y-3 pt-2">
            <TraceTree node={trace.tree} />
            {trace.logs.length > 0 ? (
              <div>
                <p className="text-label-13 text-muted-foreground mb-2">Correlated logs</p>
                <ul className="font-mono text-xs space-y-1">
                  {trace.logs.map((l, i) => (
                    <li key={`${l.time}-${i}`}>{l.body}</li>
                  ))}
                </ul>
              </div>
            ) : null}
          </div>
        ) : null}
      </Card>
    </div>
  );
}

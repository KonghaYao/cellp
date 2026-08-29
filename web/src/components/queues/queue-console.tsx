import { useCallback, useEffect, useState } from "react";
import { Eye, Pause, Play, RotateCcw, Trash2 } from "lucide-react";
import {
  CellpApiError,
  getQueue,
  pauseQueue,
  peekQueue,
  purgeQueue,
  redriveQueue,
  resumeQueue,
  type BindingsQueue,
  type QueueInfo,
  type QueueListItem,
  type QueuePeekMessage,
} from "@/lib/cellp-api";
import { PeekMessages } from "@/components/queues/peek-messages";
import { PurgeQueueDialog } from "@/components/queues/purge-queue-dialog";
import { Badge } from "@/components/ui/badge";
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

export const CONSUMER_FETCH_NOTE =
  "A consumer script cannot also export fetch(). Pull consumers are not available.";

export type QueueRow = {
  name: string;
  binding?: string;
  consumer: boolean;
  deadLetterQueue?: string;
};

export function mergeQueueRows(
  listed: QueueListItem[],
  bindings: BindingsQueue[],
): QueueRow[] {
  const byName = new Map<string, QueueRow>();
  for (const q of listed) {
    byName.set(q.name, { name: q.name, consumer: false });
  }
  for (const q of bindings) {
    const existing = byName.get(q.name);
    if (existing) {
      existing.binding = q.binding ?? existing.binding;
      existing.consumer = q.consumer;
      existing.deadLetterQueue = q.dead_letter_queue ?? existing.deadLetterQueue;
    } else {
      byName.set(q.name, {
        name: q.name,
        binding: q.binding,
        consumer: q.consumer,
        deadLetterQueue: q.dead_letter_queue,
      });
    }
  }
  return [...byName.values()];
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function asBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function infoNumber(info: QueueInfo, keys: string[]): number | undefined {
  for (const key of keys) {
    const n = asNumber(info[key]);
    if (n != null) return n;
  }
  return undefined;
}

function redrivenCount(result: unknown): number | null {
  if (typeof result === "object" && result != null && "redriven" in result) {
    const n = (result as { redriven: unknown }).redriven;
    if (typeof n === "number" && Number.isFinite(n)) return n;
  }
  return null;
}

interface QueueConsoleProps {
  projectId: string;
  versionId: string;
  queues: QueueRow[];
}

export function QueueConsole({
  projectId,
  versionId,
  queues,
}: QueueConsoleProps) {
  const [selectedName, setSelectedName] = useState(queues[0]?.name ?? "");
  const [info, setInfo] = useState<QueueInfo | null>(null);
  const [messages, setMessages] = useState<QueuePeekMessage[]>([]);
  const [detailLoading, setDetailLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [purgeOpen, setPurgeOpen] = useState(false);
  const [peekLimit, setPeekLimit] = useState(10);

  const selected = queues.find((q) => q.name === selectedName) ?? queues[0];

  const firstName = queues[0]?.name ?? "";

  useEffect(() => {
    setSelectedName(firstName);
  }, [projectId, versionId, firstName]);

  const loadDetail = useCallback(
    async (name: string, limit: number) => {
      setDetailLoading(true);
      setError(null);
      try {
        const clamped = Math.min(100, Math.max(1, limit));
        const [queueInfo, peek] = await Promise.all([
          getQueue(projectId, versionId, name),
          peekQueue(projectId, versionId, name, clamped),
        ]);
        setInfo(queueInfo);
        setMessages(peek.messages ?? []);
      } catch (e) {
        setInfo(null);
        setMessages([]);
        setError(
          e instanceof CellpApiError
            ? `${e.message}${e.status ? ` (${e.status})` : ""}`
            : "Failed to load queue",
        );
      } finally {
        setDetailLoading(false);
      }
    },
    [projectId, versionId],
  );

  useEffect(() => {
    if (!selectedName) return;
    void loadDetail(selectedName, peekLimit);
  }, [selectedName, loadDetail, peekLimit]);

  async function runAction(
    label: string,
    fn: () => Promise<unknown>,
    success: (result: unknown) => string,
  ): Promise<boolean> {
    if (!selected) return false;
    setBusy(true);
    setError(null);
    setFeedback(null);
    try {
      const result = await fn();
      setFeedback(success(result));
      await loadDetail(selected.name, peekLimit);
      return true;
    } catch (e) {
      setError(
        e instanceof CellpApiError
          ? `${e.message} (${e.status})`
          : `${label} failed`,
      );
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function handlePurge() {
    if (!selected) return;
    const ok = await runAction(
      "Purge",
      () => purgeQueue(projectId, versionId, selected.name, { force: true }),
      () => "Queue purged",
    );
    if (ok) setPurgeOpen(false);
  }

  const paused = asBoolean(info?.paused) ?? false;
  const backlog =
    infoNumber(info ?? {}, ["backlogCount", "pending"]) ?? messages.length;
  const stored = infoNumber(info ?? {}, ["stored"]);
  const backlogBytes = infoNumber(info ?? {}, ["backlogBytes"]);
  const hasConsumer = queues.some((q) => q.consumer);

  return (
    <div className="space-y-4">
      <div className="overflow-hidden rounded-md border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Queue</TableHead>
              <TableHead>Binding</TableHead>
              <TableHead>Role</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {queues.map((q) => (
              <TableRow
                key={q.name}
                className={cn(
                  "cursor-pointer",
                  q.name === selected?.name && "bg-accent/40",
                )}
                onClick={() => setSelectedName(q.name)}
              >
                <TableCell className="font-mono text-sm">{q.name}</TableCell>
                <TableCell className="font-mono text-sm text-muted-foreground">
                  {q.binding ?? "—"}
                </TableCell>
                <TableCell>
                  <Badge variant="outline" className="normal-case tracking-normal">
                    {q.consumer ? "consumer" : "producer"}
                  </Badge>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {hasConsumer && (
        <p
          role="note"
          data-testid="queue-consumer-note"
          className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm text-muted-foreground"
        >
          {CONSUMER_FETCH_NOTE}
        </p>
      )}

      {selected && (
        <Card>
          <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle className="font-mono">{selected.name}</CardTitle>
              <p className="mt-1 text-sm text-muted-foreground">
                {selected.binding ? `Binding ${selected.binding}` : "No producer binding"}
                {selected.deadLetterQueue
                  ? ` · DLQ ${selected.deadLetterQueue}`
                  : ""}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy || detailLoading}
                onClick={() => void loadDetail(selected.name, peekLimit)}
              >
                <Eye className="size-3.5" />
                Peek
              </Button>
              {paused ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() =>
                    void runAction(
                      "Resume",
                      () => resumeQueue(projectId, versionId, selected.name),
                      () => "Delivery resumed",
                    )
                  }
                >
                  <Play className="size-3.5" />
                  Resume
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() =>
                    void runAction(
                      "Pause",
                      () => pauseQueue(projectId, versionId, selected.name),
                      () => "Delivery paused",
                    )
                  }
                >
                  <Pause className="size-3.5" />
                  Pause
                </Button>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() =>
                  void runAction(
                    "Redrive",
                    () => redriveQueue(projectId, versionId, selected.name),
                    (result) => {
                      const n = redrivenCount(result);
                      return n == null
                        ? "Redrive complete"
                        : `Redrove ${n} message${n === 1 ? "" : "s"}`;
                    },
                  )
                }
              >
                <RotateCcw className="size-3.5" />
                Redrive
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={busy}
                onClick={() => setPurgeOpen(true)}
              >
                <Trash2 className="size-3.5" />
                Purge
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {detailLoading ? (
              <Skeleton className="h-24 w-full" />
            ) : (
              <dl data-testid="queue-info" className="grid gap-3 sm:grid-cols-3">
                <div>
                  <dt className="text-xs uppercase tracking-wider text-muted-foreground">
                    Backlog
                  </dt>
                  <dd className="mt-1 text-sm">
                    {backlog} message{backlog === 1 ? "" : "s"}
                    {backlogBytes != null ? ` · ${backlogBytes} B` : ""}
                  </dd>
                </div>
                <div>
                  <dt className="text-xs uppercase tracking-wider text-muted-foreground">
                    Stored
                  </dt>
                  <dd className="mt-1 text-sm">{stored ?? "—"}</dd>
                </div>
                <div>
                  <dt className="text-xs uppercase tracking-wider text-muted-foreground">
                    Delivery
                  </dt>
                  <dd className="mt-1 flex items-center gap-2 text-sm">
                    <Badge variant={paused ? "muted" : "outline"}>
                      {paused ? "Paused" : "Delivering"}
                    </Badge>
                  </dd>
                </div>
              </dl>
            )}

            {error && (
              <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </p>
            )}
            {feedback && (
              <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
                {feedback}
              </p>
            )}

            <div data-testid="queue-peek">
              <div className="mb-2 flex items-center justify-between gap-2">
                <h3 className="text-sm font-medium">Peek</h3>
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  Limit
                  <input
                    type="number"
                    min={1}
                    max={100}
                    value={peekLimit}
                    onChange={(e) => {
                      const n = Number(e.target.value);
                      if (Number.isInteger(n)) setPeekLimit(n);
                    }}
                    className="h-8 w-16 rounded-md border border-border bg-background px-2 font-mono text-sm"
                  />
                </label>
              </div>
              <div className="overflow-hidden rounded-md border border-border">
                {detailLoading ? (
                  <Skeleton className="h-32 w-full rounded-none" />
                ) : (
                  <PeekMessages messages={messages} />
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {selected && (
        <PurgeQueueDialog
          open={purgeOpen}
          queueName={selected.name}
          loading={busy}
          onConfirm={() => void handlePurge()}
          onCancel={() => setPurgeOpen(false)}
        />
      )}
    </div>
  );
}

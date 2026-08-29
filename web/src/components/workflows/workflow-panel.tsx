import { useEffect, useState } from "react";
import { Info } from "lucide-react";
import {
  CellpApiError,
  listWorkflowInstances,
  type WorkflowInstance,
  type WorkflowInstances,
  type WorkflowListItem,
} from "@/lib/cellp-api";
import { formatDateTime } from "@/lib/format";
import { Badge } from "@/components/ui/badge";
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

interface WorkflowPanelProps {
  projectId: string;
  versionId: string;
  workflows: WorkflowListItem[];
}

function instanceStatus(row: WorkflowInstance): string {
  if (typeof row.status === "string" && row.status) return row.status;
  if (row.reserved) return "reserved";
  return "—";
}

function instanceTime(row: WorkflowInstance): string {
  return formatDateTime(row.updated_at ?? row.created_at);
}

export function WorkflowPanel({
  projectId,
  versionId,
  workflows,
}: WorkflowPanelProps) {
  const [selectedName, setSelectedName] = useState(
    workflows[0]?.workflow_name ?? "",
  );
  const [detail, setDetail] = useState<WorkflowInstances | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const selected =
    workflows.find((w) => w.workflow_name === selectedName) ?? workflows[0];

  const firstName = workflows[0]?.workflow_name ?? "";

  useEffect(() => {
    setSelectedName(firstName);
  }, [projectId, versionId, firstName]);

  useEffect(() => {
    if (!selected?.workflow_name) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const data = await listWorkflowInstances(
          projectId,
          versionId,
          selected.workflow_name,
        );
        if (!cancelled) setDetail(data);
      } catch (e) {
        if (!cancelled) {
          setDetail(null);
          setError(
            e instanceof CellpApiError
              ? `${e.message} (${e.status})`
              : "Failed to load workflow instances",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, versionId, selected?.workflow_name]);

  const instances = detail?.instances ?? [];
  const limitation = detail?.limitation;

  return (
    <div className="space-y-4">
      <div className="overflow-hidden rounded-md border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>Workflow</TableHead>
              <TableHead>Binding</TableHead>
              <TableHead>Class</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {workflows.map((w) => (
              <TableRow
                key={w.workflow_name}
                className={cn(
                  "cursor-pointer",
                  w.workflow_name === selected?.workflow_name && "bg-accent/40",
                )}
                onClick={() => setSelectedName(w.workflow_name)}
              >
                <TableCell className="font-mono text-sm">
                  {w.workflow_name}
                </TableCell>
                <TableCell className="font-mono text-sm text-muted-foreground">
                  {w.binding}
                </TableCell>
                <TableCell className="font-mono text-sm text-muted-foreground">
                  {w.class_name}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <p
        data-testid="workflow-read-only-note"
        className="text-sm text-muted-foreground"
      >
        Instances are read-only. Pause, resume, restart, and delete are not
        offered — celld has no workflow operator CLI.
      </p>

      {typeof limitation === "string" && limitation.length > 0 && (
        <div
          role="status"
          data-testid="workflow-limitation"
          className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm"
        >
          <div className="flex items-start gap-2">
            <Info className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <p>{limitation}</p>
          </div>
        </div>
      )}

      {selected && (
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="font-mono">{selected.workflow_name}</CardTitle>
              {detail?.filter && (
                <Badge variant="outline" className="normal-case tracking-normal">
                  filter: {detail.filter}
                </Badge>
              )}
            </div>
            <p className="text-sm text-muted-foreground">
              Binding {selected.binding}
              {detail?.script_name ? ` · script ${detail.script_name}` : ""}
            </p>
          </CardHeader>
          <CardContent>
            {error && (
              <p className="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </p>
            )}
            {loading ? (
              <Skeleton className="h-32 w-full" />
            ) : (
              <div
                data-testid="workflow-instances"
                className="overflow-hidden rounded-md border border-border"
              >
                <Table>
                  <TableHeader>
                    <TableRow className="hover:bg-transparent">
                      <TableHead>ID</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Time</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {instances.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={3}
                          className="py-8 text-center text-muted-foreground"
                        >
                          No instances
                        </TableCell>
                      </TableRow>
                    ) : (
                      instances.map((row) => (
                        <TableRow key={row.id}>
                          <TableCell className="max-w-xs truncate font-mono text-xs">
                            {row.id}
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant="outline"
                              className="normal-case tracking-normal"
                            >
                              {instanceStatus(row)}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {instanceTime(row)}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

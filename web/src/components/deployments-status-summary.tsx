import { Link } from "react-router-dom";
import type { Version } from "@/lib/cellp-api";
import { summarizeVersionFleet } from "@/lib/inspection";
import { deploymentsHref } from "@/lib/routes";
import { cn } from "@/lib/utils";

interface DeploymentsStatusSummaryProps {
  projectId: string;
  versions: Version[];
  prodVersionId: string | null;
}

export function DeploymentsStatusSummary({
  projectId,
  versions,
  prodVersionId,
}: DeploymentsStatusSummaryProps) {
  const fleet = summarizeVersionFleet(versions);
  const chips = [
    { key: "ready", label: "Ready", count: fleet.ready, param: "ready" },
    { key: "progress", label: "In progress", count: fleet.inProgress, param: "" },
    { key: "failed", label: "Failed", count: fleet.failed, param: "failed" },
  ].filter((c) => c.count > 0 || c.key === "ready");

  return (
    <div
      className="flex flex-wrap items-center gap-2 text-sm"
      data-testid="deployments-status-summary"
    >
      <span className="text-muted-foreground">Fleet:</span>
      {chips.map((c) => (
        <Link
          key={c.key}
          to={
            c.param
              ? `${deploymentsHref(projectId)}?status=${encodeURIComponent(c.param)}`
              : deploymentsHref(projectId)
          }
          className={cn(
            "rounded-full border border-border px-2.5 py-0.5 tabular-nums transition-colors hover:bg-muted/60",
            c.key === "failed" && c.count > 0 && "border-destructive/40 text-destructive",
          )}
        >
          {c.label} {c.count}
        </Link>
      ))}
      {prodVersionId ? (
        <span className="text-xs text-muted-foreground">
          Prod: <span className="font-mono">{prodVersionId}</span>
        </span>
      ) : (
        <span className="text-xs text-muted-foreground">No prod pointer</span>
      )}
    </div>
  );
}

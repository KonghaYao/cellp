import { Info } from "lucide-react";

/** Preview binding copy — keep identical on hub, KV, Queue, and Workflow surfaces. */
export const AD7_BANNER_TEXT =
  "Preview KV / Queue inherit parent keys and backlog via branch (like D1). Workflow and Cron instances start empty.";

export function Ad7Banner({ className }: { className?: string }) {
  return (
    <div
      role="status"
      data-testid="ad7-banner"
      className={
        className ??
        "rounded-md border border-border bg-muted/40 px-4 py-3 text-sm"
      }
    >
      <div className="flex items-start gap-2">
        <Info className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <p>{AD7_BANNER_TEXT}</p>
      </div>
    </div>
  );
}

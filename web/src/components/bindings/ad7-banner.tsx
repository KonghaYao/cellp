import { Info } from "lucide-react";

/** AD-7 empty-start copy — keep identical on hub, KV, Queue, and Workflow surfaces. */
export const AD7_BANNER_TEXT =
  "Preview KV / Queue start empty and do not inherit Production keys or backlog. D1 still uses branch.";

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

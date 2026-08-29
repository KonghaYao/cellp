import { STATUS_DOT, STATUS_TEXT, statusLabel } from "@/lib/status";
import { cn } from "@/lib/utils";

export function StatusIndicator({
  status,
  className,
}: {
  status: string;
  className?: string;
}) {
  const dot = STATUS_DOT[status] ?? "bg-zinc-400";
  const text = STATUS_TEXT[status] ?? "text-muted-foreground";

  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <span className={cn("size-2 shrink-0 rounded-full", dot)} />
      <span className={cn("text-sm capitalize", text)}>{statusLabel(status)}</span>
    </span>
  );
}

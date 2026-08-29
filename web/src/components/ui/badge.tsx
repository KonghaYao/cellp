import { cn } from "@/lib/utils";

export function Badge({
  className,
  variant = "default",
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & {
  variant?: "default" | "prod" | "outline" | "muted";
}) {
  const styles = {
    default: "bg-secondary text-secondary-foreground",
    prod: "border border-foreground/15 bg-foreground text-background",
    outline: "border border-border bg-transparent text-muted-foreground",
    muted: "bg-muted text-muted-foreground",
  };

  return (
    <span
      className={cn(
        "inline-flex items-center rounded px-1.5 py-0.5 text-[11px] font-medium uppercase tracking-wide",
        styles[variant],
        className,
      )}
      {...props}
    />
  );
}

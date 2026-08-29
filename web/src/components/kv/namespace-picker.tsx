import { Layers } from "lucide-react";
import type { KvNamespace } from "@/lib/cellp-api";
import { cn } from "@/lib/utils";

interface NamespacePickerProps {
  namespaces: KvNamespace[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  className?: string;
}

export function NamespacePicker({
  namespaces,
  selectedId,
  onSelect,
  className,
}: NamespacePickerProps) {
  return (
    <nav
      aria-label="KV namespaces"
      data-testid="kv-namespace-picker"
      className={cn("rounded-lg border border-border bg-card", className)}
    >
      <div className="border-b border-border px-3 py-2">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Namespaces
        </p>
        <p className="text-xs text-muted-foreground">
          {namespaces.length} total
        </p>
      </div>
      <ul className="max-h-[28rem] overflow-y-auto p-1">
        {namespaces.map((ns) => {
          const isSelected = selectedId === ns.id;
          return (
            <li key={ns.id}>
              <button
                type="button"
                onClick={() => onSelect(ns.id)}
                aria-pressed={isSelected}
                className={cn(
                  "flex w-full items-start gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors",
                  isSelected
                    ? "bg-accent text-accent-foreground"
                    : "hover:bg-muted/60",
                )}
              >
                <Layers className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono">{ns.binding}</span>
                  <span className="mt-0.5 block truncate font-mono text-xs text-muted-foreground">
                    {ns.id}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

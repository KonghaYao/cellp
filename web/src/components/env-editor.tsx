import { useEffect, useState, useTransition } from "react";
import { Plus, Trash2 } from "lucide-react";
import {
  CellpApiError,
  getVersionEnv,
  putVersionEnv,
  type WorkerEnvVar,
} from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const fieldClass =
  "w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60";

interface EnvEditorProps {
  projectId: string;
  versionId: string;
}

export function EnvEditor({ projectId, versionId }: EnvEditorProps) {
  const [rows, setRows] = useState<WorkerEnvVar[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [pending, startTransition] = useTransition();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const vars = await getVersionEnv(projectId, versionId);
        if (!cancelled) {
          setRows(vars);
          setError(null);
        }
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof CellpApiError ? e.message : "Failed to load env");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, versionId]);

  function updateRow(index: number, patch: Partial<WorkerEnvVar>) {
    setRows((prev) => prev.map((row, i) => (i === index ? { ...row, ...patch } : row)));
    setSaved(false);
  }

  function addRow() {
    setRows((prev) => [
      ...prev,
      { key: "", value: "", source: "override", readonly: false },
    ]);
    setSaved(false);
  }

  function removeRow(index: number) {
    setRows((prev) => prev.filter((_, i) => i !== index));
    setSaved(false);
  }

  function handleSave() {
    const vars: Record<string, string> = {};
    for (const row of rows) {
      if (row.readonly || row.source === "platform") continue;
      const key = row.key.trim();
      if (!key) continue;
      vars[key] = row.value;
    }
    startTransition(async () => {
      try {
        await putVersionEnv(projectId, versionId, vars);
        const next = await getVersionEnv(projectId, versionId);
        setRows(next);
        setError(null);
        setSaved(true);
      } catch (e) {
        setSaved(false);
        setError(e instanceof CellpApiError ? e.message : "Failed to save env");
      }
    });
  }

  return (
    <div className="rounded-lg border border-border bg-card p-5" data-testid="worker-env-editor">
      <div className="mb-1 flex items-center justify-between gap-3">
        <h2 className="text-label-14 font-medium">Worker environment</h2>
        <Button type="button" size="sm" onClick={handleSave} disabled={pending || loading}>
          {pending ? "Saving…" : "Save"}
        </Button>
      </div>
      <p className="mb-4 text-sm text-muted-foreground">
        Overrides apply via celld <span className="font-mono">CELLD_VARS_FILE</span>.{" "}
        <span className="font-mono">PROJECT_ID</span> and{" "}
        <span className="font-mono">VERSION_ID</span> are platform-owned. Saving a ready
        version restarts its celld.
      </p>

      {loading && <p className="text-sm text-muted-foreground">Loading…</p>}
      {error && (
        <p className="mb-3 text-sm text-destructive" role="alert">
          {error}
        </p>
      )}
      {saved && !error && (
        <p className="mb-3 text-sm text-muted-foreground">Saved.</p>
      )}

      {!loading && (
        <div className="space-y-2">
          {rows.map((row, index) => (
            <div key={`${row.source}-${index}`} className="flex items-center gap-2">
              <input
                aria-label={`Env key ${index + 1}`}
                className={cn(fieldClass, "max-w-[12rem]")}
                value={row.key}
                disabled={row.readonly}
                placeholder="KEY"
                onChange={(e) => updateRow(index, { key: e.target.value, source: "override" })}
              />
              <input
                aria-label={`Env value ${index + 1}`}
                className={fieldClass}
                value={row.value}
                disabled={row.readonly}
                placeholder="value"
                onChange={(e) => updateRow(index, { value: e.target.value, source: "override" })}
              />
              <span className="w-20 shrink-0 text-[11px] uppercase tracking-wide text-muted-foreground">
                {row.source}
              </span>
              {!row.readonly && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-label={`Remove ${row.key || "row"}`}
                  onClick={() => removeRow(index)}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              )}
            </div>
          ))}
          <Button type="button" size="sm" variant="outline" onClick={addRow}>
            <Plus className="size-3.5" />
            Add variable
          </Button>
        </div>
      )}
    </div>
  );
}

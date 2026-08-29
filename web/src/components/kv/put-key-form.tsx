import { useState, useTransition } from "react";
import { CellpApiError, putKvKey } from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const fieldClass =
  "w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

interface PutKeyFormProps {
  projectId: string;
  versionId: string;
  ns: string;
  onPut: (key: string) => void;
}

export function PutKeyForm({
  projectId,
  versionId,
  ns,
  onPut,
}: PutKeyFormProps) {
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [ttl, setTtl] = useState("");
  const [base64, setBase64] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    const key = name.trim();
    if (!key) {
      setError("Key name is required");
      return;
    }
    setError(null);
    const ttlNumber = ttl.trim() ? Number(ttl) : undefined;
    if (ttlNumber != null && (!Number.isFinite(ttlNumber) || ttlNumber < 0)) {
      setError("TTL must be a non-negative number of seconds");
      return;
    }
    startTransition(async () => {
      try {
        await putKvKey(projectId, versionId, ns, key, {
          value,
          ttl: ttlNumber,
          base64: base64 || undefined,
        });
        setName("");
        setValue("");
        setTtl("");
        setBase64(false);
        onPut(key);
      } catch (e) {
        setError(e instanceof CellpApiError ? e.message : "Failed to put key");
      }
    });
  }

  return (
    <Card data-testid="kv-put-form">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Put key</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <label className="block space-y-1.5 text-sm">
            <span className="text-muted-foreground">Key name</span>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              aria-label="Key name"
              className={fieldClass}
              required
            />
          </label>
          <label className="block space-y-1.5 text-sm">
            <span className="text-muted-foreground">Value</span>
            <textarea
              value={value}
              onChange={(e) => setValue(e.target.value)}
              aria-label="Put value"
              rows={4}
              spellCheck={false}
              className={cn(fieldClass, "resize-y leading-relaxed")}
            />
          </label>
          <div className="flex flex-wrap items-end gap-4">
            <label className="block w-40 space-y-1.5 text-sm">
              <span className="text-muted-foreground">TTL (seconds)</span>
              <input
                type="number"
                min={0}
                value={ttl}
                onChange={(e) => setTtl(e.target.value)}
                aria-label="TTL"
                placeholder="optional"
                className={fieldClass}
              />
            </label>
            <label className="flex cursor-pointer items-center gap-2 pb-2 text-sm text-muted-foreground">
              <input
                type="checkbox"
                checked={base64}
                onChange={(e) => setBase64(e.target.checked)}
                className="size-3.5 rounded border-border"
              />
              Value is base64
            </label>
            <Button type="submit" className="ml-auto" disabled={pending}>
              {pending ? "Putting…" : "Put key"}
            </Button>
          </div>
          {error && (
            <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          )}
        </form>
      </CardContent>
    </Card>
  );
}

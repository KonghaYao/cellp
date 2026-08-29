import { useEffect, useState, useTransition } from "react";
import {
  CellpApiError,
  deleteKvKey,
  getKvKey,
  putKvKey,
  type KvValue,
} from "@/lib/cellp-api";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

const fieldClass =
  "w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

interface KeyEditorProps {
  projectId: string;
  versionId: string;
  ns: string;
  selectedKey: string | null;
  refreshToken: number;
  onDeleted: (key: string) => void;
  onSaved: (key: string) => void;
}

export function KeyEditor({
  projectId,
  versionId,
  ns,
  selectedKey,
  refreshToken,
  onDeleted,
  onSaved,
}: KeyEditorProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [value, setValue] = useState<KvValue | null>(null);
  const [draft, setDraft] = useState("");
  const [pending, startTransition] = useTransition();
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    if (!selectedKey) {
      setValue(null);
      setDraft("");
      setError(null);
      setLoading(false);
      return;
    }
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await getKvKey(projectId, versionId, ns, selectedKey);
        if (cancelled) return;
        setValue(data);
        setDraft(data.value);
        setError(null);
      } catch (e) {
        if (!cancelled) {
          setValue(null);
          setDraft("");
          setError(
            e instanceof CellpApiError ? e.message : "Failed to load key",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [ns, projectId, refreshToken, selectedKey, versionId]);

  function handleSave() {
    if (!selectedKey) return;
    setError(null);
    startTransition(async () => {
      try {
        await putKvKey(projectId, versionId, ns, selectedKey, {
          value: draft,
          base64: value?.encoding === "base64",
        });
        onSaved(selectedKey);
      } catch (e) {
        setError(e instanceof CellpApiError ? e.message : "Failed to save key");
      }
    });
  }

  function handleDelete() {
    if (!selectedKey) return;
    startTransition(async () => {
      try {
        await deleteKvKey(projectId, versionId, ns, selectedKey);
        setConfirmDelete(false);
        onDeleted(selectedKey);
      } catch (e) {
        setConfirmDelete(false);
        setError(
          e instanceof CellpApiError ? e.message : "Failed to delete key",
        );
      }
    });
  }

  if (!selectedKey) {
    return (
      <Card>
        <CardContent className="py-12 text-center text-sm text-muted-foreground">
          Select a key to view its value
        </CardContent>
      </Card>
    );
  }

  return (
    <Card data-testid="kv-key-editor">
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0 pb-3">
        <div className="min-w-0">
          <CardTitle className="truncate font-mono text-base">
            {selectedKey}
          </CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            {loading ? "Loading value…" : "Get · put · delete"}
          </p>
        </div>
        {value && (
          <Badge variant="outline" className="normal-case tracking-normal">
            {value.encoding}
          </Badge>
        )}
      </CardHeader>
      <CardContent className="space-y-3">
        {error && (
          <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}
        {loading ? (
          <Skeleton className="h-32 w-full" />
        ) : (
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            rows={8}
            spellCheck={false}
            aria-label="Key value"
            className={cn(fieldClass, "resize-y leading-relaxed")}
          />
        )}
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={loading || pending}
            onClick={() => setConfirmDelete(true)}
          >
            Delete
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={loading || pending}
            onClick={handleSave}
          >
            {pending ? "Saving…" : "Save"}
          </Button>
        </div>
      </CardContent>
      <ConfirmDialog
        open={confirmDelete}
        title="Delete key?"
        description={`Delete ${selectedKey} from this namespace? This cannot be undone.`}
        confirmLabel="Delete"
        destructive
        loading={pending}
        onConfirm={handleDelete}
        onCancel={() => setConfirmDelete(false)}
      />
    </Card>
  );
}

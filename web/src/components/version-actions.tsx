import { useState, useTransition } from "react";
import { ExternalLink, Rocket, Trash2 } from "lucide-react";
import { promoteVersion, destroyVersion, CellpApiError } from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/confirm-dialog";

interface VersionActionsProps {
  projectId: string;
  versionId: string;
  status: string;
  isProd: boolean;
  previewUrl?: string;
  onComplete?: () => void | Promise<void>;
}

export function VersionActions({
  projectId,
  versionId,
  status,
  isProd,
  previewUrl,
  onComplete,
}: VersionActionsProps) {
  const [pending, startTransition] = useTransition();
  const [promoteOpen, setPromoteOpen] = useState(false);
  const [destroyOpen, setDestroyOpen] = useState(false);
  const [promoted, setPromoted] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const isProdNow = isProd || promoted;
  const canPromote = status === "ready" && !isProdNow;
  const canDestroy = status !== "destroyed" && status !== "draining";

  function showFeedback(message: string) {
    setFeedback(message);
    setTimeout(() => setFeedback(null), 3000);
  }

  function handlePromote() {
    setError(null);
    startTransition(async () => {
      try {
        await promoteVersion(projectId, versionId);
        setPromoted(true);
        setPromoteOpen(false);
        showFeedback("Promoted to production");
        await onComplete?.();
      } catch (e) {
        setError(
          e instanceof CellpApiError ? e.message : "Promote failed",
        );
      }
    });
  }

  function handleDestroy() {
    setError(null);
    startTransition(async () => {
      try {
        await destroyVersion(projectId, versionId);
        setDestroyOpen(false);
        showFeedback("Destroy initiated");
        await onComplete?.();
      } catch (e) {
        setError(
          e instanceof CellpApiError ? e.message : "Destroy failed",
        );
      }
    });
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {previewUrl && (
        <a
          href={previewUrl}
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-8 items-center gap-2 rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
        >
          <ExternalLink className="size-3.5" />
          Open preview
        </a>
      )}
      {canPromote && (
        <Button onClick={() => setPromoteOpen(true)} disabled={pending} size="sm">
          <Rocket className="size-3.5" />
          Promote to prod
        </Button>
      )}
      {canDestroy && (
        <Button
          variant="destructive"
          onClick={() => setDestroyOpen(true)}
          disabled={pending}
          size="sm"
        >
          <Trash2 className="size-3.5" />
          Destroy
        </Button>
      )}
      {isProdNow && (
        <span className="text-sm text-muted-foreground">
          Current production version
        </span>
      )}
      {feedback && (
        <span className="rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs text-emerald-700">
          {feedback}
        </span>
      )}
      {error && <p className="w-full text-sm text-destructive">{error}</p>}

      <ConfirmDialog
        open={promoteOpen}
        title="Promote to production?"
        description={`Version ${versionId} will become the production cutover for ${projectId}. This triggers an atomic prod pointer update.`}
        confirmLabel="Promote"
        loading={pending}
        onConfirm={handlePromote}
        onCancel={() => setPromoteOpen(false)}
      />
      <ConfirmDialog
        open={destroyOpen}
        title="Destroy version?"
        description={`Version ${versionId} will enter draining and then be destroyed. Gateway routes will be removed.`}
        confirmLabel="Destroy"
        destructive
        loading={pending}
        onConfirm={handleDestroy}
        onCancel={() => setDestroyOpen(false)}
      />
    </div>
  );
}

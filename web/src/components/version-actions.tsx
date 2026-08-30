import { useState, useTransition } from "react";
import { Archive, ExternalLink, Moon, Pin, PinOff, Rocket, Sun, Trash2 } from "lucide-react";
import {
  promoteVersion,
  destroyVersion,
  archiveVersion,
  wakeVersion,
  pinVersion,
  unpinVersion,
  CellpApiError,
} from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/confirm-dialog";

interface VersionActionsProps {
  projectId: string;
  versionId: string;
  status: string;
  isProd: boolean;
  pinned?: boolean;
  previewUrl?: string;
  onComplete?: () => void | Promise<void>;
}

export function VersionActions({
  projectId,
  versionId,
  status,
  isProd,
  pinned = false,
  previewUrl,
  onComplete,
}: VersionActionsProps) {
  const [pending, startTransition] = useTransition();
  const [promoteOpen, setPromoteOpen] = useState(false);
  const [destroyOpen, setDestroyOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const [promoted, setPromoted] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const isProdNow = isProd || promoted;
  const canPromote = status === "ready" && !isProdNow;
  const canArchive = status === "ready" && !isProdNow;
  const canWake = status === "archived";
  const canPin = (status === "ready" || status === "archived") && !pinned;
  const canUnpin = pinned;
  const canDestroy =
    status !== "destroyed" && status !== "draining" && !isProdNow;
  const destroyBlockedByProd = isProdNow && status !== "destroyed" && status !== "draining";

  function showFeedback(message: string) {
    setFeedback(message);
    setTimeout(() => setFeedback(null), 3000);
  }

  function runAction(fn: () => Promise<void>) {
    setError(null);
    startTransition(async () => {
      try {
        await fn();
        await onComplete?.();
      } catch (e) {
        setError(e instanceof CellpApiError ? e.message : "Action failed");
      }
    });
  }

  function handlePromote() {
    runAction(async () => {
      await promoteVersion(projectId, versionId);
      setPromoted(true);
      setPromoteOpen(false);
      showFeedback("Promoted to production");
    });
  }

  function handleDestroy() {
    runAction(async () => {
      await destroyVersion(projectId, versionId);
      setDestroyOpen(false);
      showFeedback("Destroy initiated");
    });
  }

  function handleArchive() {
    runAction(async () => {
      await archiveVersion(projectId, versionId);
      setArchiveOpen(false);
      showFeedback("Version archived");
    });
  }

  function handleWake() {
    runAction(async () => {
      await wakeVersion(projectId, versionId);
      showFeedback("Version woken");
    });
  }

  function handlePin() {
    runAction(async () => {
      await pinVersion(projectId, versionId);
      showFeedback("Pinned");
    });
  }

  function handleUnpin() {
    runAction(async () => {
      await unpinVersion(projectId, versionId);
      showFeedback("Unpinned");
    });
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {previewUrl && status === "ready" && (
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
      {status === "archived" && (
        <span className="inline-flex items-center gap-1 rounded-md border border-border bg-muted px-2 py-1 text-xs text-muted-foreground">
          <Moon className="size-3 h-3" />
          Archived
        </span>
      )}
      {pinned && (
        <span className="inline-flex items-center gap-1 rounded-md border border-amber-200 bg-amber-50 px-2 py-1 text-xs text-amber-800">
          <Pin className="size-3 h-3" />
          Pinned
        </span>
      )}
      {canWake && (
        <Button onClick={handleWake} disabled={pending} size="sm" variant="outline">
          <Sun className="size-3.5" />
          Wake
        </Button>
      )}
      {canPromote && (
        <Button onClick={() => setPromoteOpen(true)} disabled={pending} size="sm">
          <Rocket className="size-3.5" />
          Promote to prod
        </Button>
      )}
      {canArchive && (
        <Button onClick={() => setArchiveOpen(true)} disabled={pending} size="sm" variant="outline">
          <Archive className="size-3.5" />
          Archive
        </Button>
      )}
      {canPin && (
        <Button onClick={handlePin} disabled={pending} size="sm" variant="outline">
          <Pin className="size-3.5" />
          Pin
        </Button>
      )}
      {canUnpin && (
        <Button onClick={handleUnpin} disabled={pending} size="sm" variant="outline">
          <PinOff className="size-3.5" />
          Unpin
        </Button>
      )}
      {destroyBlockedByProd && (
        <span className="text-sm text-muted-foreground">
          Demote or promote another version first
        </span>
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
        open={archiveOpen}
        title="Archive version?"
        description={`Version ${versionId} will stop its celld process but keep S3 data. Preview URLs return 503 until you wake it.`}
        confirmLabel="Archive"
        loading={pending}
        onConfirm={handleArchive}
        onCancel={() => setArchiveOpen(false)}
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

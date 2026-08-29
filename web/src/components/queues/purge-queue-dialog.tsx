import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface PurgeQueueDialogProps {
  open: boolean;
  queueName: string;
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function PurgeQueueDialog({
  open,
  queueName,
  loading = false,
  onConfirm,
  onCancel,
}: PurgeQueueDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [forceChecked, setForceChecked] = useState(false);

  useEffect(() => {
    const el = dialogRef.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  useEffect(() => {
    if (open) setForceChecked(false);
  }, [open]);

  if (!open) return null;

  return (
    <dialog
      ref={dialogRef}
      className={cn(
        "fixed inset-0 z-50 m-auto w-full max-w-md rounded-lg border border-border bg-card p-0 shadow-lg backdrop:bg-black/40",
      )}
      onClose={onCancel}
      onClick={(e) => {
        if (e.target === dialogRef.current) onCancel();
      }}
    >
      <div className="p-6">
        <h2 className="text-lg font-semibold">Purge queue?</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          This permanently deletes every message in{" "}
          <span className="font-mono text-foreground">{queueName}</span>. The
          API will only accept <span className="font-mono">{"{ force: true }"}</span>
          . Confirm that explicitly below — purge is never sent without it.
        </p>
        <label className="mt-4 flex items-start gap-2 text-sm">
          <input
            type="checkbox"
            className="mt-0.5 size-4 accent-destructive"
            checked={forceChecked}
            onChange={(e) => setForceChecked(e.target.checked)}
            disabled={loading}
          />
          <span>
            Send <span className="font-mono">force: true</span> and delete the
            backlog
          </span>
        </label>
        <div className="mt-6 flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={loading}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={loading || !forceChecked}
          >
            {loading ? "Purging…" : "Purge with force"}
          </Button>
        </div>
      </div>
    </dialog>
  );
}

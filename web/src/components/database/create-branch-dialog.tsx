import { useEffect, useRef, useState, useTransition } from "react";
import { useNavigate } from "react-router-dom";
import { GitBranch, Plus } from "lucide-react";
import {
  createVersion,
  CellpApiError,
  type Version,
} from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface CreateBranchDialogProps {
  projectId: string;
  versions: Version[];
  defaultParentId: string;
  onCreated?: () => void | Promise<void>;
  className?: string;
}

import { storageBrowserHref } from "@/lib/routes";

export function CreateBranchDialog({
  projectId,
  versions,
  defaultParentId,
  onCreated,
  className,
}: CreateBranchDialogProps) {
  const navigate = useNavigate();
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [open, setOpen] = useState(false);
  const [pending, startTransition] = useTransition();
  const [branchId, setBranchId] = useState("");
  const [parentId, setParentId] = useState(defaultParentId);
  const [gitRef, setGitRef] = useState("");
  const [error, setError] = useState<string | null>(null);

  const readyParents = versions.filter((v) => v.status === "ready");

  useEffect(() => {
    const el = dialogRef.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  useEffect(() => {
    if (open) {
      setParentId(defaultParentId);
      setBranchId("");
      setGitRef("");
      setError(null);
    }
  }, [open, defaultParentId]);

  function handleOpen() {
    setOpen(true);
  }

  function handleClose() {
    if (!pending) setOpen(false);
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    const id = branchId.trim();
    if (!id) {
      setError("Branch name is required");
      return;
    }
    if (!parentId) {
      setError("Select a parent branch");
      return;
    }

    setError(null);
    startTransition(async () => {
      try {
        const result = await createVersion(projectId, {
          id,
          parent_version_id: parentId,
          git_ref: gitRef.trim() || `branch/${id}`,
        });
        setOpen(false);
        await onCreated?.();
        navigate(storageBrowserHref(projectId, result.id));
      } catch (e) {
        setError(
          e instanceof CellpApiError ? e.message : "Failed to create branch",
        );
      }
    });
  }

  return (
    <>
      <Button type="button" size="sm" onClick={handleOpen} className={className}>
        <Plus className="size-3.5" />
        New branch
      </Button>

      {open && (
        <dialog
          ref={dialogRef}
          className={cn(
            "fixed inset-0 z-50 m-auto w-full max-w-md rounded-lg border border-border bg-card p-0 shadow-lg backdrop:bg-black/40",
          )}
          onClose={handleClose}
          onClick={(e) => {
            if (e.target === dialogRef.current) handleClose();
          }}
        >
          <form onSubmit={handleSubmit} className="p-6">
            <div className="flex items-center gap-2">
              <GitBranch className="size-4 text-muted-foreground" />
              <h2 className="text-lg font-semibold">Create branch</h2>
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              Fork a new data branch from a parent version. Deployment starts
              immediately after creation.
            </p>

            <div className="mt-5 space-y-4">
              <label className="block space-y-1.5 text-sm">
                <span className="text-muted-foreground">Branch name</span>
                <input
                  type="text"
                  value={branchId}
                  onChange={(e) => setBranchId(e.target.value)}
                  placeholder="v-feature-x"
                  className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  autoFocus
                  required
                />
              </label>

              <label className="block space-y-1.5 text-sm">
                <span className="text-muted-foreground">Parent branch</span>
                <select
                  value={parentId}
                  onChange={(e) => setParentId(e.target.value)}
                  className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  required
                >
                  {readyParents.map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.id} ({v.git_ref || "—"})
                    </option>
                  ))}
                </select>
              </label>

              <label className="block space-y-1.5 text-sm">
                <span className="text-muted-foreground">Git ref (optional)</span>
                <input
                  type="text"
                  value={gitRef}
                  onChange={(e) => setGitRef(e.target.value)}
                  placeholder="feature/my-change"
                  className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </label>
            </div>

            {error && (
              <p className="mt-4 text-sm text-destructive">{error}</p>
            )}

            <div className="mt-6 flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={handleClose}
                disabled={pending}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={pending}>
                {pending ? "Creating…" : "Create branch"}
              </Button>
            </div>
          </form>
        </dialog>
      )}
    </>
  );
}

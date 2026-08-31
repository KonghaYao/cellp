import { useEffect, useRef, useState, useTransition, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { FolderPlus } from "lucide-react";
import { createProject, CellpApiError } from "@/lib/cellp-api";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const PROJECT_ID_PATTERN = /^[a-z][a-z0-9-]{1,62}[a-z0-9]$/;

interface CreateProjectDialogProps {
  onCreated?: () => void | Promise<void>;
  className?: string;
}

export function CreateProjectDialog({
  onCreated,
  className,
}: CreateProjectDialogProps) {
  const navigate = useNavigate();
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [open, setOpen] = useState(false);
  const [pending, startTransition] = useTransition();
  const [projectId, setProjectId] = useState("");
  const [gitRemote, setGitRemote] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const el = dialogRef.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  useEffect(() => {
    if (open) {
      setProjectId("");
      setGitRemote("");
      setError(null);
    }
  }, [open]);

  function handleClose() {
    if (pending) return;
    setOpen(false);
  }

  function validateId(id: string): string | null {
    const trimmed = id.trim();
    if (!trimmed) return "Project id is required.";
    if (!PROJECT_ID_PATTERN.test(trimmed)) {
      return "Use 3–64 chars: lowercase letters, numbers, hyphens; start with a letter.";
    }
    return null;
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const id = projectId.trim();
    const idErr = validateId(id);
    if (idErr) {
      setError(idErr);
      return;
    }
    setError(null);
    startTransition(async () => {
      try {
        await createProject({
          id,
          git_remote: gitRemote.trim() || undefined,
        });
        setOpen(false);
        await onCreated?.();
        navigate(`/projects/${encodeURIComponent(id)}`);
      } catch (err) {
        setError(
          err instanceof CellpApiError
            ? `${err.message} (${err.status})`
            : "Failed to create project",
        );
      }
    });
  }

  return (
    <>
      <Button type="button" onClick={() => setOpen(true)} className={className}>
        <FolderPlus className="size-4" />
        New project
      </Button>
      {open ? (
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
          <form className="p-6" onSubmit={handleSubmit}>
            <h2 className="text-lg font-semibold">Create project</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Registers an empty project. Deployments arrive via{" "}
              <code className="text-xs">cellp dev</code> or CI — the Dashboard
              does not upload Worker bundles.
            </p>
            <label className="mt-4 block text-sm font-medium">
              Project id
              <input
                type="text"
                value={projectId}
                onChange={(e) => setProjectId(e.target.value)}
                placeholder="my-shop"
                autoComplete="off"
                className="mt-1 h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
                disabled={pending}
              />
            </label>
            <label className="mt-3 block text-sm font-medium">
              Git remote{" "}
              <span className="font-normal text-muted-foreground">(optional)</span>
              <input
                type="url"
                value={gitRemote}
                onChange={(e) => setGitRemote(e.target.value)}
                placeholder="https://github.com/org/repo"
                autoComplete="off"
                className="mt-1 h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
                disabled={pending}
              />
            </label>
            {error ? (
              <p className="mt-3 text-sm text-destructive" role="alert">
                {error}
              </p>
            ) : null}
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
                {pending ? "Creating…" : "Create"}
              </Button>
            </div>
          </form>
        </dialog>
      ) : null}
    </>
  );
}

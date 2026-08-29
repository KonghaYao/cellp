import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ChevronDown, Database } from "lucide-react";
import {
  getProject,
  listVersions,
  CellpApiError,
  type Version,
} from "@/lib/cellp-api";
import { StatusIndicator } from "@/components/status-indicator";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface VersionSwitcherProps {
  projectId: string;
  versionId: string;
  onVersionChange?: (versionId: string) => void;
  className?: string;
}

import { storageBrowserHref } from "@/lib/routes";

export function VersionSwitcher({
  projectId,
  versionId,
  onVersionChange,
  className,
}: VersionSwitcherProps) {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const project = await getProject(projectId);
        const allVersions: Version[] = [];
        let cursor: string | null = null;
        do {
          const page = await listVersions(projectId, { cursor });
          allVersions.push(...page.versions);
          cursor = page.next_cursor;
        } while (cursor);

        if (cancelled) return;
        setProdVersionId(project.prod_version_id);
        setVersions(allVersions);
        setError(null);
      } catch (e) {
        if (!cancelled) {
          setError(
            e instanceof CellpApiError
              ? e.message
              : "Failed to load versions",
          );
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId]);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(event.target as Node)
      ) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [open]);

  const sorted = useMemo(
    () =>
      [...versions].sort((a, b) => {
        const aTime = a.created_at ? new Date(a.created_at).getTime() : 0;
        const bTime = b.created_at ? new Date(b.created_at).getTime() : 0;
        return bTime - aTime;
      }),
    [versions],
  );

  const current = versions.find((v) => v.id === versionId);

  function selectVersion(nextId: string) {
    if (nextId === versionId) {
      setOpen(false);
      return;
    }
    onVersionChange?.(nextId);
    navigate(storageBrowserHref(projectId, nextId));
    setOpen(false);
  }

  if (loading) {
    return <Skeleton className={cn("h-10 w-full max-w-md", className)} />;
  }

  if (error) {
    return (
      <div
        className={cn(
          "rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive",
          className,
        )}
      >
        {error}
      </div>
    );
  }

  return (
    <div ref={containerRef} className={cn("relative max-w-md", className)}>
      <Button
        type="button"
        variant="outline"
        className="h-auto w-full justify-between gap-3 px-3 py-2 text-left"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        aria-haspopup="listbox"
      >
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Database className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate font-mono text-sm">
              {current?.id ?? versionId}
            </span>
            {prodVersionId === versionId && (
              <Badge variant="prod">Production</Badge>
            )}
          </div>
          {current && (
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {current.git_ref || "—"}
            </p>
          )}
        </div>
        <ChevronDown
          className={cn(
            "size-4 shrink-0 text-muted-foreground transition-transform",
            open && "rotate-180",
          )}
        />
      </Button>

      {open && (
        <div
          role="listbox"
          className="absolute z-20 mt-1 max-h-72 w-full overflow-auto rounded-lg border border-border bg-card py-1 shadow-md"
        >
          {sorted.length === 0 ? (
            <p className="px-3 py-2 text-sm text-muted-foreground">
              No versions found
            </p>
          ) : (
            sorted.map((version) => {
              const isProd = prodVersionId === version.id;
              const isSelected = version.id === versionId;
              return (
                <button
                  key={version.id}
                  type="button"
                  role="option"
                  aria-selected={isSelected}
                  className={cn(
                    "flex w-full flex-col gap-1 border-b border-border/50 px-3 py-2 text-left last:border-0 hover:bg-muted/60",
                    isSelected && "bg-accent/50",
                  )}
                  onClick={() => selectVersion(version.id)}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm">{version.id}</span>
                    {isProd && <Badge variant="prod">Production</Badge>}
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="text-xs text-muted-foreground">
                      {version.git_ref || "—"}
                    </span>
                    <StatusIndicator status={version.status} />
                  </div>
                </button>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

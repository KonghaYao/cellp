import { Link } from "react-router-dom";
import { Boxes } from "lucide-react";

interface AppHeaderProps {
  projectId?: string;
}

export function AppHeader({ projectId }: AppHeaderProps) {
  return (
    <header className="sticky top-0 z-40 border-b border-border bg-background/80 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-4 px-4 sm:px-6">
        <Link to="/" className="flex items-center gap-2 transition-opacity hover:opacity-80">
          <div className="flex size-7 items-center justify-center rounded-md border border-border bg-card">
            <Boxes className="size-4" />
          </div>
          <span className="text-sm font-semibold tracking-tight">cellp</span>
        </Link>

        {projectId && (
          <>
            <span className="text-muted-foreground/50">/</span>
            <Link
              to={`/projects/${projectId}`}
              className="truncate text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              {projectId}
            </Link>
          </>
        )}

        <div className="ml-auto flex items-center gap-3">
          <span className="hidden text-xs text-muted-foreground sm:inline">
            Control plane
          </span>
        </div>
      </div>
    </header>
  );
}

import { useEffect, useState } from "react";
import { Link, Outlet, useLocation } from "react-router-dom";
import { Boxes, ChevronRight, Menu, X } from "lucide-react";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { cn } from "@/lib/utils";

export function AppShell() {
  const location = useLocation();
  const projectMatch = location.pathname.match(/^\/projects\/([^/]+)/);
  const projectId = projectMatch?.[1];
  const inProject = Boolean(projectId);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  useEffect(() => {
    setMobileNavOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex min-h-screen bg-background">
      {/* Desktop sidebar */}
      <aside
        className={cn(
          "sticky top-0 hidden h-screen w-[240px] shrink-0 flex-col border-r border-border bg-sidebar md:flex",
        )}
      >
        <SidebarChrome inProject={inProject} projectId={projectId} />
      </aside>

      {/* Mobile drawer overlay */}
      {mobileNavOpen && (
        <button
          type="button"
          aria-label="Close navigation"
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={() => setMobileNavOpen(false)}
        />
      )}

      {/* Mobile drawer */}
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 flex w-[min(280px,85vw)] flex-col border-r border-border bg-sidebar transition-transform duration-200 md:hidden",
          mobileNavOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <div className="flex h-14 items-center justify-end border-b border-border px-2">
          <button
            type="button"
            aria-label="Close navigation"
            className="inline-flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            onClick={() => setMobileNavOpen(false)}
          >
            <X className="size-5" />
          </button>
        </div>
        <SidebarChrome inProject={inProject} projectId={projectId} />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border bg-background/95 px-4 backdrop-blur-sm md:px-8">
          <button
            type="button"
            aria-label="Open navigation"
            className="inline-flex size-9 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground md:hidden"
            onClick={() => setMobileNavOpen(true)}
          >
            <Menu className="size-5" />
          </button>
          {inProject ? (
            <>
              <Link
                to="/"
                className="text-sm text-muted-foreground transition-colors hover:text-foreground"
              >
                Projects
              </Link>
              <ChevronRight className="size-3.5 text-muted-foreground/60" />
              <span className="truncate font-mono text-sm font-medium">
                {projectId}
              </span>
            </>
          ) : (
            <span className="text-sm font-medium">Projects</span>
          )}
        </header>
        <main className="flex-1 overflow-y-auto px-4 py-6 md:px-8 md:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

function SidebarChrome({
  inProject,
  projectId,
}: {
  inProject: boolean;
  projectId?: string;
}) {
  return (
    <>
      <div className="flex h-14 items-center gap-2 border-b border-border px-4">
        <Link
          to="/"
          className="flex min-w-0 flex-1 items-center gap-2 transition-opacity hover:opacity-80"
        >
          <div className="flex size-7 shrink-0 items-center justify-center rounded-md border border-border bg-card">
            <Boxes className="size-4" />
          </div>
          <span className="truncate text-sm font-semibold tracking-tight">
            cellp
          </span>
        </Link>
      </div>

      <nav className="flex-1 overflow-y-auto">
        <AppSidebar projectId={inProject ? projectId : undefined} />
      </nav>

      <div className="border-t border-border p-3">
        <p className="text-xs text-muted-foreground">Control plane</p>
      </div>
    </>
  );
}

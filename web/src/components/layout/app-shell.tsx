import { Link, NavLink, Outlet, useLocation } from "react-router-dom";
import { Boxes, ChevronRight, LayoutGrid } from "lucide-react";
import { ProjectSidebar } from "@/components/layout/project-sidebar";
import { cn } from "@/lib/utils";

export function AppShell() {
  const location = useLocation();
  const projectMatch = location.pathname.match(/^\/projects\/([^/]+)/);
  const projectId = projectMatch?.[1];
  const inProject = Boolean(projectId);

  return (
    <div className="flex min-h-screen bg-background">
      <aside
        className={cn(
          "sticky top-0 flex h-screen w-[240px] shrink-0 flex-col border-r border-border bg-sidebar",
        )}
      >
        <div className="flex h-14 items-center gap-2 border-b border-border px-4">
          <Link
            to="/"
            className="flex min-w-0 flex-1 items-center gap-2 transition-opacity hover:opacity-80"
          >
            <div className="flex size-7 shrink-0 items-center justify-center rounded-md border border-border bg-card">
              <Boxes className="size-4" />
            </div>
            <span className="truncate text-sm font-semibold tracking-tight">cellp</span>
          </Link>
        </div>

        <nav className="flex-1 overflow-y-auto p-2">
          {!inProject ? (
            <ul className="space-y-0.5">
              <SidebarLink
                to="/"
                icon={<LayoutGrid className="size-4" />}
                label="Projects"
                end
              />
            </ul>
          ) : (
            <ProjectSidebar projectId={projectId!} />
          )}
        </nav>

        <div className="border-t border-border p-3">
          <p className="text-xs text-muted-foreground">Control plane</p>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        {inProject && (
          <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border bg-background/95 px-8 backdrop-blur-sm">
            <Link
              to="/"
              className="text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              Projects
            </Link>
            <ChevronRight className="size-3.5 text-muted-foreground/60" />
            <span className="truncate font-mono text-sm font-medium">{projectId}</span>
          </header>
        )}
        <main className="flex-1 overflow-y-auto px-8 py-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

function SidebarLink({
  to,
  icon,
  label,
  end,
}: {
  to: string;
  icon: React.ReactNode;
  label: string;
  end?: boolean;
}) {
  return (
    <li>
      <NavLink
        to={to}
        end={end}
        className={({ isActive }) =>
          cn(
            "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
            isActive
              ? "bg-accent font-medium text-foreground"
              : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
          )
        }
      >
        {icon}
        {label}
      </NavLink>
    </li>
  );
}

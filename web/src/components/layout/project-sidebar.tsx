import { NavLink } from "react-router-dom";
import {
  Database,
  LayoutDashboard,
  Rocket,
  Settings,
} from "lucide-react";
import {
  deploymentsHref,
  projectOverviewHref,
  settingsHref,
  storageHref,
} from "@/lib/routes";
import { cn } from "@/lib/utils";

interface ProjectSidebarProps {
  projectId: string;
}

const NAV = [
  { key: "overview", label: "Overview", icon: LayoutDashboard, href: projectOverviewHref },
  { key: "deployments", label: "Deployments", icon: Rocket, href: deploymentsHref },
  { key: "storage", label: "Storage", icon: Database, href: storageHref },
] as const;

export function ProjectSidebar({ projectId }: ProjectSidebarProps) {
  return (
    <div className="space-y-6">
      <div className="px-2">
        <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          Project
        </p>
        <p className="mt-1 truncate font-mono text-sm font-medium">{projectId}</p>
      </div>

      <ul className="space-y-0.5">
        {NAV.map(({ key, label, icon: Icon, href }) => (
          <li key={key}>
            <NavLink
              to={href(projectId)}
              end={key === "overview"}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
                  isActive
                    ? "bg-accent font-medium text-foreground"
                    : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                )
              }
            >
              <Icon className="size-4 shrink-0" />
              {label}
            </NavLink>
          </li>
        ))}
      </ul>

      <div>
        <p className="mb-1 px-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          Settings
        </p>
        <ul className="space-y-0.5">
          <li>
            <NavLink
              to={settingsHref(projectId)}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
                  isActive
                    ? "bg-accent font-medium text-foreground"
                    : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                )
              }
            >
              <Settings className="size-4 shrink-0" />
              General
            </NavLink>
          </li>
        </ul>
      </div>
    </div>
  );
}

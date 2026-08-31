import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import {
  Activity,
  Clock,
  Database,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LayoutGrid,
  ListOrdered,
  Rocket,
  ScanSearch,
  Settings,
  Workflow,
} from "lucide-react";
import {
  bindingsCronHref,
  bindingsD1Href,
  bindingsKvHref,
  bindingsQueuesHref,
  bindingsR2Href,
  bindingsWorkflowsHref,
  deploymentsHref,
  inspectHref,
  platformHref,
  projectOverviewHref,
  settingsHref,
  storageHref,
} from "@/lib/routes";
import { cn } from "@/lib/utils";

interface AppSidebarProps {
  projectId?: string;
}

const GLOBAL_NAV = [
  { key: "projects", label: "Projects", icon: LayoutGrid, to: "/", end: true },
  {
    key: "platform",
    label: "Platform health",
    icon: Activity,
    to: platformHref(),
    end: true,
  },
] as const;

const BINDINGS_NAV = [
  { key: "d1", label: "D1", icon: Database, to: bindingsD1Href() },
  { key: "kv", label: "KV", icon: KeyRound, to: bindingsKvHref() },
  { key: "queues", label: "Queues", icon: ListOrdered, to: bindingsQueuesHref() },
  { key: "workflows", label: "Workflows", icon: Workflow, to: bindingsWorkflowsHref() },
  { key: "r2", label: "R2", icon: HardDrive, to: bindingsR2Href() },
  { key: "cron", label: "Cron", icon: Clock, to: bindingsCronHref() },
] as const;

function projectNav(projectId: string) {
  return [
    {
      key: "overview",
      label: "Overview",
      icon: LayoutDashboard,
      to: projectOverviewHref(projectId),
      end: true,
    },
    {
      key: "inspect",
      label: "Inspect",
      icon: ScanSearch,
      to: inspectHref(projectId),
      end: true,
    },
    {
      key: "deployments",
      label: "Deployments",
      icon: Rocket,
      to: deploymentsHref(projectId),
      end: false,
    },
    {
      key: "storage",
      label: "Storage",
      icon: Database,
      to: storageHref(projectId),
      end: false,
    },
    {
      key: "settings",
      label: "Settings",
      icon: Settings,
      to: settingsHref(projectId),
      end: true,
    },
  ] as const;
}

function NavItem({
  to,
  icon: Icon,
  label,
  end,
}: {
  to: string;
  icon: typeof LayoutGrid;
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
        <Icon className="size-4 shrink-0" />
        {label}
      </NavLink>
    </li>
  );
}

function NavSection({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <p className="px-2.5 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </p>
      <ul className="space-y-0.5">{children}</ul>
    </div>
  );
}

export function AppSidebar({ projectId }: AppSidebarProps) {
  return (
    <nav className="flex flex-1 flex-col gap-6 overflow-y-auto p-2">
      <NavSection title="Workspace">
        {GLOBAL_NAV.map(({ key, ...item }) => (
          <NavItem key={key} {...item} />
        ))}
      </NavSection>

      <NavSection title="Bindings">
        {BINDINGS_NAV.map(({ key, ...item }) => (
          <NavItem key={key} {...item} end />
        ))}
      </NavSection>

      {projectId ? (
        <div>
          <p className="px-2.5 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
            Project
          </p>
          <p className="truncate px-2.5 pb-2 font-mono text-xs text-muted-foreground">
            {projectId}
          </p>
          <ul className="space-y-0.5">
            {projectNav(projectId).map(({ key, ...item }) => (
              <NavItem key={key} {...item} />
            ))}
          </ul>
        </div>
      ) : null}
    </nav>
  );
}

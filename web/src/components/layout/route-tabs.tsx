import { NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";

export interface RouteTab {
  label: string;
  to: string;
  end?: boolean;
}

export function RouteTabs({ tabs, ariaLabel = "Tabs" }: { tabs: RouteTab[]; ariaLabel?: string }) {
  return (
    <div className="border-b border-border">
      <nav
        className="-mb-px flex gap-0 overflow-x-auto"
        aria-label={ariaLabel}
      >
        {tabs.map((tab) => (
          <NavLink
            key={tab.to}
            to={tab.to}
            end={tab.end}
            className={({ isActive }) =>
              cn(
                "whitespace-nowrap border-b-2 px-4 py-2.5 text-sm font-medium transition-colors",
                isActive
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:border-border hover:text-foreground",
              )
            }
          >
            {tab.label}
          </NavLink>
        ))}
      </nav>
    </div>
  );
}

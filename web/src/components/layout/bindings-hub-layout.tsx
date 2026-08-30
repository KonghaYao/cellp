import { Outlet } from "react-router-dom";
import { RouteTabs } from "@/components/layout/route-tabs";
import {
  bindingsCronHref,
  bindingsD1Href,
  bindingsKvHref,
  bindingsQueuesHref,
  bindingsR2Href,
  bindingsWorkflowsHref,
} from "@/lib/routes";

const TABS = [
  { label: "D1", to: bindingsD1Href(), end: true },
  { label: "KV", to: bindingsKvHref(), end: true },
  { label: "Queues", to: bindingsQueuesHref(), end: true },
  { label: "Workflows", to: bindingsWorkflowsHref(), end: true },
  { label: "R2", to: bindingsR2Href(), end: true },
  { label: "Cron", to: bindingsCronHref(), end: true },
] as const;

export function BindingsHubLayout() {
  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="text-heading-24 font-semibold tracking-tight">Bindings</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          Fleet-wide registry of D1, KV, Queue, Workflow, R2, and Cron instances across
          all projects.
        </p>
      </div>

      <RouteTabs tabs={[...TABS]} ariaLabel="Binding types" />

      <Outlet />
    </div>
  );
}

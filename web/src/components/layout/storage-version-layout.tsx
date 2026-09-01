import { useMemo } from "react";
import { Outlet, useLocation, useParams } from "react-router-dom";
import { VersionSwitcher } from "@/components/database/version-switcher";
import { RouteTabs } from "@/components/layout/route-tabs";
import {
  storageBrowserHref,
  storageKvHref,
  storageQueuesHref,
  storageWorkflowsHref,
  versionHrefForStoragePathname,
} from "@/lib/routes";

export function StorageVersionLayout() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const location = useLocation();

  const versionHref = useMemo(
    () => (projectId: string, versionId: string) =>
      versionHrefForStoragePathname(projectId, versionId, location.pathname),
    [location.pathname],
  );

  const tabs = useMemo(
    () => [
      { label: "Data", to: storageBrowserHref(id, vid), end: true },
      { label: "KV", to: storageKvHref(id, vid), end: true },
      { label: "Queues", to: storageQueuesHref(id, vid), end: true },
      { label: "Workflows", to: storageWorkflowsHref(id, vid), end: true },
    ],
    [id, vid],
  );

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-label-13 text-muted-foreground">Storage</p>
          <h1 className="font-mono text-heading-24 font-semibold tracking-tight">
            {vid}
          </h1>
        </div>
        <VersionSwitcher
          projectId={id}
          versionId={vid}
          versionHref={versionHref}
          className="w-full sm:w-72"
        />
      </div>

      <RouteTabs tabs={tabs} ariaLabel="Storage bindings" />

      <Outlet />
    </div>
  );
}

import { BrowserRouter, Navigate, Route, Routes, useParams, useSearchParams } from "react-router-dom";
import { AppShell } from "@/components/layout/app-shell";
import { ProjectLayout } from "@/components/layout/project-layout";
import { DatabasePage } from "@/pages/DatabasePage";
import { DeploymentsPage } from "@/pages/DeploymentsPage";
import { KvPage } from "@/pages/KvPage";
import { PlatformPage } from "@/pages/PlatformPage";
import { ProjectInspectPage } from "@/pages/ProjectInspectPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { QueuesPage } from "@/pages/QueuesPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { StoragePage } from "@/pages/StoragePage";
import { VersionPage } from "@/pages/VersionPage";
import { WorkflowsPage } from "@/pages/WorkflowsPage";
import { bindingsHref, storageBrowserHref } from "@/lib/routes";
import { StorageVersionLayout } from "@/components/layout/storage-version-layout";
import { BindingsHubLayout } from "@/components/layout/bindings-hub-layout";
import { GlobalBindingListPage } from "@/pages/bindings/GlobalBindingListPage";

function LegacyDatabaseRedirect() {
  const path = window.location.pathname;
  const match = path.match(/^\/projects\/([^/]+)\/versions\/([^/]+)\/database/);
  if (match) {
    return <Navigate to={storageBrowserHref(match[1], match[2])} replace />;
  }
  return <Navigate to="/" replace />;
}

function BindingsRedirect() {
  const { id = "" } = useParams<{ id: string }>();
  const [params] = useSearchParams();
  const version = params.get("version");
  return <Navigate to={bindingsHref(id, version ?? undefined)} replace />;
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<ProjectsPage />} />
          <Route path="/platform" element={<PlatformPage />} />
          <Route path="/bindings" element={<BindingsHubLayout />}>
            <Route index element={<Navigate to="d1" replace />} />
            <Route
              path="d1"
              element={
                <GlobalBindingListPage
                  kind="d1"
                  emptyDescription="No ready deployments declare a D1 database binding."
                />
              }
            />
            <Route
              path="kv"
              element={
                <GlobalBindingListPage
                  kind="kv"
                  emptyDescription="No ready deployments declare a KV namespace binding."
                />
              }
            />
            <Route
              path="queues"
              element={
                <GlobalBindingListPage
                  kind="queues"
                  emptyDescription="No ready deployments declare a Queue binding."
                />
              }
            />
            <Route
              path="workflows"
              element={
                <GlobalBindingListPage
                  kind="workflows"
                  emptyDescription="No ready deployments declare a Workflow binding."
                />
              }
            />
            <Route
              path="r2"
              element={
                <GlobalBindingListPage
                  kind="r2"
                  emptyDescription="No ready deployments declare an R2 bucket binding."
                />
              }
            />
            <Route
              path="cron"
              element={
                <GlobalBindingListPage
                  kind="cron"
                  emptyDescription="No ready deployments declare a Cron trigger."
                />
              }
            />
          </Route>
          <Route path="/projects/:id" element={<ProjectLayout />}>
            <Route index element={<ProjectOverviewPage />} />
            <Route path="inspect" element={<ProjectInspectPage />} />
            <Route path="deployments" element={<DeploymentsPage />} />
            <Route path="bindings" element={<BindingsRedirect />} />
            <Route path="storage" element={<StoragePage />} />
            <Route path="storage/:vid" element={<StorageVersionLayout />}>
              <Route index element={<Navigate to="browser" replace />} />
              <Route path="browser" element={<DatabasePage />} />
              <Route path="kv" element={<KvPage />} />
              <Route path="queues" element={<QueuesPage />} />
              <Route path="workflows" element={<WorkflowsPage />} />
            </Route>
            <Route path="settings" element={<SettingsPage />} />
            <Route path="versions/:vid" element={<VersionPage />} />
            <Route
              path="versions/:vid/database"
              element={<LegacyDatabaseRedirect />}
            />
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

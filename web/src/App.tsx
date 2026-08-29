import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "@/components/layout/app-shell";
import { ProjectLayout } from "@/components/layout/project-layout";
import { DatabasePage } from "@/pages/DatabasePage";
import { DeploymentsPage } from "@/pages/DeploymentsPage";
import { ProjectOverviewPage } from "@/pages/ProjectOverviewPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { StoragePage } from "@/pages/StoragePage";
import { VersionPage } from "@/pages/VersionPage";
import { storageBrowserHref } from "@/lib/routes";

function LegacyDatabaseRedirect() {
  const path = window.location.pathname;
  const match = path.match(/^\/projects\/([^/]+)\/versions\/([^/]+)\/database/);
  if (match) {
    return <Navigate to={storageBrowserHref(match[1], match[2])} replace />;
  }
  return <Navigate to="/" replace />;
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<ProjectsPage />} />
          <Route path="/projects/:id" element={<ProjectLayout />}>
            <Route index element={<ProjectOverviewPage />} />
            <Route path="deployments" element={<DeploymentsPage />} />
            <Route path="storage" element={<StoragePage />} />
            <Route path="storage/:vid/browser" element={<DatabasePage />} />
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

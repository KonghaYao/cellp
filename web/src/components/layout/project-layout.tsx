import { Outlet } from "react-router-dom";

/** Nested layout under /projects/:id — sidebar comes from AppShell. */
export function ProjectLayout() {
  return <Outlet />;
}

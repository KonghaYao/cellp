/**
 * 用户心流：从项目列表进入部署与 Storage 的导航链路（与 operator-journey 文档一致）
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes } from "react-router-dom";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { DeploymentsPage } from "@/pages/DeploymentsPage";
import { ProjectLayout } from "@/components/layout/project-layout";
import { ProjectOverviewPage } from "@/pages/ProjectOverviewPage";
import { AppShell } from "@/components/layout/app-shell";
import {
  deploymentsHref,
  platformHref,
  storageBrowserHref,
  storageHref,
} from "@/lib/routes";
import {
  makeProjectDetail,
  makeProjectSummary,
  makeRootVersion,
} from "@/test/fixtures/versions";
import { renderWithRouter } from "@/test/test-utils";

const mockListProjects = vi.fn();
const mockGetProject = vi.fn();
const mockListVersions = vi.fn();
const mockCheckDatabaseAvailability = vi.fn();
const mockHealthCheck = vi.fn();

vi.mock("@/lib/cellp-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/cellp-api")>();
  return {
    ...actual,
    listProjects: (...args: Parameters<typeof actual.listProjects>) =>
      mockListProjects(...args),
    getProject: (...args: Parameters<typeof actual.getProject>) =>
      mockGetProject(...args),
    listVersions: (...args: Parameters<typeof actual.listVersions>) =>
      mockListVersions(...args),
    checkDatabaseAvailability: (
      ...args: Parameters<typeof actual.checkDatabaseAvailability>
    ) => mockCheckDatabaseAvailability(...args),
    healthCheck: (...args: Parameters<typeof actual.healthCheck>) =>
      mockHealthCheck(...args),
  };
});

describe("心流：运营导航链路", () => {
  beforeEach(() => {
    mockHealthCheck.mockResolvedValue({ status: "ok" });
    mockCheckDatabaseAvailability.mockResolvedValue({ available: false });
    mockListProjects.mockResolvedValue({
      projects: [makeProjectSummary()],
      next_cursor: null,
    });
    mockGetProject.mockResolvedValue(makeProjectDetail());
    mockListVersions.mockResolvedValue({
      versions: [makeRootVersion()],
      next_cursor: null,
    });
  });

  it("路由 helper 与文档中的 Storage / Platform 路径一致", () => {
    expect(deploymentsHref("demo-app")).toBe("/projects/demo-app/deployments");
    expect(storageHref("demo-app")).toBe("/projects/demo-app/storage");
    expect(storageBrowserHref("demo-app", "v1")).toBe(
      "/projects/demo-app/storage/v1/browser",
    );
    expect(platformHref()).toBe("/platform");
  });

  it("Projects → 项目 → Deployments 列表", async () => {
    const user = userEvent.setup();

    renderWithRouter(
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<ProjectsPage />} />
          <Route path="/projects/:id" element={<ProjectLayout />}>
            <Route index element={<ProjectOverviewPage />} />
            <Route path="deployments" element={<DeploymentsPage />} />
          </Route>
        </Route>
      </Routes>,
      { initialEntries: ["/"] },
    );

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "demo-app" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("link", { name: "demo-app" }));

    await waitFor(() => {
      expect(screen.getAllByRole("link", { name: "Deployments" }).length).toBeGreaterThan(0);
    });

    await user.click(screen.getAllByRole("link", { name: "Deployments" })[0]!);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Versions", level: 1 }),
      ).toBeInTheDocument();
    });
    expect(screen.getAllByRole("link", { name: "v1" }).length).toBeGreaterThan(0);
  });
});

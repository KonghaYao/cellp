import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { Route, Routes } from "react-router-dom";
import { ProjectInspectPage } from "@/pages/ProjectInspectPage";
import { renderWithRouter } from "@/test/test-utils";
import { makeVersion } from "@/test/fixtures/versions";

const mockGetProject = vi.fn();
const mockListVersions = vi.fn();
const mockGetRuntimeRoutes = vi.fn();
const mockGetHealthDeep = vi.fn();
const mockFetchMetrics = vi.fn();
const mockGetBindings = vi.fn();

vi.mock("@/lib/cellp-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/cellp-api")>();
  return {
    ...actual,
    getProject: (...args: Parameters<typeof actual.getProject>) =>
      mockGetProject(...args),
    listVersions: (...args: Parameters<typeof actual.listVersions>) =>
      mockListVersions(...args),
    getRuntimeRoutes: (...args: Parameters<typeof actual.getRuntimeRoutes>) =>
      mockGetRuntimeRoutes(...args),
    getHealthDeep: (...args: Parameters<typeof actual.getHealthDeep>) =>
      mockGetHealthDeep(...args),
    fetchMetricsGauges: (...args: Parameters<typeof actual.fetchMetricsGauges>) =>
      mockFetchMetrics(...args),
    getBindings: (...args: Parameters<typeof actual.getBindings>) =>
      mockGetBindings(...args),
  };
});

describe("心流：项目 Inspect 页", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetProject.mockResolvedValue({
      id: "demo-app",
      prod_version_id: "v1",
      prod_url: null,
      git_remote: null,
      created_at: "",
      version_count: 1,
    });
    mockListVersions.mockResolvedValue({
      versions: [makeVersion({ id: "v1", status: "ready", project_id: "demo-app" })],
      next_cursor: null,
    });
    mockGetRuntimeRoutes.mockResolvedValue({
      summary: { active_routes: 1, healthy: 1, unhealthy: 0 },
      routes: [
        {
          project_id: "demo-app",
          version_id: "v1",
          active: true,
          upstream: "http://127.0.0.1:8792",
          version_status: "ready",
          celld_health: "ok",
        },
      ],
    });
    mockGetHealthDeep.mockResolvedValue({ status: "ok", checks: {} });
    mockFetchMetrics.mockResolvedValue({ cellp_pending_jobs: 0 });
    mockGetBindings.mockResolvedValue({
      d1: [{ binding: "DB", database_name: "main" }],
      kv: [],
      queues: [],
      workflows: [],
      r2: [],
      crons: [],
    });
  });

  it("展示 Inspect 标题与 runtime 路由表", async () => {
    renderWithRouter(
      <Routes>
        <Route path="/projects/:id/inspect" element={<ProjectInspectPage />} />
      </Routes>,
      { initialEntries: ["/projects/demo-app/inspect"] },
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Inspect" })).toBeInTheDocument();
    });
    expect(screen.getByRole("link", { name: "v1" })).toBeInTheDocument();
    expect(screen.getByText(/Ready versions/i)).toBeInTheDocument();
  });
});

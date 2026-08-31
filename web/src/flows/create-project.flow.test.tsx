/**
 * 用户心流：注册空项目 → 进入概览（Dashboard 第一步，部署仍走 CLI/CI）
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes } from "react-router-dom";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { makeProjectDetail } from "@/test/fixtures/versions";
import { renderWithRouter } from "@/test/test-utils";

const mockListProjects = vi.fn();
const mockCreateProject = vi.fn();

vi.mock("@/lib/cellp-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/cellp-api")>();
  return {
    ...actual,
    listProjects: (...args: Parameters<typeof actual.listProjects>) =>
      mockListProjects(...args),
    createProject: (...args: Parameters<typeof actual.createProject>) =>
      mockCreateProject(...args),
  };
});

describe("心流：创建项目", () => {
  beforeEach(() => {
    mockListProjects.mockResolvedValue({ projects: [], next_cursor: null });
    mockCreateProject.mockResolvedValue(
      makeProjectDetail({ id: "my-shop", prod_version_id: null, version_count: 0 }),
    );
  });

  it("填写 id 后创建并导航到项目概览", async () => {
    const user = userEvent.setup();

    renderWithRouter(
      <Routes>
        <Route path="/" element={<ProjectsPage />} />
        <Route
          path="/projects/:id"
          element={<h1 data-testid="project-overview">Overview</h1>}
        />
      </Routes>,
      { initialEntries: ["/"], routes: undefined },
    );

    // renderWithRouter wraps ui only when routes undefined - fix: pass routes as wrapper content
    // Actually my renderWithRouter when routes is undefined wraps `ui` - but I passed Routes as ui. Good.

    await waitFor(() => {
      expect(screen.getByText("No projects yet")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "New project" }));
    await user.type(screen.getByLabelText(/Project id/i), "my-shop");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateProject).toHaveBeenCalledWith({ id: "my-shop" });
    });

    await waitFor(() => {
      expect(screen.getByTestId("project-overview")).toBeInTheDocument();
    });
  });
});

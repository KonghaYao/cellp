/**
 * 用户心流：项目概览 Operator checklist（与 operator-journey 文档一致）
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { Route, Routes } from "react-router-dom";
import {
  OperatorChecklist,
  findPromoteCandidate,
} from "@/components/operator-checklist";
import { ProjectOverviewPage } from "@/pages/ProjectOverviewPage";
import { ProjectLayout } from "@/components/layout/project-layout";
import { AppShell } from "@/components/layout/app-shell";
import {
  makeProjectDetail,
  makeRootVersion,
  makeVersion,
} from "@/test/fixtures/versions";
import { renderWithRouter } from "@/test/test-utils";

const mockGetProject = vi.fn();
const mockListVersions = vi.fn();
const mockCheckDatabaseAvailability = vi.fn();
const mockHealthCheck = vi.fn();

vi.mock("@/lib/cellp-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/cellp-api")>();
  return {
    ...actual,
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

describe("心流：Operator checklist", () => {
  beforeEach(() => {
    mockHealthCheck.mockResolvedValue({ status: "ok" });
    mockCheckDatabaseAvailability.mockResolvedValue({ available: false });
  });

  it("findPromoteCandidate 返回首个 ready 且非 prod 的 version", () => {
    const versions = [
      makeRootVersion({ id: "v1", status: "ready" }),
      makeVersion({ id: "v-preview", status: "ready" }),
    ];
    expect(findPromoteCandidate(versions, "v1")?.id).toBe("v-preview");
    expect(
      findPromoteCandidate([makeVersion({ id: "v-preview", status: "ready" })], "v-preview"),
    ).toBeNull();
  });

  it("无 version 时强调先 deploy", () => {
    renderWithRouter(
      <OperatorChecklist
        projectId="my-shop"
        prodVersionId={null}
        versions={[]}
      />,
    );

    expect(screen.getByTestId("operator-checklist")).toBeInTheDocument();
    expect(screen.getByTestId("operator-checklist-deploy-first")).toHaveTextContent(
      /Deploy first/i,
    );
    expect(screen.getByText(/cellp dev --project my-shop/i)).toBeInTheDocument();
  });

  it("有 ready 非 prod 时展示 Promote 入口", () => {
    renderWithRouter(
      <OperatorChecklist
        projectId="demo-app"
        prodVersionId="v1"
        versions={[
          makeRootVersion({ id: "v1", status: "ready" }),
          makeVersion({ id: "v-preview", status: "ready" }),
        ]}
      />,
    );

    const link = screen.getByTestId("operator-checklist-promote-link");
    expect(link).toHaveAttribute(
      "href",
      "/projects/demo-app/versions/v-preview",
    );
    expect(link).toHaveTextContent(/Promote v-preview/i);
  });

  it("ProjectOverviewPage 挂载 checklist", async () => {
    mockGetProject.mockResolvedValue(
      makeProjectDetail({ prod_version_id: "v1" }),
    );
    mockListVersions.mockResolvedValue({
      versions: [makeRootVersion()],
      next_cursor: null,
    });

    renderWithRouter(
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/projects/:id" element={<ProjectLayout />}>
            <Route index element={<ProjectOverviewPage />} />
          </Route>
        </Route>
      </Routes>,
      { initialEntries: ["/projects/demo-app"] },
    );

    await waitFor(() => {
      expect(screen.getByTestId("operator-checklist")).toBeInTheDocument();
    });
  });
});

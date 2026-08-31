/**
 * 用户心流：PR 预览分支 → 看到快照说明 → 确认 Promote（非 merge）
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { VersionDetailView } from "@/components/version-detail-view";
import { VersionActions } from "@/components/version-actions";
import { makeVersion } from "@/test/fixtures/versions";
import { renderWithRouter } from "@/test/test-utils";

const mockCheckDatabaseAvailability = vi.fn();
const mockGetVersionEnv = vi.fn();
const mockPromoteVersion = vi.fn();

vi.mock("@/lib/cellp-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/cellp-api")>();
  return {
    ...actual,
    checkDatabaseAvailability: (
      ...args: Parameters<typeof actual.checkDatabaseAvailability>
    ) => mockCheckDatabaseAvailability(...args),
    getVersionEnv: (...args: Parameters<typeof actual.getVersionEnv>) =>
      mockGetVersionEnv(...args),
    promoteVersion: (...args: Parameters<typeof actual.promoteVersion>) =>
      mockPromoteVersion(...args),
  };
});

describe("心流：预览分支与 Promote", () => {
  beforeEach(() => {
    mockCheckDatabaseAvailability.mockResolvedValue({ available: false });
    mockGetVersionEnv.mockResolvedValue([]);
    mockPromoteVersion.mockResolvedValue({
      prod_version_id: "v-preview",
      prod_url: "http://127.0.0.1:8787/demo-app/",
    });
  });

  it("子 version 展示 preview 快照说明（ISSUE-03 心智）", async () => {
    const version = makeVersion({
      id: "v-preview",
      parent_version_id: "v1",
      status: "ready",
    });

    renderWithRouter(
      <VersionDetailView
        projectId="demo-app"
        versionId="v-preview"
        initialVersion={version}
        prodVersionId="v1"
        prodUrl="http://127.0.0.1:8787/demo-app/"
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("preview-snapshot-notice")).toBeInTheDocument();
    });
    expect(screen.getByText(/Promote does not merge/i)).toBeInTheDocument();
    expect(screen.getByText("Preview branch")).toBeInTheDocument();
  });

  it("根 version 不展示快照说明", async () => {
    const version = makeVersion({
      id: "v1",
      parent_version_id: null,
    });

    renderWithRouter(
      <VersionDetailView
        projectId="demo-app"
        versionId="v1"
        initialVersion={version}
        prodVersionId="v1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "v1" })).toBeInTheDocument();
    });
    expect(screen.queryByTestId("preview-snapshot-notice")).not.toBeInTheDocument();
  });

  it("Promote 需二次确认并调用 API", async () => {
    const user = userEvent.setup();
    const onComplete = vi.fn();

    renderWithRouter(
      <VersionActions
        projectId="demo-app"
        versionId="v-preview"
        status="ready"
        isProd={false}
        previewUrl="http://127.0.0.1:8787/demo-app/v-preview/"
        onComplete={onComplete}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Promote to prod" }));
    expect(
      screen.getByRole("heading", { name: "Promote to production?" }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Promote" }));

    await waitFor(() => {
      expect(mockPromoteVersion).toHaveBeenCalledWith("demo-app", "v-preview");
    });
    await waitFor(() => {
      expect(onComplete).toHaveBeenCalled();
    });
  });
});

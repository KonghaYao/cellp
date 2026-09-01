import type { ProjectDetail, ProjectSummary, Version } from "@/lib/cellp-api";

export function makeVersion(overrides: Partial<Version> = {}): Version {
  return {
    id: "v-preview",
    project_id: "demo-app",
    parent_version_id: "v1",
    status: "ready",
    git_ref: "feature/checkout",
    git_sha: "abc1234567890",
    data_branch: "demo-app/v-preview",
    preview_url: "http://v-preview.demo-app.ingress.local/",
    created_at: "2026-01-02T00:30:00.000Z",
    updated_at: "2026-01-02T01:00:00.000Z",
    ready_at: "2026-01-02T01:00:00.000Z",
    error: null,
    ...overrides,
  };
}

export function makeRootVersion(overrides: Partial<Version> = {}): Version {
  return makeVersion({
    id: "v1",
    parent_version_id: null,
    git_ref: "main",
    ...overrides,
  });
}

export function makeProjectSummary(
  overrides: Partial<ProjectSummary> = {},
): ProjectSummary {
  return {
    id: "demo-app",
    prod_version_id: "v1",
    version_count: 3,
    created_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

export function makeProjectDetail(
  overrides: Partial<ProjectDetail> = {},
): ProjectDetail {
  return {
    id: "demo-app",
    prod_version_id: "v1",
    prod_url: "http://demo-app.ingress.local/",
    git_remote: null,
    created_at: "2026-01-01T00:00:00.000Z",
    version_count: 3,
    ...overrides,
  };
}

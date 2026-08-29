/**
 * cellp REST API client — consumes cellpd :8790/v1 only (TP-UI-4).
 * Generated from DESIGN.md §9 + mock-platform contract.
 */

const DEFAULT_API_URL = "http://127.0.0.1:8790";

export class CellpApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly body?: unknown,
  ) {
    super(message);
    this.name = "CellpApiError";
  }
}

export interface ProjectSummary {
  id: string;
  prod_version_id: string | null;
  version_count: number;
  created_at: string;
}

export interface Version {
  id: string;
  project_id: string;
  parent_version_id: string | null;
  status: string;
  git_ref: string;
  git_sha: string;
  artifact_uri?: string | null;
  artifact_digest?: string | null;
  data_branch: string;
  preview_url: string;
  ttl?: string | null;
  created_at: string;
  updated_at: string;
  ready_at: string | null;
  error: string | null;
}

export interface ProjectDetail {
  id: string;
  prod_version_id: string | null;
  git_remote: string | null;
  created_at: string;
  version_count?: number;
  /** Legacy embed; prefer listVersions with cursor pagination. */
  versions?: Version[];
}

export interface PaginatedProjects {
  projects: ProjectSummary[];
  next_cursor: string | null;
}

export interface PaginatedVersions {
  versions: Version[];
  next_cursor: string | null;
}

export interface ListProjectsOptions {
  cursor?: string | null;
  limit?: number;
}

export interface ListVersionsOptions {
  cursor?: string | null;
  limit?: number;
  status?: string;
}

/** Default page size for cursor lists (override via VITE_CELLP_PAGE_SIZE). */
export const LIST_PAGE_SIZE =
  Number(import.meta.env.VITE_CELLP_PAGE_SIZE) || 50;

export interface PromoteResponse {
  status: string;
  prod_version_id: string;
}

export interface DestroyResponse {
  status: string;
}

export type DatabaseBranchMethod = "d1_branch" | "offshoot_export";

export interface DatabaseMetadata {
  database_id: string;
  database_name: string;
  data_branch: string;
  parent_version_id: string | null;
  branch_method: DatabaseBranchMethod;
  status: string;
}

export interface DatabaseTable {
  name: string;
  type: string;
  row_count: number;
}

export interface DatabaseTablesResponse {
  tables: DatabaseTable[];
}

export interface DatabaseColumn {
  name: string;
  type: string;
}

export interface DatabaseRowsResponse {
  columns: DatabaseColumn[];
  rows: Record<string, unknown>[];
  total: number;
  limit: number;
  offset: number;
}

export interface GetDatabaseTableRowsOptions {
  limit?: number;
  offset?: number;
}

export interface DatabaseQueryRequest {
  sql: string;
}

export interface DatabaseQueryResponse {
  columns: DatabaseColumn[];
  rows: Record<string, unknown>[];
  duration_ms: number;
  rows_affected: number | null;
}

export interface CreateVersionRequest {
  id?: string;
  parent_version_id?: string | null;
  git_ref?: string;
  git_sha?: string;
  artifact_digest?: string;
}

export interface CreateVersionResponse {
  id: string;
  status: string;
  project_id: string;
}

function apiBase(): string {
  const url = import.meta.env.VITE_CELLP_API_URL ?? DEFAULT_API_URL;
  return url.replace(/\/$/, "");
}

function adminToken(): string {
  return import.meta.env.VITE_CELLP_ADMIN_TOKEN ?? "";
}

function deployToken(): string {
  return import.meta.env.VITE_CELLP_DEPLOY_TOKEN ?? adminToken();
}

async function request<T>(
  path: string,
  init: RequestInit = {},
  token?: string,
): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }
  const authToken = token ?? adminToken();
  if (authToken && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${authToken}`);
  }

  const res = await fetch(`${apiBase()}${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });

  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }

  if (!res.ok) {
    const msg =
      typeof body === "object" &&
      body !== null &&
      "error" in body &&
      typeof (body as { error: unknown }).error === "string"
        ? (body as { error: string }).error
        : `API ${res.status}`;
    throw new CellpApiError(msg, res.status, body);
  }

  return body as T;
}

function buildQuery(params: Record<string, string | undefined>): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value != null && value !== "") qs.set(key, value);
  }
  const s = qs.toString();
  return s ? `?${s}` : "";
}

export async function listProjects(
  options: ListProjectsOptions = {},
): Promise<PaginatedProjects> {
  const limit = options.limit ?? LIST_PAGE_SIZE;
  const query = buildQuery({
    limit: String(limit),
    cursor: options.cursor ?? undefined,
  });
  const data = await request<PaginatedProjects>(`/v1/projects${query}`);
  return {
    projects: data.projects ?? [],
    next_cursor: data.next_cursor ?? null,
  };
}

export async function listVersions(
  projectId: string,
  options: ListVersionsOptions = {},
): Promise<PaginatedVersions> {
  const limit = options.limit ?? LIST_PAGE_SIZE;
  const query = buildQuery({
    limit: String(limit),
    cursor: options.cursor ?? undefined,
    status: options.status,
  });
  const data = await request<PaginatedVersions>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions${query}`,
  );
  return {
    versions: data.versions ?? [],
    next_cursor: data.next_cursor ?? null,
  };
}

export async function getProject(id: string): Promise<ProjectDetail> {
  return request<ProjectDetail>(`/v1/projects/${encodeURIComponent(id)}`);
}

export async function getVersion(
  projectId: string,
  versionId: string,
): Promise<Version> {
  return request<Version>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}`,
  );
}

export async function promoteVersion(
  projectId: string,
  versionId: string,
): Promise<ProjectDetail> {
  await request<PromoteResponse>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/promote`,
    { method: "POST" },
  );
  return getProject(projectId);
}

export async function destroyVersion(
  projectId: string,
  versionId: string,
): Promise<ProjectDetail> {
  await request<DestroyResponse>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}`,
    { method: "DELETE" },
  );
  return getProject(projectId);
}

export async function healthCheck(): Promise<{ status: string }> {
  return request<{ status: string }>("/v1/health");
}

/** Client-side fetch for polling (same auth as server). */
export async function fetchVersionClient(
  projectId: string,
  versionId: string,
): Promise<Version> {
  return getVersion(projectId, versionId);
}

export async function getDatabase(
  projectId: string,
  versionId: string,
): Promise<DatabaseMetadata> {
  return request<DatabaseMetadata>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/database`,
  );
}

export async function listDatabaseTables(
  projectId: string,
  versionId: string,
): Promise<DatabaseTablesResponse> {
  const data = await request<DatabaseTablesResponse>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/database/tables`,
  );
  return { tables: data.tables ?? [] };
}

export async function getDatabaseTableRows(
  projectId: string,
  versionId: string,
  tableName: string,
  options: GetDatabaseTableRowsOptions = {},
): Promise<DatabaseRowsResponse> {
  const limit = options.limit ?? LIST_PAGE_SIZE;
  const offset = options.offset ?? 0;
  const query = buildQuery({
    limit: String(limit),
    offset: String(offset),
  });
  return request<DatabaseRowsResponse>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/database/tables/${encodeURIComponent(tableName)}/rows${query}`,
  );
}

export async function queryDatabase(
  projectId: string,
  versionId: string,
  sql: string,
): Promise<DatabaseQueryResponse> {
  return request<DatabaseQueryResponse>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/database/query`,
    {
      method: "POST",
      body: JSON.stringify({ sql } satisfies DatabaseQueryRequest),
    },
  );
}

/** Fork a data branch (requires deploy token; falls back to admin token in dev). */
export async function createVersion(
  projectId: string,
  body: CreateVersionRequest,
): Promise<CreateVersionResponse> {
  return request<CreateVersionResponse>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions`,
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    deployToken(),
  );
}

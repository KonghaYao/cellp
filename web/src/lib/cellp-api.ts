/**
 * cellp REST API client — consumes cellpd :8790/v1 only (TP-UI-4).
 * Generated from DESIGN.md §9 + mock-platform contract.
 */

import { gatewayBase } from "@/lib/format";

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
  pinned?: boolean;
  last_access_at?: string | null;
  error: string | null;
}

export interface ProjectDetail {
  id: string;
  prod_version_id: string | null;
  /** Production gateway URL from API ({GATEWAY_URL}/{projectID}/). */
  prod_url?: string | null;
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
  q?: string;
}

/** @deprecated Server-side ?q= search replaces client-side pagination cap. */
export const PROJECT_SEARCH_MAX_PAGES = 10;

export function projectMatchesQuery(
  project: ProjectSummary,
  query: string,
): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return project.id.toLowerCase().includes(q);
}

export interface ListVersionsOptions {
  cursor?: string | null;
  limit?: number;
  status?: string;
}

/** Default page size for cursor lists (override via VITE_CELLP_PAGE_SIZE). */
export const LIST_PAGE_SIZE =
  Number(import.meta.env.VITE_CELLP_PAGE_SIZE) || 50;

/** GET …/bindings — DESIGN §8.4 / OpenAPI `Bindings` (six arrays). */
export interface Bindings {
  d1: BindingsD1[];
  kv: BindingsKV[];
  queues: BindingsQueue[];
  workflows: BindingsWorkflow[];
  r2: BindingsR2[];
  crons: string[];
}

export interface BindingsD1 {
  binding: string;
  database_name: string;
  database_id?: string;
}

export interface BindingsKV {
  binding: string;
  /** wrangler `kv_namespaces[].id` — path `{ns}` verbatim. */
  id: string;
}

export interface BindingsQueue {
  name: string;
  binding?: string;
  consumer: boolean;
  dead_letter_queue?: string;
}

export interface BindingsWorkflow {
  binding: string;
  name: string;
  class_name: string;
}

export interface BindingsR2 {
  binding: string;
  bucket_name: string;
}

export function emptyBindings(): Bindings {
  return {
    d1: [],
    kv: [],
    queues: [],
    workflows: [],
    r2: [],
    crons: [],
  };
}

export function normalizeBindings(
  data: Partial<Bindings> | null | undefined,
): Bindings {
  return {
    d1: data?.d1 ?? [],
    kv: data?.kv ?? [],
    queues: data?.queues ?? [],
    workflows: data?.workflows ?? [],
    r2: data?.r2 ?? [],
    crons: data?.crons ?? [],
  };
}

export function hasAnyBindings(bindings: Bindings): boolean {
  return (
    bindings.d1.length +
      bindings.kv.length +
      bindings.queues.length +
      bindings.workflows.length +
      bindings.r2.length +
      bindings.crons.length >
    0
  );
}

export interface KvNamespace {
  id: string;
  binding: string;
}

export interface KvNamespacesResponse {
  namespaces: KvNamespace[];
}

export interface KvKey {
  name: string;
  expiration?: number;
  metadata?: unknown;
}

export interface KvListResult {
  keys: KvKey[];
  cursor?: string;
}

export interface ListKvKeysOptions {
  prefix?: string;
  cursor?: string;
  limit?: number;
}

export interface KvValue {
  key: string;
  value: string;
  encoding: "utf-8" | "base64" | string;
}

export interface KvPutBody {
  value: string;
  ttl?: number;
  metadata?: Record<string, unknown> | string;
  base64?: boolean;
  binary?: boolean;
}

export interface KvInfo {
  keys: number;
  bytes: number;
  stored: number;
  /** celld `kv info` live count; treat as `keys` when omitted. */
  live?: number;
}

export interface QueueListItem {
  name: string;
}

export interface QueueListResponse {
  queues: QueueListItem[];
}

export interface QueuePeekMessage {
  id?: string;
  bodyBase64?: string;
  contentType?: string;
  [key: string]: unknown;
}

export interface QueuePeekResult {
  messages?: QueuePeekMessage[];
  [key: string]: unknown;
}

export interface QueueInfo {
  name?: string;
  paused?: boolean;
  pending?: number;
  backlogCount?: number;
  backlogBytes?: number;
  stored?: number;
  oldestMessageTimestamp?: number | string | null;
  [key: string]: unknown;
}

export interface WorkflowListItem {
  binding: string;
  workflow_name: string;
  class_name: string;
}

export interface WorkflowListResponse {
  workflows: WorkflowListItem[];
}

export interface WorkflowInstance {
  scope?: string;
  class?: string;
  id: string;
  reserved?: boolean;
  status?: string;
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowInstances {
  workflow_name: string;
  binding: string;
  script_name?: string;
  filter?: "workflow" | "script" | string;
  limitation?: string | null;
  wrangler_workflows?: string[];
  instances: WorkflowInstance[];
}

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
  const configured = import.meta.env.VITE_CELLP_API_URL as string | undefined;
  if (configured === "") return "";
  if (configured) return configured.replace(/\/$/, "");
  if (import.meta.env.DEV) return "";
  return DEFAULT_API_URL.replace(/\/$/, "");
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

  if (res.status === 204) {
    return undefined as T;
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

function versionRoot(projectId: string, versionId: string): string {
  return `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}`;
}

export interface CreateProjectInput {
  id: string;
  git_remote?: string;
}

export async function createProject(
  input: CreateProjectInput,
): Promise<ProjectDetail> {
  const body: { id: string; git_remote?: string } = { id: input.id };
  if (input.git_remote) body.git_remote = input.git_remote;
  return request<ProjectDetail>("/v1/projects", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export async function listProjects(
  options: ListProjectsOptions = {},
): Promise<PaginatedProjects> {
  const limit = options.limit ?? LIST_PAGE_SIZE;
  const query = buildQuery({
    limit: String(limit),
    cursor: options.cursor ?? undefined,
    q: options.q,
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

export async function archiveVersion(
  projectId: string,
  versionId: string,
): Promise<Version> {
  await request<{ status: string }>(
    `${versionRoot(projectId, versionId)}/archive`,
    { method: "POST" },
  );
  return getVersion(projectId, versionId);
}

export async function wakeVersion(
  projectId: string,
  versionId: string,
): Promise<Version> {
  await request<{ status: string }>(
    `${versionRoot(projectId, versionId)}/wake`,
    { method: "POST" },
  );
  return getVersion(projectId, versionId);
}

export async function pinVersion(
  projectId: string,
  versionId: string,
): Promise<Version> {
  await request<{ pinned: boolean }>(
    `${versionRoot(projectId, versionId)}/pin`,
    { method: "POST" },
  );
  return getVersion(projectId, versionId);
}

export async function unpinVersion(
  projectId: string,
  versionId: string,
): Promise<Version> {
  await request<{ pinned: boolean }>(
    `${versionRoot(projectId, versionId)}/unpin`,
    { method: "POST" },
  );
  return getVersion(projectId, versionId);
}

export type WorkerEnvSource = "platform" | "override" | "wrangler";

export interface WorkerEnvVar {
  key: string;
  value: string;
  source: WorkerEnvSource;
  readonly: boolean;
}

export async function getVersionEnv(
  projectId: string,
  versionId: string,
): Promise<WorkerEnvVar[]> {
  const data = await request<{ vars: WorkerEnvVar[] }>(
    `${versionRoot(projectId, versionId)}/env`,
  );
  return data.vars ?? [];
}

export async function putVersionEnv(
  projectId: string,
  versionId: string,
  vars: Record<string, string>,
): Promise<void> {
  await request(`${versionRoot(projectId, versionId)}/env`, {
    method: "PUT",
    body: JSON.stringify({ vars }),
  });
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

export async function getBindings(
  projectId: string,
  versionId: string,
): Promise<Bindings> {
  const data = await request<Partial<Bindings>>(
    `${versionRoot(projectId, versionId)}/bindings`,
  );
  return normalizeBindings(data);
}

export async function listKvNamespaces(
  projectId: string,
  versionId: string,
): Promise<KvNamespacesResponse> {
  const data = await request<KvNamespacesResponse>(
    `${versionRoot(projectId, versionId)}/kv`,
  );
  return { namespaces: data.namespaces ?? [] };
}

export async function listKvKeys(
  projectId: string,
  versionId: string,
  ns: string,
  options: ListKvKeysOptions = {},
): Promise<KvListResult> {
  const query = buildQuery({
    prefix: options.prefix,
    cursor: options.cursor,
    limit: options.limit != null ? String(options.limit) : undefined,
  });
  const data = await request<KvListResult>(
    `${versionRoot(projectId, versionId)}/kv/${encodeURIComponent(ns)}/keys${query}`,
  );
  return { keys: data.keys ?? [], cursor: data.cursor };
}

export async function getKvKey(
  projectId: string,
  versionId: string,
  ns: string,
  key: string,
): Promise<KvValue> {
  return request<KvValue>(
    `${versionRoot(projectId, versionId)}/kv/${encodeURIComponent(ns)}/keys/${encodeURIComponent(key)}`,
  );
}

export async function putKvKey(
  projectId: string,
  versionId: string,
  ns: string,
  key: string,
  body: KvPutBody,
): Promise<void> {
  await request<void>(
    `${versionRoot(projectId, versionId)}/kv/${encodeURIComponent(ns)}/keys/${encodeURIComponent(key)}`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
  );
}

export async function deleteKvKey(
  projectId: string,
  versionId: string,
  ns: string,
  key: string,
): Promise<void> {
  await request<void>(
    `${versionRoot(projectId, versionId)}/kv/${encodeURIComponent(ns)}/keys/${encodeURIComponent(key)}`,
    { method: "DELETE" },
  );
}

export async function getKvInfo(
  projectId: string,
  versionId: string,
  ns: string,
): Promise<KvInfo> {
  return request<KvInfo>(
    `${versionRoot(projectId, versionId)}/kv/${encodeURIComponent(ns)}`,
  );
}

export async function listQueues(
  projectId: string,
  versionId: string,
): Promise<QueueListResponse> {
  const data = await request<QueueListResponse>(
    `${versionRoot(projectId, versionId)}/queues`,
  );
  return { queues: data.queues ?? [] };
}

export async function getQueue(
  projectId: string,
  versionId: string,
  name: string,
): Promise<QueueInfo> {
  return request<QueueInfo>(
    `${versionRoot(projectId, versionId)}/queues/${encodeURIComponent(name)}`,
  );
}

export async function peekQueue(
  projectId: string,
  versionId: string,
  name: string,
  limit?: number,
): Promise<QueuePeekResult> {
  const query = buildQuery({
    limit: limit != null ? String(limit) : undefined,
  });
  return request<QueuePeekResult>(
    `${versionRoot(projectId, versionId)}/queues/${encodeURIComponent(name)}/peek${query}`,
  );
}

export async function pauseQueue(
  projectId: string,
  versionId: string,
  name: string,
): Promise<unknown> {
  return request(
    `${versionRoot(projectId, versionId)}/queues/${encodeURIComponent(name)}/pause`,
    { method: "POST" },
  );
}

export async function resumeQueue(
  projectId: string,
  versionId: string,
  name: string,
): Promise<unknown> {
  return request(
    `${versionRoot(projectId, versionId)}/queues/${encodeURIComponent(name)}/resume`,
    { method: "POST" },
  );
}

export async function redriveQueue(
  projectId: string,
  versionId: string,
  name: string,
  limit?: number,
): Promise<unknown> {
  const query = buildQuery({
    limit: limit != null ? String(limit) : undefined,
  });
  return request(
    `${versionRoot(projectId, versionId)}/queues/${encodeURIComponent(name)}/redrive${query}`,
    { method: "POST" },
  );
}

export async function purgeQueue(
  projectId: string,
  versionId: string,
  name: string,
  body: { force: true },
): Promise<unknown> {
  return request(
    `${versionRoot(projectId, versionId)}/queues/${encodeURIComponent(name)}/purge`,
    {
      method: "POST",
      body: JSON.stringify(body),
    },
  );
}

export async function listWorkflows(
  projectId: string,
  versionId: string,
): Promise<WorkflowListResponse> {
  const data = await request<WorkflowListResponse>(
    `${versionRoot(projectId, versionId)}/workflows`,
  );
  return { workflows: data.workflows ?? [] };
}

export async function listWorkflowInstances(
  projectId: string,
  versionId: string,
  name: string,
): Promise<WorkflowInstances> {
  const data = await request<WorkflowInstances>(
    `${versionRoot(projectId, versionId)}/workflows/${encodeURIComponent(name)}/instances`,
  );
  return {
    workflow_name: data.workflow_name,
    binding: data.binding,
    script_name: data.script_name,
    filter: data.filter,
    limitation: data.limitation ?? null,
    wrangler_workflows: data.wrangler_workflows ?? [],
    instances: data.instances ?? [],
  };
}

function apiErrorCode(body: unknown): string {
  if (
    typeof body === "object" &&
    body !== null &&
    "error" in body &&
    typeof (body as { error: unknown }).error === "string"
  ) {
    return (body as { error: string }).error;
  }
  return "";
}

export function bindingsErrorMessage(e: unknown): {
  title: string;
  description: string;
} {
  if (e instanceof CellpApiError) {
    const code = apiErrorCode(e.body);
    if (e.status === 404) {
      if (code === "version_not_found") {
        return {
          title: "Deployment not found",
          description:
            "This version no longer exists. Pick another deployment from the switcher.",
        };
      }
      if (code === "version_not_ready") {
        return {
          title: "Deployment not ready",
          description:
            "Bindings are available once the deployment reaches ready status.",
        };
      }
      if (code === "wrangler_not_found" || code === "bindings_not_found") {
        return {
          title: "No wrangler manifest",
          description:
            "This deployment artifact has no wrangler.jsonc. Redeploy with a valid Worker bundle.",
        };
      }
      return {
        title: "Bindings API not available",
        description:
          "The cellpd server does not expose the bindings route yet. Rebuild and restart cellpd, then reload this page.",
      };
    }
    return {
      title: "Could not load bindings",
      description: `${e.message} (${e.status})`,
    };
  }
  return {
    title: "Could not load bindings",
    description: "Check your connection to the cellp API and try again.",
  };
}

export async function getDatabase(
  projectId: string,
  versionId: string,
): Promise<DatabaseMetadata> {
  return request<DatabaseMetadata>(
    `/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/database`,
  );
}

export type DatabaseUnavailableReason =
  | "not_found"
  | "not_ready"
  | "network"
  | "server";

export type DatabaseAvailability =
  | { available: true; database: DatabaseMetadata }
  | {
      available: false;
      reason: DatabaseUnavailableReason;
      message: string;
    };

function databaseErrorCode(body: unknown): string {
  if (
    typeof body === "object" &&
    body !== null &&
    "error" in body &&
    typeof (body as { error: unknown }).error === "string"
  ) {
    return (body as { error: string }).error;
  }
  return "";
}

/** Probe whether a version has an attached database (for gating links). */
export async function checkDatabaseAvailability(
  projectId: string,
  versionId: string,
): Promise<DatabaseAvailability> {
  try {
    const database = await getDatabase(projectId, versionId);
    return { available: true, database };
  } catch (e) {
    if (e instanceof CellpApiError) {
      if (e.status === 404) {
        const code = databaseErrorCode(e.body);
        if (code === "version_not_ready" || code.includes("not_ready")) {
          return {
            available: false,
            reason: "not_ready",
            message: "Deployment is not ready yet",
          };
        }
        return {
          available: false,
          reason: "not_found",
          message: "No database attached to this deployment",
        };
      }
      return {
        available: false,
        reason: "server",
        message: `${e.message} (${e.status})`,
      };
    }
    return {
      available: false,
      reason: "network",
      message: "Could not reach the API",
    };
  }
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

export interface DeepHealth {
  status: string;
  registry?: string;
  checks?: Record<string, unknown>;
}

export interface RuntimeRouteRow {
  project_id: string;
  version_id: string;
  active: boolean;
  upstream: string;
  version_status?: string;
  celld_health: string;
}

export interface RuntimeRoutesResponse {
  summary: {
    active_routes: number;
    healthy: number;
    unhealthy: number;
  };
  routes: RuntimeRouteRow[];
}

export async function getHealthDeep(): Promise<DeepHealth> {
  return request<DeepHealth>("/v1/health/deep");
}

export async function getRuntimeRoutes(): Promise<RuntimeRoutesResponse> {
  return request<RuntimeRoutesResponse>("/v1/runtime/routes");
}

/** Parse simple Prometheus gauge lines into a map. */
export function parsePrometheusGauges(text: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const line of text.split("\n")) {
    if (!line || line.startsWith("#")) continue;
    const match = line.match(/^([a-zA-Z_:][a-zA-Z0-9_:]*)\s+(-?\d+(?:\.\d+)?)/);
    if (match) out[match[1]] = Number(match[2]);
  }
  return out;
}

export async function fetchMetricsGauges(): Promise<Record<string, number>> {
  const res = await fetch(`${apiBase()}/metrics`, { cache: "no-store" });
  const text = await res.text();
  if (!res.ok) {
    throw new CellpApiError(`metrics ${res.status}`, res.status, text);
  }
  return parsePrometheusGauges(text);
}

export async function getGatewayHealthDeep(): Promise<DeepHealth> {
  const base = gatewayBase();
  const res = await fetch(`${base}/health/deep`, { cache: "no-store" });
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
    throw new CellpApiError(`gateway health ${res.status}`, res.status, body);
  }
  return body as DeepHealth;
}

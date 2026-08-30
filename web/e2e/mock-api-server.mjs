#!/usr/bin/env node
/**
 * Minimal cellp API mock for Playwright e2e (TP-UI-5, TP6-A4).
 * Listens on MOCK_CELLP_PORT (default 9876).
 */
import http from "node:http";

const PORT = Number(process.env.MOCK_CELLP_PORT || 9876);
const DEFAULT_LIMIT = Number(process.env.MOCK_PAGE_SIZE || 50);
const TOKEN =
  process.env.VITE_CELLP_ADMIN_TOKEN ||
  "test-admin-token";

function version(id, projectId, overrides = {}) {
  return {
    id,
    project_id: projectId,
    parent_version_id: null,
    status: "ready",
    git_ref: "main",
    git_sha: "abc1234def5678",
    data_branch: `${projectId}/${id}`,
    preview_url: `http://127.0.0.1:8787/${projectId}/${id}/`,
    created_at: "2026-01-01T00:30:00.000Z",
    updated_at: "2026-01-01T01:00:00.000Z",
    ready_at: "2026-01-01T01:00:00.000Z",
    error: null,
    ...overrides,
  };
}

const VERSION_ENV = {
  "demo-app/v1": { GREETING: "from-wrangler" },
};

function platformEnvKeys() {
  return new Set(["PROJECT_ID", "VERSION_ID"]);
}

function stripPlatformEnv(vars = {}) {
  const out = {};
  for (const [k, v] of Object.entries(vars)) {
    const upper = String(k).toUpperCase();
    if (upper.startsWith("CELLP_") || upper.startsWith("CELLD_")) continue;
    if (platformEnvKeys().has(upper)) continue;
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(k)) continue;
    out[k] = String(v);
  }
  return out;
}

function envKey(projectId, versionId) {
  return `${projectId}/${versionId}`;
}

const state = {
  projects: {
    "demo-app": {
      id: "demo-app",
      prod_version_id: "v1",
      git_remote: null,
      created_at: "2026-01-01T00:00:00.000Z",
      versions: {
        v1: version("v1", "demo-app", {
          parent_version_id: null,
          git_ref: "main",
          git_sha: "abc1234def5678",
          created_at: "2026-01-01T00:30:00.000Z",
          updated_at: "2026-01-01T01:00:00.000Z",
          ready_at: "2026-01-01T01:00:00.000Z",
        }),
        v2: version("v2", "demo-app", {
          parent_version_id: "v1",
          git_ref: "feature/x",
          git_sha: "fedcba9876543",
          created_at: "2026-01-02T00:30:00.000Z",
          updated_at: "2026-01-02T01:00:00.000Z",
          ready_at: "2026-01-02T01:00:00.000Z",
        }),
        v3: version("v3", "demo-app", {
          parent_version_id: "v2",
          git_ref: "feature/y",
          git_sha: "1111222233334444",
          created_at: "2026-01-03T00:30:00.000Z",
          updated_at: "2026-01-03T01:00:00.000Z",
          ready_at: "2026-01-03T01:00:00.000Z",
        }),
        v4: version("v4", "demo-app", {
          parent_version_id: "v3",
          git_ref: "feature/z",
          git_sha: "aaaabbbbccccdddd",
          created_at: "2026-01-04T00:30:00.000Z",
          updated_at: "2026-01-04T01:00:00.000Z",
          ready_at: "2026-01-04T01:00:00.000Z",
        }),
        v5: version("v5", "demo-app", {
          parent_version_id: "v4",
          git_ref: "feature/w",
          git_sha: "eeeeffff00001111",
          created_at: "2026-01-05T00:30:00.000Z",
          updated_at: "2026-01-05T01:00:00.000Z",
          ready_at: "2026-01-05T01:00:00.000Z",
        }),
      },
    },
    "extra-app": {
      id: "extra-app",
      prod_version_id: null,
      git_remote: null,
      created_at: "2026-01-02T00:00:00.000Z",
      versions: {
        v1: version("v1", "extra-app", {
          created_at: "2026-01-02T00:30:00.000Z",
        }),
        "v-pending": version("v-pending", "extra-app", {
          parent_version_id: "v1",
          status: "pending",
          ready_at: null,
          git_ref: "feature/pending",
          created_at: "2026-01-02T00:45:00.000Z",
        }),
      },
    },
    "third-app": {
      id: "third-app",
      prod_version_id: null,
      git_remote: null,
      created_at: "2026-01-03T00:00:00.000Z",
      versions: {},
    },
  },
};

function corsHeaders() {
  return {
    "content-type": "application/json",
    "access-control-allow-origin": "*",
    "access-control-allow-methods": "GET, POST, PUT, DELETE, OPTIONS",
    "access-control-allow-headers": "Authorization, Content-Type",
  };
}

function json(res, status, body) {
  res.writeHead(status, corsHeaders());
  res.end(JSON.stringify(body, null, 2));
}

function noContent(res) {
  res.writeHead(204, {
    "access-control-allow-origin": "*",
    "access-control-allow-methods": "GET, POST, PUT, DELETE, OPTIONS",
    "access-control-allow-headers": "Authorization, Content-Type",
  });
  res.end();
}

function auth(req) {
  const h = req.headers.authorization || "";
  return h === `Bearer ${TOKEN}`;
}

function parseBody(req) {
  return new Promise((resolve, reject) => {
    let data = "";
    req.on("data", (c) => (data += c));
    req.on("end", () => {
      try {
        resolve(data ? JSON.parse(data) : {});
      } catch (e) {
        reject(e);
      }
    });
  });
}

function paginate(items, cursor, limit) {
  const offset = cursor ? Number.parseInt(cursor, 10) : 0;
  const safeOffset = Number.isFinite(offset) && offset >= 0 ? offset : 0;
  const page = items.slice(safeOffset, safeOffset + limit);
  const nextOffset = safeOffset + limit;
  const next_cursor = nextOffset < items.length ? String(nextOffset) : null;
  return { page, next_cursor };
}

function projectSummaries() {
  return Object.values(state.projects)
    .map((p) => ({
      id: p.id,
      prod_version_id: p.prod_version_id,
      version_count: Object.keys(p.versions).length,
      created_at: p.created_at,
    }))
    .sort((a, b) => a.id.localeCompare(b.id));
}

function projectVersions(project, statusFilter) {
  let versions = Object.values(project.versions);
  if (statusFilter) {
    versions = versions.filter((v) => v.status === statusFilter);
  }
  return versions.sort((a, b) => {
    const aTime = new Date(a.created_at).getTime();
    const bTime = new Date(b.created_at).getTime();
    if (aTime !== bTime) return bTime - aTime;
    return b.id.localeCompare(a.id);
  });
}

function projectDetail(project) {
  return {
    id: project.id,
    prod_version_id: project.prod_version_id,
    git_remote: project.git_remote,
    created_at: project.created_at,
    version_count: Object.keys(project.versions).length,
  };
}

const TABLE_SCHEMAS = {
  users: [
    { name: "id", type: "INTEGER" },
    { name: "email", type: "TEXT" },
    { name: "name", type: "TEXT" },
    { name: "created_at", type: "TEXT" },
  ],
  posts: [
    { name: "id", type: "INTEGER" },
    { name: "user_id", type: "INTEGER" },
    { name: "title", type: "TEXT" },
    { name: "body", type: "TEXT" },
    { name: "created_at", type: "TEXT" },
  ],
  comments: [
    { name: "id", type: "INTEGER" },
    { name: "post_id", type: "INTEGER" },
    { name: "user_id", type: "INTEGER" },
    { name: "body", type: "TEXT" },
    { name: "created_at", type: "TEXT" },
  ],
};

const VERSION_ROW_COUNTS = {
  v1: { users: 10, posts: 25, comments: 50 },
  v2: { users: 12, posts: 35, comments: 60 },
  v3: { users: 15, posts: 45, comments: 75 },
  v4: { users: 16, posts: 50, comments: 80 },
  v5: { users: 17, posts: 55, comments: 85 },
};

function hasChildBranches(project, versionId) {
  return Object.values(project.versions).some(
    (v) =>
      v.parent_version_id === versionId &&
      v.status !== "destroyed" &&
      v.status !== "draining",
  );
}

function forkRowCounts(parentId) {
  const parent = VERSION_ROW_COUNTS[parentId];
  if (!parent) return { users: 5, posts: 10, comments: 20 };
  return {
    users: parent.users + 2,
    posts: parent.posts + 5,
    comments: parent.comments + 10,
  };
}

function generateUsers(count) {
  const names = ["Alice", "Bob", "Carol", "Dave", "Eve", "Frank", "Grace", "Hank"];
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    email: `${names[i % names.length].toLowerCase()}${i > 7 ? i : ""}@example.com`,
    name: names[i % names.length] + (i > 7 ? ` ${i}` : ""),
    created_at: `2026-01-${String((i % 28) + 1).padStart(2, "0")}T10:00:00.000Z`,
  }));
}

function generatePosts(count) {
  const titles = [
    "Hello World",
    "Getting Started",
    "Deep Dive",
    "Best Practices",
    "Release Notes",
  ];
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    user_id: (i % 10) + 1,
    title: `${titles[i % titles.length]} #${i + 1}`,
    body: `Content for post ${i + 1}.`,
    created_at: `2026-01-${String((i % 28) + 1).padStart(2, "0")}T12:00:00.000Z`,
  }));
}

function generateComments(count) {
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    post_id: (i % 25) + 1,
    user_id: (i % 10) + 1,
    body: `Comment text for comment ${i + 1}.`,
    created_at: `2026-01-${String((i % 28) + 1).padStart(2, "0")}T14:00:00.000Z`,
  }));
}

function versionTableData(versionId) {
  const counts = VERSION_ROW_COUNTS[versionId];
  if (!counts) return null;
  return {
    users: generateUsers(counts.users),
    posts: generatePosts(counts.posts),
    comments: generateComments(counts.comments),
  };
}

function databaseMetadata(projectId, versionId, version) {
  return {
    database_id: `db-${projectId}-${versionId}`,
    database_name: "main",
    data_branch: version.data_branch,
    parent_version_id: version.parent_version_id,
    branch_method:
      versionId === "v1"
        ? "d1_branch"
        : versionId === "v3"
          ? "offshoot_export"
          : "d1_branch",
    status: "ready",
  };
}

function bindingsManifest(projectId, versionId) {
  return {
    d1: [
      {
        binding: "DB",
        database_name: "main",
        database_id: `db-${projectId}-${versionId}`,
      },
    ],
    kv: [
      {
        binding: "CACHE",
        id: `${projectId}-cache`,
      },
    ],
    queues: [
      {
        name: "tasks",
        binding: "TASKS",
        consumer: false,
      },
    ],
    workflows: [
      {
        binding: "REPORTS",
        name: "report-builder",
        class_name: "ReportBuilder",
      },
    ],
    r2: [
      {
        binding: "FILES",
        bucket_name: `${projectId}-files`,
      },
    ],
    crons: versionId === "v1" ? ["0 * * * *"] : [],
  };
}

/** Per-version KV maps. Child versions branch from parent (AD-8); sibling writes stay isolated. */
const kvStores = {};
const queueStores = {};
const workflowStores = {};

function versionParentId(projectId, versionId) {
  return state.projects[projectId]?.versions?.[versionId]?.parent_version_id ?? null;
}

function kvStoreId(projectId, versionId, ns) {
  return `${projectId}::${versionId}::${ns}`;
}

function queueStoreId(projectId, versionId, name) {
  return `${projectId}::${versionId}::${name}`;
}

function workflowStoreId(projectId, versionId, name) {
  return `${projectId}::${versionId}::${name}`;
}

function seedKvMap(projectId, versionId, ns) {
  const expectedNs = `${projectId}-cache`;
  const map = new Map();
  if (ns !== expectedNs) return map;

  const parentId = versionParentId(projectId, versionId);
  if (parentId) {
    const parentMap = getKvMap(projectId, parentId, ns);
    for (const [key, entry] of parentMap) {
      map.set(key, { ...entry });
    }
  }

  if (projectId === "demo-app" && versionId === "v1") {
    map.set("greeting", { value: "hello-prod", encoding: "utf-8" });
    map.set("item-1", { value: "one", encoding: "utf-8" });
    map.set("item-2", { value: "two", encoding: "utf-8" });
  }
  if (projectId === "demo-app" && versionId === "v3") {
    map.set("preview-v3", { value: "isolated-v3", encoding: "utf-8" });
  }
  return map;
}

function getKvMap(projectId, versionId, ns) {
  const id = kvStoreId(projectId, versionId, ns);
  if (!kvStores[id]) {
    kvStores[id] = seedKvMap(projectId, versionId, ns);
  }
  return kvStores[id];
}

function declaredKvIds(projectId) {
  return [`${projectId}-cache`];
}

function seedQueue(projectId, versionId, name) {
  const messages = [];
  const parentId = versionParentId(projectId, versionId);
  if (parentId) {
    const parentQ = getQueueState(projectId, parentId, name);
    for (const msg of parentQ.messages) {
      messages.push({ ...msg });
    }
  }
  if (projectId === "demo-app" && versionId === "v1" && name === "tasks") {
    messages.push({
      id: "msg-1",
      bodyBase64: Buffer.from("hello-queue").toString("base64"),
      contentType: "text/plain",
    });
  }
  return { paused: false, messages };
}

function getQueueState(projectId, versionId, name) {
  const id = queueStoreId(projectId, versionId, name);
  if (!queueStores[id]) {
    queueStores[id] = seedQueue(projectId, versionId, name);
  }
  return queueStores[id];
}

function declaredQueueNames() {
  return ["tasks"];
}

function findWorkflow(projectId, versionId, name) {
  const declared = bindingsManifest(projectId, versionId).workflows;
  return (
    declared.find((w) => w.name === name) ||
    declared.find((w) => w.binding === name) ||
    null
  );
}

const WORKFLOW_FILTER_LIMITATION =
  "celld `cell list` 只给出 `__Workflow.{script}:<64-hex>`；实例内部名 `{workflow_name}/{instance_id}` 被哈希，无法按 workflow 名精确过滤。本响应返回该 version / 该 script 下全部 Workflow cell，并附 wrangler 声明的 workflow 名。精确归属需等 `celld workflow`.";

function seedWorkflowInstances(projectId, versionId, wf) {
  if (projectId === "demo-app" && versionId === "v1") {
    return [
      {
        scope: `__Workflow.${projectId}:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
        class: `__Workflow.${projectId}`,
        id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        reserved: true,
        status: "running",
        created_at: "2026-01-01T01:00:00.000Z",
        updated_at: "2026-01-01T01:05:00.000Z",
      },
    ];
  }
  return [];
}

function getWorkflowInstances(projectId, versionId, wf) {
  const id = workflowStoreId(projectId, versionId, wf.name);
  if (!workflowStores[id]) {
    workflowStores[id] = seedWorkflowInstances(projectId, versionId, wf);
  }
  return workflowStores[id];
}

function requireReadyVersion(project, versionId) {
  const v = project?.versions?.[versionId];
  if (!v) return { error: "version_not_found", status: 404 };
  if (v.status !== "ready") return { error: "version_not_ready", status: 404 };
  return { version: v };
}

function parseSelectSql(sql) {
  const normalized = sql.trim().replace(/\s+/g, " ");
  const selectMatch = normalized.match(
    /^SELECT\s+(.+?)\s+FROM\s+(\w+)(?:\s+LIMIT\s+(\d+))?(?:\s+OFFSET\s+(\d+))?$/i,
  );
  if (!selectMatch) return null;
  const [, columnsPart, tableName, limitStr, offsetStr] = selectMatch;
  return {
    columns: columnsPart === "*" ? null : columnsPart.split(/\s*,\s*/).map((c) => c.trim()),
    tableName: tableName.toLowerCase(),
    limit: limitStr ? Number(limitStr) : undefined,
    offset: offsetStr ? Number(offsetStr) : 0,
  };
}

function parseWriteSql(sql) {
  const normalized = sql.trim().replace(/\s+/g, " ");
  const insertMatch = normalized.match(/^INSERT\s+INTO\s+\w+/i);
  const updateMatch = normalized.match(/^UPDATE\s+\w+/i);
  const deleteMatch = normalized.match(/^DELETE\s+FROM\s+\w+/i);
  if (insertMatch) return { type: "insert" };
  if (updateMatch) return { type: "update" };
  if (deleteMatch) return { type: "delete" };
  return null;
}

function projectRows(tableData, tableName, columns, limit, offset) {
  const schema = TABLE_SCHEMAS[tableName];
  const allRows = tableData[tableName];
  if (!schema || !allRows) return null;

  let selectedRows = allRows;
  if (columns) {
    selectedRows = allRows.map((row) => {
      const picked = {};
      for (const col of columns) {
        if (col in row) picked[col] = row[col];
      }
      return picked;
    });
  }

  const resultColumns = columns
    ? schema.filter((c) => columns.includes(c.name))
    : schema;

  const safeOffset = Number.isFinite(offset) && offset >= 0 ? offset : 0;
  const safeLimit = Number.isFinite(limit) && limit > 0 ? limit : allRows.length;
  const page = selectedRows.slice(safeOffset, safeOffset + safeLimit);

  return {
    columns: resultColumns,
    rows: page,
    total: allRows.length,
    limit: safeLimit,
    offset: safeOffset,
  };
}

function executeQuery(tableData, sql) {
  const start = Date.now();
  const write = parseWriteSql(sql);
  if (write) {
    return {
      columns: [],
      rows: [],
      duration_ms: Date.now() - start + 5,
      rows_affected: write.type === "insert" ? 1 : write.type === "update" ? 3 : 2,
    };
  }

  const parsed = parseSelectSql(sql);
  if (!parsed) {
    return { error: "invalid_sql", status: 400 };
  }

  const { tableName, columns, limit, offset } = parsed;
  if (!TABLE_SCHEMAS[tableName] || !tableData[tableName]) {
    return { error: "table_not_found", status: 400 };
  }

  const result = projectRows(tableData, tableName, columns, limit, offset ?? 0);
  return {
    columns: result.columns,
    rows: result.rows,
    duration_ms: Date.now() - start + 12,
    rows_affected: null,
  };
}

const server = http.createServer(async (req, res) => {
  if (req.method === "OPTIONS") {
    res.writeHead(204, {
      "access-control-allow-origin": "*",
      "access-control-allow-methods": "GET, POST, PUT, DELETE, OPTIONS",
      "access-control-allow-headers": "Authorization, Content-Type",
    });
    res.end();
    return;
  }

  const url = new URL(req.url, `http://127.0.0.1:${PORT}`);
  const parts = url.pathname.split("/").filter(Boolean);

  if (req.method === "GET" && url.pathname === "/v1/health") {
    return json(res, 200, { status: "ok" });
  }

  if (req.method === "GET" && url.pathname === "/v1/health/deep") {
    return json(res, 200, {
      status: "ok",
      checks: {
        registry: { status: "ok", latency_ms: 1 },
        rustfs: { status: "ok", latency_ms: 2 },
        celld: { status: "ok", detail: "installed" },
        runtimes: { active_routes: 2, healthy: 2, unhealthy: 0 },
        queue: { pending_jobs: 0, queue_max: 10000 },
      },
    });
  }

  if (req.method === "GET" && url.pathname === "/metrics") {
    res.writeHead(200, {
      "content-type": "text/plain; version=0.0.4",
      "access-control-allow-origin": "*",
    });
    res.end(
      [
        "cellp_pending_jobs 0",
        "cellp_routes_active 2",
        "cellp_celld_healthy 2",
        "cellp_celld_unhealthy 0",
        "cellp_gateway_requests_total 42",
      ].join("\n"),
    );
    return;
  }

  if (parts[0] !== "v1") {
    return json(res, 404, { error: "not found" });
  }

  if (!auth(req)) {
    return json(res, 401, { error: "unauthorized" });
  }

  if (req.method === "GET" && parts[1] === "runtime" && parts[2] === "routes") {
    return json(res, 200, {
      summary: { active_routes: 2, healthy: 2, unhealthy: 0 },
      routes: [
        {
          project_id: "demo-app",
          version_id: "v1",
          active: true,
          upstream: "127.0.0.1:8803",
          version_status: "ready",
          celld_health: "ok",
        },
        {
          project_id: "demo-app",
          version_id: "v2",
          active: true,
          upstream: "127.0.0.1:8804",
          version_status: "ready",
          celld_health: "ok",
        },
      ],
    });
  }

  if (parts[1] === "projects" && parts.length === 2 && req.method === "GET") {
    const limit = Number(url.searchParams.get("limit")) || DEFAULT_LIMIT;
    const cursor = url.searchParams.get("cursor");
    const q = (url.searchParams.get("q") || "").trim().toLowerCase();
    let items = projectSummaries();
    if (q) {
      items = items.filter((p) => p.id.toLowerCase().includes(q));
    }
    const { page, next_cursor } = paginate(items, cursor, limit);
    return json(res, 200, { projects: page, next_cursor });
  }

  const projectId = parts[2];
  const project = state.projects[projectId];

  if (parts[1] === "projects" && parts.length === 3 && req.method === "GET") {
    if (!project) return json(res, 404, { error: "project_not_found" });
    return json(res, 200, projectDetail(project));
  }

  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts.length === 4 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const limit = Number(url.searchParams.get("limit")) || DEFAULT_LIMIT;
    const cursor = url.searchParams.get("cursor");
    const statusFilter = url.searchParams.get("status") || undefined;
    const all = projectVersions(project, statusFilter);
    const { page, next_cursor } = paginate(all, cursor, limit);
    return json(res, 200, { versions: page, next_cursor });
  }

  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts.length === 4 &&
    req.method === "POST"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    let body;
    try {
      body = await parseBody(req);
    } catch {
      return json(res, 400, { error: "invalid_json" });
    }
    const id = body.id?.trim();
    if (!id) return json(res, 400, { error: "id_required" });
    if (project.versions[id]) return json(res, 409, { error: "version_exists" });

    const parentId = body.parent_version_id ?? null;
    if (parentId) {
      const parent = project.versions[parentId];
      if (!parent) return json(res, 404, { error: "parent_not_found" });
      if (parent.status !== "ready") {
        return json(res, 422, { error: "parent_not_ready" });
      }
    }

    const now = new Date().toISOString();
    const newVersion = version(id, projectId, {
      parent_version_id: parentId,
      git_ref: body.git_ref || `branch/${id}`,
      git_sha: body.git_sha || "0000000000000000",
      data_branch: `${projectId}/${id}`,
      status: "ready",
      created_at: now,
      updated_at: now,
      ready_at: now,
    });
    project.versions[id] = newVersion;
    const inherited = parentId
      ? { ...(VERSION_ENV[envKey(projectId, parentId)] || {}) }
      : {};
    VERSION_ENV[envKey(projectId, id)] = {
      ...inherited,
      ...stripPlatformEnv(body.env || {}),
    };
    if (parentId) {
      VERSION_ROW_COUNTS[id] = forkRowCounts(parentId);
    } else {
      VERSION_ROW_COUNTS[id] = { users: 5, posts: 10, comments: 20 };
    }
    return json(res, 202, {
      id,
      status: "ready",
      project_id: projectId,
    });
  }

  const versionId = parts[4];

  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "env" &&
    parts.length === 6
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const v = project.versions?.[versionId];
    if (!v) return json(res, 404, { error: "version_not_found" });
    const key = envKey(projectId, versionId);
    if (req.method === "GET") {
      const overrides = VERSION_ENV[key] || {};
      const vars = [
        { key: "PROJECT_ID", value: projectId, source: "platform", readonly: true },
        { key: "VERSION_ID", value: versionId, source: "platform", readonly: true },
        ...Object.entries(overrides).map(([k, value]) => ({
          key: k,
          value,
          source: "override",
          readonly: false,
        })),
      ];
      return json(res, 200, { vars });
    }
    if (req.method === "PUT") {
      let body;
      try {
        body = await parseBody(req);
      } catch {
        return json(res, 400, { error: "invalid_json" });
      }
      VERSION_ENV[key] = stripPlatformEnv(body.vars || {});
      return json(res, 200, { id: versionId, project_id: projectId, status: v.status });
    }
  }

  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts.length === 5 &&
    req.method === "GET"
  ) {
    const v = project?.versions?.[versionId];
    if (!v) return json(res, 404, { error: "version_not_found" });
    return json(res, 200, v);
  }

  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "promote" &&
    req.method === "POST"
  ) {
    const v = project?.versions?.[versionId];
    if (!v) return json(res, 404, { error: "version_not_found" });
    if (v.status !== "ready") {
      return json(res, 409, { error: "invalid_status" });
    }
    project.prod_version_id = versionId;
    return json(res, 200, {
      status: "promoted",
      prod_version_id: versionId,
    });
  }

  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts.length === 5 &&
    req.method === "DELETE"
  ) {
    const v = project?.versions?.[versionId];
    if (!v) return json(res, 404, { error: "version_not_found" });
    if (project.prod_version_id === versionId) {
      return json(res, 409, { error: "cannot_delete_prod" });
    }
    if (hasChildBranches(project, versionId)) {
      return json(res, 409, { error: "has_child_branches" });
    }
    v.status = "draining";
    setTimeout(() => {
      v.status = "destroyed";
    }, 100);
    return json(res, 202, { status: "draining" });
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/bindings
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "bindings" &&
    parts.length === 6 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    return json(res, 200, bindingsManifest(projectId, versionId));
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/kv
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "kv" &&
    parts.length === 6 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const namespaces = bindingsManifest(projectId, versionId).kv.map((k) => ({
      id: k.id,
      binding: k.binding,
    }));
    return json(res, 200, { namespaces });
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/kv/{ns}
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "kv" &&
    parts.length === 7 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const ns = decodeURIComponent(parts[6]);
    if (!declaredKvIds(projectId).includes(ns)) {
      return json(res, 404, { error: "kv_namespace_not_found" });
    }
    const map = getKvMap(projectId, versionId, ns);
    let bytes = 0;
    for (const entry of map.values()) {
      bytes += Buffer.byteLength(entry.value, "utf8");
    }
    return json(res, 200, {
      keys: map.size,
      live: map.size,
      bytes,
      stored: map.size,
    });
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/kv/{ns}/keys
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "kv" &&
    parts[7] === "keys" &&
    parts.length === 8 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const ns = decodeURIComponent(parts[6]);
    if (!declaredKvIds(projectId).includes(ns)) {
      return json(res, 404, { error: "kv_namespace_not_found" });
    }
    const map = getKvMap(projectId, versionId, ns);
    const prefix = url.searchParams.get("prefix") || "";
    const limit = Number(url.searchParams.get("limit")) || 1000;
    const cursor = url.searchParams.get("cursor");
    let names = [...map.keys()].sort();
    if (prefix) names = names.filter((n) => n.startsWith(prefix));
    const offset = cursor ? Number.parseInt(cursor, 10) : 0;
    const safeOffset = Number.isFinite(offset) && offset >= 0 ? offset : 0;
    const page = names.slice(safeOffset, safeOffset + limit);
    const nextOffset = safeOffset + limit;
    const nextCursor = nextOffset < names.length ? String(nextOffset) : undefined;
    const body = {
      keys: page.map((name) => ({ name })),
    };
    if (nextCursor) body.cursor = nextCursor;
    return json(res, 200, body);
  }

  // GET|PUT|DELETE /v1/projects/{projectId}/versions/{versionId}/kv/{ns}/keys/{key}
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "kv" &&
    parts[7] === "keys" &&
    parts.length >= 9
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const ns = decodeURIComponent(parts[6]);
    if (!declaredKvIds(projectId).includes(ns)) {
      return json(res, 404, { error: "kv_namespace_not_found" });
    }
    const key = parts.slice(8).map((p) => decodeURIComponent(p)).join("/");
    if (!key) return json(res, 400, { error: "key_required" });
    const map = getKvMap(projectId, versionId, ns);

    if (req.method === "GET") {
      const entry = map.get(key);
      if (!entry) return json(res, 404, { error: "key_not_found" });
      return json(res, 200, {
        key,
        value: entry.value,
        encoding: entry.encoding || "utf-8",
      });
    }

    if (req.method === "PUT") {
      let body;
      try {
        body = await parseBody(req);
      } catch {
        return json(res, 400, { error: "invalid_json" });
      }
      if (body.value === undefined || body.value === null) {
        return json(res, 400, { error: "value_required" });
      }
      map.set(key, {
        value: String(body.value),
        encoding: body.base64 || body.binary ? "base64" : "utf-8",
      });
      return noContent(res);
    }

    if (req.method === "DELETE") {
      map.delete(key);
      return noContent(res);
    }
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/queues
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "queues" &&
    parts.length === 6 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    return json(res, 200, {
      queues: declaredQueueNames().map((name) => ({ name })),
    });
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/queues/{name}
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "queues" &&
    parts.length === 7 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const name = decodeURIComponent(parts[6]);
    if (!declaredQueueNames().includes(name)) {
      return json(res, 404, { error: "queue_not_found" });
    }
    const q = getQueueState(projectId, versionId, name);
    const backlogCount = q.messages.length;
    const backlogBytes = q.messages.reduce((sum, m) => {
      if (typeof m.bodyBase64 !== "string") return sum;
      return sum + Buffer.from(m.bodyBase64, "base64").length;
    }, 0);
    return json(res, 200, {
      name,
      paused: q.paused,
      pending: backlogCount,
      backlogCount,
      backlogBytes,
      stored: backlogCount,
    });
  }

  // GET peek / POST pause|resume|redrive|purge
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "queues" &&
    parts.length === 8
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const name = decodeURIComponent(parts[6]);
    if (!declaredQueueNames().includes(name)) {
      return json(res, 404, { error: "queue_not_found" });
    }
    const action = parts[7];
    const q = getQueueState(projectId, versionId, name);

    if (action === "peek" && req.method === "GET") {
      const rawLimit = url.searchParams.get("limit");
      const limit = rawLimit ? Number(rawLimit) : 10;
      if (!Number.isInteger(limit) || limit < 1 || limit > 100) {
        return json(res, 400, { error: "invalid_limit" });
      }
      return json(res, 200, { messages: q.messages.slice(0, limit) });
    }

    if (action === "pause" && req.method === "POST") {
      q.paused = true;
      return json(res, 200, { ok: true, paused: true });
    }

    if (action === "resume" && req.method === "POST") {
      q.paused = false;
      return json(res, 200, { ok: true, paused: false });
    }

    if (action === "redrive" && req.method === "POST") {
      const rawLimit = url.searchParams.get("limit");
      const limit = rawLimit ? Number(rawLimit) : 100;
      if (!Number.isInteger(limit) || limit < 1 || limit > 100) {
        return json(res, 400, { error: "invalid_limit" });
      }
      return json(res, 200, { redriven: 0 });
    }

    if (action === "purge" && req.method === "POST") {
      let body;
      try {
        body = await parseBody(req);
      } catch {
        return json(res, 400, { error: "force_required" });
      }
      if (body.force !== true) {
        return json(res, 400, { error: "force_required" });
      }
      q.messages = [];
      return json(res, 200, { ok: true, purged: true });
    }
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/workflows
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "workflows" &&
    parts.length === 6 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const items = bindingsManifest(projectId, versionId).workflows.map((w) => ({
      binding: w.binding,
      workflow_name: w.name,
      class_name: w.class_name,
    }));
    return json(res, 200, { workflows: items });
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/workflows/{name}/instances
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "workflows" &&
    parts[7] === "instances" &&
    parts.length === 8 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const name = decodeURIComponent(parts[6]);
    const wf = findWorkflow(projectId, versionId, name);
    if (!wf) return json(res, 404, { error: "workflow_not_found" });
    const instances = getWorkflowInstances(projectId, versionId, wf);
    const declared = bindingsManifest(projectId, versionId).workflows;
    const multi = declared.length >= 2 || versionId === "v2";
    return json(res, 200, {
      workflow_name: wf.name,
      binding: wf.binding,
      script_name: projectId,
      filter: multi ? "script" : "workflow",
      limitation: multi ? WORKFLOW_FILTER_LIMITATION : null,
      wrangler_workflows: declared.map((w) => w.name),
      instances,
    });
  }

  // Workflows are read-only — any POST is outside the contract.
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "workflows" &&
    req.method === "POST"
  ) {
    return json(res, 404, { error: "not found" });
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/database
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "database" &&
    parts.length === 6 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const tableData = versionTableData(versionId);
    if (!tableData) return json(res, 404, { error: "database_not_found" });
    return json(res, 200, databaseMetadata(projectId, versionId, check.version));
  }

  // GET /v1/projects/{projectId}/versions/{versionId}/database/tables
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "database" &&
    parts[6] === "tables" &&
    parts.length === 7 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const tableData = versionTableData(versionId);
    if (!tableData) return json(res, 404, { error: "database_not_found" });
    const tables = Object.keys(TABLE_SCHEMAS).map((name) => ({
      name,
      type: "table",
      row_count: tableData[name].length,
    }));
    return json(res, 200, { tables });
  }

  const tableName = parts[7];

  // GET /v1/projects/{projectId}/versions/{versionId}/database/tables/{tableName}/rows
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "database" &&
    parts[6] === "tables" &&
    parts[8] === "rows" &&
    parts.length === 9 &&
    req.method === "GET"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const tableData = versionTableData(versionId);
    if (!tableData) return json(res, 404, { error: "database_not_found" });
    const limit = Number(url.searchParams.get("limit")) || DEFAULT_LIMIT;
    const offset = Number(url.searchParams.get("offset")) || 0;
    const result = projectRows(tableData, tableName.toLowerCase(), null, limit, offset);
    if (!result) return json(res, 404, { error: "table_not_found" });
    return json(res, 200, result);
  }

  // POST /v1/projects/{projectId}/versions/{versionId}/database/query
  if (
    parts[1] === "projects" &&
    parts[3] === "versions" &&
    parts[5] === "database" &&
    parts[6] === "query" &&
    parts.length === 7 &&
    req.method === "POST"
  ) {
    if (!project) return json(res, 404, { error: "project_not_found" });
    const check = requireReadyVersion(project, versionId);
    if (check.error) return json(res, check.status, { error: check.error });
    const tableData = versionTableData(versionId);
    if (!tableData) return json(res, 404, { error: "database_not_found" });
    let body;
    try {
      body = await parseBody(req);
    } catch {
      return json(res, 400, { error: "invalid_json" });
    }
    if (!body.sql || typeof body.sql !== "string") {
      return json(res, 400, { error: "sql_required" });
    }
    const result = executeQuery(tableData, body.sql);
    if (result.error) return json(res, result.status, { error: result.error });
    return json(res, 200, result);
  }

  await parseBody(req).catch(() => ({}));
  return json(res, 404, { error: "not found" });
});

server.listen(PORT, "127.0.0.1", () => {
  console.log(`mock cellp API on http://127.0.0.1:${PORT}`);
});

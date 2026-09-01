#!/usr/bin/env node
/**
 * cellp mock — local dev only (过渡，由 cellpd 替换).
 * API :8790 · Gateway :8787（模拟 cellpd 内置 Gateway）
 * Registry: dev/data/registry.json（cellpd 使用 SQLite）
 */
import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, "../..");
const ENV_PATH = path.join(ROOT, "dev/.env");

function loadEnv() {
  const env = {
    PLATFORM_PORT: "8790",
    GATEWAY_PORT: "8787",
    CELLD_PORT: "8792",
    PLATFORM_TOKEN: "dev-local-token",
    GATEWAY_URL: "http://127.0.0.1:8787",
    CELLP_INGRESS_BASE_DOMAIN: "ingress.local",
    INGRESS_HOST_ONLY: "1",
  };
  if (fs.existsSync(ENV_PATH)) {
    for (const line of fs.readFileSync(ENV_PATH, "utf8").split("\n")) {
      const m = line.match(/^([A-Z_]+)=(.*)$/);
      if (m) env[m[1]] = m[2].trim();
    }
  }
  return env;
}

const env = loadEnv();
const REGISTRY_PATH = path.resolve(ROOT, env.REGISTRY_DB?.replace(/^\.\//, "") || "dev/data/registry.json").replace(/\.sqlite$/, ".json");
const TOKEN = env.PLATFORM_TOKEN;
const CELLD_HOST = "127.0.0.1";
const CELLD_PORT = Number(env.CELLD_PORT || 8792);

function loadRegistry() {
  if (!fs.existsSync(REGISTRY_PATH)) {
    return { projects: {} };
  }
  return JSON.parse(fs.readFileSync(REGISTRY_PATH, "utf8"));
}

function saveRegistry(reg) {
  fs.mkdirSync(path.dirname(REGISTRY_PATH), { recursive: true });
  fs.writeFileSync(REGISTRY_PATH, JSON.stringify(reg, null, 2));
}

function json(res, status, body) {
  res.writeHead(status, { "content-type": "application/json" });
  res.end(JSON.stringify(body, null, 2));
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

function proxyToCelld(req, res, upstreamPath) {
  const opts = {
    hostname: CELLD_HOST,
    port: CELLD_PORT,
    path: upstreamPath,
    method: req.method,
    headers: { ...req.headers, host: `${CELLD_HOST}:${CELLD_PORT}` },
  };
  const upstream = http.request(opts, (upRes) => {
    res.writeHead(upRes.statusCode || 502, upRes.headers);
    upRes.pipe(res);
  });
  upstream.on("error", () => {
    if (!res.headersSent) {
      res.writeHead(502, { "content-type": "text/plain" });
      res.end("bad gateway — celld not reachable");
    }
  });
  req.pipe(upstream);
}

/** AD-12: prod `{project}.{base}` or preview `{version}.{project}.{base}` */
function matchesIngressHost(reqHost, ingressBase) {
  const suffix = `.${ingressBase}`;
  if (!reqHost.endsWith(suffix)) return false;
  const prefix = reqHost.slice(0, -suffix.length);
  if (!prefix) return false;
  const labels = prefix.split(".").filter(Boolean);
  return labels.length === 1 || labels.length === 2;
}

const apiServer = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  const parts = url.pathname.split("/").filter(Boolean);

  if (req.method === "GET" && url.pathname === "/v1/health") {
    return json(res, 200, { status: "ok", registry: REGISTRY_PATH, gateway: env.GATEWAY_URL });
  }

  if (parts[0] !== "v1") {
    return json(res, 404, { error: "not found" });
  }

  if (req.method !== "GET" && !auth(req)) {
    return json(res, 401, { error: "unauthorized" });
  }

  const reg = loadRegistry();

  if (parts[1] === "projects" && parts.length === 2 && req.method === "GET") {
    const projects = Object.values(reg.projects).map((p) => ({
      id: p.id,
      prod_version_id: p.prod_version_id,
      version_count: Object.keys(p.versions || {}).length,
      created_at: p.created_at,
    }));
    return json(res, 200, { projects });
  }

  if (parts[1] === "projects" && parts.length === 2 && req.method === "POST") {
    try {
      const body = await parseBody(req);
      const id = body.id;
      if (!id) return json(res, 400, { error: "id required" });
      if (!reg.projects[id]) {
        reg.projects[id] = {
          id,
          git_remote: body.git_remote || null,
          prod_version_id: null,
          versions: {},
          created_at: new Date().toISOString(),
        };
        saveRegistry(reg);
      }
      return json(res, 201, reg.projects[id]);
    } catch {
      return json(res, 400, { error: "invalid json" });
    }
  }

  const projectId = parts[2];
  const project = reg.projects[projectId];

  if (parts[1] === "projects" && parts.length === 3 && req.method === "GET") {
    if (!project) return json(res, 404, { error: "project_not_found" });
    return json(res, 200, {
      ...project,
      versions: Object.values(project.versions || {}),
    });
  }

  if (parts[1] === "projects" && parts[3] === "versions" && parts.length === 4 && req.method === "POST") {
    if (!project) {
      reg.projects[projectId] = { id: projectId, prod_version_id: null, versions: {}, created_at: new Date().toISOString() };
      saveRegistry(reg);
    }
    try {
      const body = await parseBody(req);
      const versionId = body.id || `v-${Date.now()}`;
      const previewUrl = `${env.GATEWAY_URL}/${projectId}/${versionId}/`;
      const version = {
        id: versionId,
        project_id: projectId,
        parent_version_id: body.parent_version_id || null,
        status: "ready",
        git_ref: body.git_ref || "local",
        git_sha: body.git_sha || "local",
        data_branch: `${projectId}/${versionId}`,
        preview_url: previewUrl,
        port: CELLD_PORT,
        host: CELLD_HOST,
        ready_at: new Date().toISOString(),
        error: null,
      };
      const r2 = loadRegistry();
      if (!r2.projects[projectId]) {
        r2.projects[projectId] = { id: projectId, versions: {}, prod_version_id: null, created_at: new Date().toISOString() };
      }
      r2.projects[projectId].versions[versionId] = version;
      if (!r2.projects[projectId].prod_version_id) r2.projects[projectId].prod_version_id = versionId;
      saveRegistry(r2);
      return json(res, 202, { ...version, poll_url: `/v1/projects/${projectId}/versions/${versionId}` });
    } catch {
      return json(res, 400, { error: "invalid json" });
    }
  }

  const versionId = parts[4];

  if (parts[1] === "projects" && parts[3] === "versions" && parts.length === 5 && req.method === "GET") {
    if (!project?.versions?.[versionId]) return json(res, 404, { error: "version_not_found" });
    return json(res, 200, project.versions[versionId]);
  }

  return json(res, 404, { error: "not found" });
});

const gatewayServer = http.createServer((req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  const ingressBase = (env.CELLP_INGRESS_BASE_DOMAIN || "ingress.local").toLowerCase();
  const hostOnly = env.INGRESS_HOST_ONLY === "1";
  const reqHost = (req.headers.host || "").split(":")[0].toLowerCase();

  if (url.pathname === "/health") {
    res.writeHead(200, { "content-type": "text/plain" });
    res.end("gateway ok");
    return;
  }

  // AD-12: Host *.{base} | *.*.{base} → celld; path/query unchanged (no strip)
  if (matchesIngressHost(reqHost, ingressBase)) {
    return proxyToCelld(req, res, url.pathname + url.search);
  }

  if (!hostOnly) {
    const m = url.pathname.match(/^\/([^/]+)\/([^/]+)(\/.*)?$/);
    if (m) {
      const rest = m[3] || "/";
      return proxyToCelld(req, res, rest);
    }
  }

  res.writeHead(404, { "content-type": "text/plain" });
  res.end("cellp dev gateway — use Host *." + ingressBase + " (path routing deprecated)");
});

const apiPort = Number(env.PLATFORM_PORT || 8790);
const gatewayPort = Number(env.GATEWAY_PORT || 8787);

apiServer.listen(apiPort, "127.0.0.1", () => {
  console.log(`mock API listening on http://127.0.0.1:${apiPort}`);
});

gatewayServer.listen(gatewayPort, "127.0.0.1", () => {
  console.log(`mock Gateway listening on http://127.0.0.1:${gatewayPort}`);
});

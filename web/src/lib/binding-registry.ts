import {
  checkDatabaseAvailability,
  getBindings,
  getProject,
  listKvNamespaces,
  listProjects,
  listQueues,
  listVersions,
  listWorkflowInstances,
  listWorkflows,
  type Bindings,
  type BindingsD1,
  type BindingsKV,
  type BindingsQueue,
  type BindingsR2,
  type BindingsWorkflow,
} from "@/lib/cellp-api";
import {
  storageBrowserHref,
  storageKvHref,
  storageQueuesHref,
  storageWorkflowsHref,
} from "@/lib/routes";

export type BindingKind = "d1" | "kv" | "queues" | "workflows" | "r2" | "cron";

export type BindingHealth = "ok" | "error" | "configured";

export interface BindingInstanceRow {
  key: string;
  projectId: string;
  versionId: string;
  isProd: boolean;
  kind: BindingKind;
  binding: string;
  name: string;
  detail?: string;
  manageHref?: string;
  health: BindingHealth;
  healthMessage?: string;
}

function rowsFromBindings(
  projectId: string,
  versionId: string,
  isProd: boolean,
  bindings: Bindings,
  kind: BindingKind,
): BindingInstanceRow[] {
  const base = {
    projectId,
    versionId,
    isProd,
    kind,
    health: "configured" as BindingHealth,
  };

  switch (kind) {
    case "d1":
      return bindings.d1.map((d: BindingsD1) => ({
        ...base,
        key: `${projectId}/${versionId}/d1/${d.binding}`,
        binding: d.binding,
        name: d.database_name,
        detail: d.database_id,
        manageHref: storageBrowserHref(projectId, versionId),
      }));
    case "kv":
      return bindings.kv.map((k: BindingsKV) => ({
        ...base,
        key: `${projectId}/${versionId}/kv/${k.id}`,
        binding: k.binding,
        name: k.id,
        manageHref: storageKvHref(projectId, versionId),
      }));
    case "queues":
      return bindings.queues.map((q: BindingsQueue) => ({
        ...base,
        key: `${projectId}/${versionId}/queue/${q.name}`,
        binding: q.binding ?? q.name,
        name: q.name,
        detail: q.consumer ? "consumer" : "producer",
        manageHref: storageQueuesHref(projectId, versionId),
      }));
    case "workflows":
      return bindings.workflows.map((w: BindingsWorkflow) => ({
        ...base,
        key: `${projectId}/${versionId}/wf/${w.name}`,
        binding: w.binding,
        name: w.name,
        detail: w.class_name,
        manageHref: storageWorkflowsHref(projectId, versionId),
      }));
    case "r2":
      return bindings.r2.map((r: BindingsR2) => ({
        ...base,
        key: `${projectId}/${versionId}/r2/${r.bucket_name}`,
        binding: r.binding,
        name: r.bucket_name,
        health: "configured",
        healthMessage: "Configured in wrangler (no object browser)",
      }));
    case "cron":
      return bindings.crons.map((expr) => ({
        ...base,
        key: `${projectId}/${versionId}/cron/${expr}`,
        binding: "cron",
        name: expr,
        health: "configured",
        healthMessage: "Scheduled in wrangler (celld cron trigger)",
      }));
    default:
      return [];
  }
}

async function probeRow(row: BindingInstanceRow): Promise<BindingInstanceRow> {
  if (row.kind === "r2" || row.kind === "cron") {
    return { ...row, health: "ok", healthMessage: row.healthMessage };
  }

  try {
    if (row.kind === "d1") {
      const avail = await checkDatabaseAvailability(row.projectId, row.versionId);
      return {
        ...row,
        health: avail.available ? "ok" : "error",
        healthMessage: avail.available ? "Database reachable" : avail.message,
      };
    }
    if (row.kind === "kv") {
      await listKvNamespaces(row.projectId, row.versionId);
      return { ...row, health: "ok", healthMessage: "Namespace reachable" };
    }
    if (row.kind === "queues") {
      await listQueues(row.projectId, row.versionId);
      return { ...row, health: "ok", healthMessage: "Queue API reachable" };
    }
    if (row.kind === "workflows") {
      try {
        const data = await listWorkflowInstances(row.projectId, row.versionId, row.name);
        if (data.limitation) {
          return {
            ...row,
            health: "configured",
            healthMessage: String(data.limitation),
          };
        }
        const count = data.instances.length;
        return {
          ...row,
          health: "ok",
          healthMessage:
            count > 0
              ? `${count} instance${count === 1 ? "" : "s"}`
              : "Registered (no instances yet)",
        };
      } catch {
        const listed = await listWorkflows(row.projectId, row.versionId);
        const found = listed.workflows.some((w) => w.workflow_name === row.name);
        if (found) {
          return {
            ...row,
            health: "configured",
            healthMessage: "Declared in wrangler (runtime probe unavailable)",
          };
        }
        return {
          ...row,
          health: "error",
          healthMessage: "Workflow not found in deployment",
        };
      }
    }
  } catch (e) {
    return {
      ...row,
      health: "error",
      healthMessage: e instanceof Error ? e.message : "Probe failed",
    };
  }
  return row;
}

/** Scan all ready deployments across projects for one binding kind. */
export async function loadBindingRegistry(
  kind: BindingKind,
  options: { probe?: boolean } = {},
): Promise<BindingInstanceRow[]> {
  const { probe = true } = options;
  const rows: BindingInstanceRow[] = [];
  const prodByProject = new Map<string, string | null>();

  let cursor: string | null = null;
  do {
    const page = await listProjects({ cursor });
    for (const project of page.projects) {
      prodByProject.set(project.id, project.prod_version_id);
      let vCursor: string | null = null;
      do {
        const versionsPage = await listVersions(project.id, { cursor: vCursor });
        for (const version of versionsPage.versions) {
          if (version.status !== "ready") continue;
          try {
            const bindings = await getBindings(project.id, version.id);
            const isProd = project.prod_version_id === version.id;
            rows.push(...rowsFromBindings(project.id, version.id, isProd, bindings, kind));
          } catch {
            /* skip versions without readable bindings */
          }
        }
        vCursor = versionsPage.next_cursor;
      } while (vCursor);
    }
    cursor = page.next_cursor;
  } while (cursor);

  // Refresh prod flags from project detail when list summary omitted prod id
  for (const row of rows) {
    if (!prodByProject.has(row.projectId)) {
      try {
        const p = await getProject(row.projectId);
        prodByProject.set(row.projectId, p.prod_version_id);
      } catch {
        prodByProject.set(row.projectId, null);
      }
    }
    row.isProd = prodByProject.get(row.projectId) === row.versionId;
  }

  rows.sort((a, b) => {
    const pc = a.projectId.localeCompare(b.projectId);
    if (pc !== 0) return pc;
    return a.versionId.localeCompare(b.versionId);
  });

  if (!probe || rows.length === 0) return rows;

  const probed = await Promise.all(rows.map((row) => probeRow(row)));
  return probed;
}

export const BINDING_KIND_LABEL: Record<BindingKind, string> = {
  d1: "D1",
  kv: "KV",
  queues: "Queues",
  workflows: "Workflows",
  r2: "R2",
  cron: "Cron",
};

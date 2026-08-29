/** Vercel-style storage browser path (Query / Data / Schema). */
export function storageBrowserHref(projectId: string, versionId: string): string {
  return `/projects/${projectId}/storage/${versionId}/browser`;
}

export function projectOverviewHref(projectId: string): string {
  return `/projects/${projectId}`;
}

export function deploymentsHref(projectId: string): string {
  return `/projects/${projectId}/deployments`;
}

export function storageHref(projectId: string): string {
  return `/projects/${projectId}/storage`;
}

export function storageKvHref(projectId: string, versionId: string): string {
  return `/projects/${projectId}/storage/${versionId}/kv`;
}

export function storageQueuesHref(projectId: string, versionId: string): string {
  return `/projects/${projectId}/storage/${versionId}/queues`;
}

export function storageWorkflowsHref(projectId: string, versionId: string): string {
  return `/projects/${projectId}/storage/${versionId}/workflows`;
}

/** Legacy helper: Bindings IA lives on Storage (DESIGN §8.5). */
export function bindingsHref(projectId: string, versionId?: string): string {
  const base = storageHref(projectId);
  return versionId ? `${base}?version=${encodeURIComponent(versionId)}` : base;
}

export function settingsHref(projectId: string): string {
  return `/projects/${projectId}/settings`;
}

export function platformHref(): string {
  return "/platform";
}

export function versionHref(projectId: string, versionId: string): string {
  return `/projects/${projectId}/versions/${versionId}`;
}

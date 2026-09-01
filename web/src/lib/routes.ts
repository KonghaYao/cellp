function seg(value: string): string {
  return encodeURIComponent(value);
}

function projectBase(projectId: string): string {
  return `/projects/${seg(projectId)}`;
}

/** Vercel-style storage browser path (Query / Data / Schema). */
export function storageBrowserHref(projectId: string, versionId: string): string {
  return `${projectBase(projectId)}/storage/${seg(versionId)}/browser`;
}

export function projectOverviewHref(projectId: string): string {
  return projectBase(projectId);
}

export function deploymentsHref(projectId: string): string {
  return `${projectBase(projectId)}/deployments`;
}

export function storageHref(projectId: string): string {
  return `${projectBase(projectId)}/storage`;
}

export function storageKvHref(projectId: string, versionId: string): string {
  return `${projectBase(projectId)}/storage/${seg(versionId)}/kv`;
}

export function storageQueuesHref(projectId: string, versionId: string): string {
  return `${projectBase(projectId)}/storage/${seg(versionId)}/queues`;
}

export function storageWorkflowsHref(projectId: string, versionId: string): string {
  return `${projectBase(projectId)}/storage/${seg(versionId)}/workflows`;
}

export type StorageSurface = "browser" | "kv" | "queues" | "workflows";

/** Which storage sub-page the pathname refers to (for version switcher). */
export function storageSurfaceFromPathname(pathname: string): StorageSurface {
  if (/\/storage\/[^/]+\/workflows(?:\/|$)/.test(pathname)) return "workflows";
  if (/\/storage\/[^/]+\/queues(?:\/|$)/.test(pathname)) return "queues";
  if (/\/storage\/[^/]+\/kv(?:\/|$)/.test(pathname)) return "kv";
  return "browser";
}

export function storageHrefForSurface(
  projectId: string,
  versionId: string,
  surface: StorageSurface,
): string {
  switch (surface) {
    case "kv":
      return storageKvHref(projectId, versionId);
    case "queues":
      return storageQueuesHref(projectId, versionId);
    case "workflows":
      return storageWorkflowsHref(projectId, versionId);
    default:
      return storageBrowserHref(projectId, versionId);
  }
}

export function versionHrefForStoragePathname(
  projectId: string,
  versionId: string,
  pathname: string,
): string {
  return storageHrefForSurface(
    projectId,
    versionId,
    storageSurfaceFromPathname(pathname),
  );
}

/** Legacy `/bindings` → Storage; with a version, open that deployment's D1 browser. */
export function bindingsHref(projectId: string, versionId?: string): string {
  if (versionId) {
    return storageBrowserHref(projectId, versionId);
  }
  return storageHref(projectId);
}

export function settingsHref(projectId: string): string {
  return `${projectBase(projectId)}/settings`;
}

export function inspectHref(projectId: string): string {
  return `${projectBase(projectId)}/inspect`;
}

export function platformHref(projectFilter?: string): string {
  if (!projectFilter) return "/platform";
  return `/platform?project=${encodeURIComponent(projectFilter)}`;
}

export function bindingsHubHref(): string {
  return "/bindings";
}

export function bindingsD1Href(): string {
  return "/bindings/d1";
}

export function bindingsKvHref(): string {
  return "/bindings/kv";
}

export function bindingsQueuesHref(): string {
  return "/bindings/queues";
}

export function bindingsWorkflowsHref(): string {
  return "/bindings/workflows";
}

export function bindingsR2Href(): string {
  return "/bindings/r2";
}

export function bindingsCronHref(): string {
  return "/bindings/cron";
}

export function versionHref(projectId: string, versionId: string): string {
  return `${projectBase(projectId)}/versions/${seg(versionId)}`;
}

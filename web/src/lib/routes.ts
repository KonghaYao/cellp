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

export function settingsHref(projectId: string): string {
  return `/projects/${projectId}/settings`;
}

export function versionHref(projectId: string, versionId: string): string {
  return `/projects/${projectId}/versions/${versionId}`;
}

/** Projects with a full HTML storefront (iframe / external browse). */
export const HTML_STOREFRONT_PROJECTS = new Set(["commerce-store"]);

export function projectHasHtmlStorefront(projectId: string): boolean {
  return HTML_STOREFRONT_PROJECTS.has(projectId);
}

/**
 * Map ingress Host to Dashboard path for API-only workers (dev safety net).
 * Returns null when the host should stay on gateway (commerce-store, unknown).
 */
export function ingressHostToDashboardPath(
  host: string,
  baseDomain = "ingress.local",
): string | null {
  const h = host.trim().toLowerCase();
  const suffix = `.${baseDomain.toLowerCase()}`;
  if (!h.endsWith(suffix)) return null;

  const labels = h.slice(0, -suffix.length).split(".").filter(Boolean);
  if (labels.length === 1) {
    const projectId = labels[0]!;
    if (projectHasHtmlStorefront(projectId)) return null;
    return `/projects/${encodeURIComponent(projectId)}`;
  }
  if (labels.length === 2) {
    const [versionId, projectId] = labels;
    if (projectHasHtmlStorefront(projectId)) return null;
    return `/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}`;
  }
  return null;
}

export function isAppInternalPath(url: string): boolean {
  return url.startsWith("/") && !url.startsWith("//");
}

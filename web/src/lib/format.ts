const GATEWAY_DEFAULT = "http://127.0.0.1:8787";

export function gatewayBase(): string {
  const configured = import.meta.env.VITE_CELLP_GATEWAY_URL as string | undefined;
  if (configured === "") return "/__gateway";
  if (configured) return configured.replace(/\/$/, "");
  if (import.meta.env.DEV) return "/__gateway";
  return GATEWAY_DEFAULT;
}

export function ingressBaseDomain(): string {
  const fromEnv = import.meta.env.VITE_CELLP_INGRESS_BASE_DOMAIN as
    | string
    | undefined;
  return (fromEnv?.trim() || "ingress.local").toLowerCase();
}

export function previewHost(projectId: string, versionId: string): string {
  return `${versionId}.${projectId}.${ingressBaseDomain()}`.toLowerCase();
}

export function prodHost(projectId: string): string {
  return `${projectId}.${ingressBaseDomain()}`.toLowerCase();
}

/** Host from API preview_url (http://host/…) or build from project/version. */
export function previewHostFromApi(
  projectId: string,
  versionId: string,
  previewUrl?: string | null,
): string {
  if (previewUrl) {
    try {
      return new URL(previewUrl).host;
    } catch {
      /* fall through */
    }
  }
  return previewHost(projectId, versionId);
}

/**
 * URL the browser can open in dev: Vite `/__gateway` proxy + `__cellp_host` (AD-12).
 * In production builds, uses canonical host URL from API or derived host.
 */
export function gatewayBrowseUrl(
  host: string,
  path = "/",
): string {
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  const h = host.trim();
  if (!h) return gatewayBase() + normalizedPath;

  if (import.meta.env.DEV && gatewayBase() === "/__gateway") {
    const q = new URLSearchParams({ __cellp_host: h });
    const sep = normalizedPath.includes("?") ? "&" : "?";
    return `/__gateway${normalizedPath}${sep}${q.toString()}`;
  }

  const scheme =
    (import.meta.env.VITE_CELLP_PUBLIC_SCHEME_PREVIEW as string | undefined) ||
    "http";
  return `${scheme}://${h}${normalizedPath}`;
}

export function previewBrowseUrl(
  projectId: string,
  versionId: string,
  previewUrl?: string | null,
): string {
  const host = previewHostFromApi(projectId, versionId, previewUrl);
  let path = "/";
  if (previewUrl) {
    try {
      path = new URL(previewUrl).pathname || "/";
    } catch {
      /* default */
    }
  }
  return gatewayBrowseUrl(host, path);
}

export function prodBrowseUrl(
  projectId: string,
  prodUrl?: string | null,
): string {
  if (prodUrl) {
    try {
      const u = new URL(prodUrl);
      if (u.host && !u.pathname.includes(`/${projectId}/`)) {
        return gatewayBrowseUrl(u.host, u.pathname || "/");
      }
    } catch {
      /* fall through */
    }
  }
  return gatewayBrowseUrl(prodHost(projectId), "/");
}

/** Derive production browse URL (AD-12 Host, not path /{project}/). */
export function deriveProdUrl(
  projectId: string,
  _previewUrl?: string | null,
): string {
  return prodBrowseUrl(projectId, null);
}

/** Prefer API-provided prod_url; fall back to Host-based browse URL. */
export function resolveProdUrl(
  projectId: string,
  prodUrl?: string | null,
  _previewUrl?: string | null,
): string {
  if (prodUrl) {
    try {
      const u = new URL(prodUrl);
      if (u.host.includes(".")) {
        return gatewayBrowseUrl(u.host, u.pathname || "/");
      }
    } catch {
      /* legacy path URL */
    }
    if (prodUrl.includes("ingress.local") || prodUrl.startsWith("http")) {
      return prodBrowseUrl(projectId, prodUrl);
    }
  }
  return prodBrowseUrl(projectId, prodUrl);
}

/** Human-readable URL for UI labels (not for href). */
export function ingressDisplayUrl(
  projectId: string,
  versionId?: string,
  apiUrl?: string | null,
): string {
  if (apiUrl?.trim()) return apiUrl.trim();
  if (versionId) return `http://${previewHost(projectId, versionId)}/`;
  return `http://${prodHost(projectId)}/`;
}

export function truncateSha(sha: string, length = 7): string {
  if (!sha) return "—";
  return sha.length <= length ? sha : sha.slice(0, length);
}

function isReasonableDate(date: Date): boolean {
  const year = date.getFullYear();
  return year >= 1970 && year <= 2100;
}

export function formatRelativeTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime()) || !isReasonableDate(date)) return "Unknown";

  const now = Date.now();
  const diffMs = now - date.getTime();
  if (diffMs < 0) return "Unknown";
  const diffSec = Math.round(diffMs / 1000);

  if (diffSec < 5) return "just now";
  if (diffSec < 60) return `${diffSec}s ago`;

  const diffMin = Math.round(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;

  const diffHr = Math.round(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;

  const diffDay = Math.round(diffHr / 24);
  if (diffDay < 30) return `${diffDay}d ago`;

  const diffMonth = Math.round(diffDay / 30);
  if (diffMonth < 12) return `${diffMonth}mo ago`;

  const diffYear = Math.round(diffMonth / 12);
  return `${diffYear}y ago`;
}

export function formatDuration(
  startIso: string | null | undefined,
  endIso: string | null | undefined,
): string {
  if (!startIso) return "—";
  const start = new Date(startIso).getTime();
  const end = endIso ? new Date(endIso).getTime() : Date.now();
  if (Number.isNaN(start) || Number.isNaN(end)) return "—";

  const ms = Math.max(0, end - start);
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;

  const min = Math.floor(sec / 60);
  const remSec = sec % 60;
  if (min < 60) return remSec > 0 ? `${min}m ${remSec}s` : `${min}m`;

  const hr = Math.floor(min / 60);
  const remMin = min % 60;
  return remMin > 0 ? `${hr}h ${remMin}m` : `${hr}h`;
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

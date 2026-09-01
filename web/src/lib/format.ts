import {
  isAppInternalPath,
  projectHasHtmlStorefront,
} from "@/lib/ingress-routing";
import { versionHref } from "@/lib/routes";

const GATEWAY_DEFAULT = "http://127.0.0.1:8787";

export { isAppInternalPath, projectHasHtmlStorefront };

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

/** nip.io / sslip.io — public DNS embeds LAN IP; colleagues skip /etc/hosts. */
export function ingressUsesMagicDns(): boolean {
  const base = ingressBaseDomain();
  return (
    base.endsWith(".nip.io") ||
    base.endsWith(".sslip.io") ||
    base.endsWith(".xip.io")
  );
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

/** Workers with a full HTML storefront (iframe). API-only demos return false. */
export function workerHasHtmlStorefront(projectId: string): boolean {
  return projectHasHtmlStorefront(projectId);
}

export function dashboardProjectPath(projectId: string): string {
  return `/projects/${encodeURIComponent(projectId)}`;
}

export function dashboardVersionPath(
  projectId: string,
  versionId: string,
): string {
  return versionHref(projectId, versionId);
}

/**
 * Where Primary UI links (Preview / Production) should go.
 * API-only workers open Dashboard; HTML storefronts open gateway proxy.
 */
export function previewOpenUrl(
  projectId: string,
  versionId: string,
  previewUrl?: string | null,
): string {
  if (!workerHasHtmlStorefront(projectId)) {
    return dashboardVersionPath(projectId, versionId);
  }
  return previewBrowseUrl(projectId, versionId, previewUrl, "/");
}

export function productionOpenUrl(
  projectId: string,
  prodUrl?: string | null,
): string {
  if (!workerHasHtmlStorefront(projectId)) {
    return dashboardProjectPath(projectId);
  }
  return prodBrowseUrl(projectId, prodUrl, "/");
}

/** Gateway URL for prod traffic (curl / API clients). Always Host-based AD-12. */
export function workerProductionGatewayUrl(
  projectId: string,
  prodUrl?: string | null,
): string {
  return prodBrowseUrl(projectId, prodUrl, "/");
}

/** Gateway URL for preview traffic. */
export function workerPreviewGatewayUrl(
  projectId: string,
  versionId: string,
  previewUrl?: string | null,
): string {
  return previewBrowseUrl(projectId, versionId, previewUrl, "/");
}

/** Link label in version URLs section — matches where primary href goes. */
export function previewPrimaryLinkLabel(
  projectId: string,
  versionId: string,
  previewUrl?: string | null,
): string {
  if (!workerHasHtmlStorefront(projectId)) {
    return "Open version in Dashboard";
  }
  return ingressDisplayUrl(projectId, versionId, previewUrl);
}

export function productionPrimaryLinkLabel(
  projectId: string,
  prodUrl?: string | null,
): string {
  if (!workerHasHtmlStorefront(projectId)) {
    return "Open project in Dashboard";
  }
  return ingressDisplayUrl(projectId, undefined, prodUrl);
}

/** Raw Worker HTTP via dev gateway proxy (API JSON/HTML). */
export function gatewayWorkerUrl(
  projectId: string,
  versionId: string | null,
  path = "/",
  opts?: { prod?: boolean; previewUrl?: string | null; prodUrl?: string | null },
): string {
  const host =
    opts?.prod || !versionId
      ? prodHost(projectId)
      : previewHostFromApi(projectId, versionId, opts?.previewUrl);
  return gatewayBrowseUrl(host, path);
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

  if (gatewayBase() === "/__gateway") {
    const q = new URLSearchParams({ __cellp_host: h });
    const sep = normalizedPath.includes("?") ? "&" : "?";
    return `/__gateway${normalizedPath}${sep}${q.toString()}`;
  }

  const scheme =
    (import.meta.env.VITE_CELLP_PUBLIC_SCHEME_PREVIEW as string | undefined) ||
    "http";
  return withGatewayPortIfNeeded(`${scheme}://${h}${normalizedPath}`);
}

export function previewBrowseUrl(
  projectId: string,
  versionId: string,
  previewUrl?: string | null,
  pathOverride?: string,
): string {
  const host = previewHostFromApi(projectId, versionId, previewUrl);
  let path = pathOverride ?? "/";
  if (!pathOverride && previewUrl) {
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
  pathOverride = "/",
): string {
  if (prodUrl) {
    try {
      const u = new URL(prodUrl);
      if (u.host && !u.pathname.includes(`/${projectId}/`)) {
        return gatewayBrowseUrl(u.host, pathOverride || u.pathname || "/");
      }
    } catch {
      /* fall through */
    }
  }
  return gatewayBrowseUrl(prodHost(projectId), pathOverride);
}

/** Derive production browse URL (AD-12 Host, not path /{project}/). */
export function deriveProdUrl(
  projectId: string,
  _previewUrl?: string | null,
): string {
  return productionOpenUrl(projectId, null);
}

/** Prefer API-provided prod_url; fall back to Host-based browse URL. */
export function resolveProdUrl(
  projectId: string,
  prodUrl?: string | null,
  _previewUrl?: string | null,
): string {
  return productionOpenUrl(projectId, prodUrl);
}

/** TCP port on which clients reach Gateway (dev default 8787). */
export function gatewayPublicPort(): number {
  const configured = import.meta.env.VITE_CELLP_GATEWAY_URL as string | undefined;
  const fallback =
    configured?.trim() ||
    (import.meta.env.DEV ? "http://127.0.0.1:8787" : GATEWAY_DEFAULT);
  try {
    const u = new URL(fallback);
    if (u.port) return Number.parseInt(u.port, 10);
    return u.protocol === "https:" ? 443 : 80;
  } catch {
    return 8787;
  }
}

/** Append Gateway port when API URL omits it (non-80/443). */
export function withGatewayPortIfNeeded(url: string): string {
  try {
    const u = new URL(url);
    if (u.port) return url.trim();
    const port = gatewayPublicPort();
    const scheme = u.protocol.replace(":", "");
    if (
      (scheme === "http" && port === 80) ||
      (scheme === "https" && port === 443)
    ) {
      return url.trim();
    }
    u.port = String(port);
    return u.toString();
  } catch {
    return url;
  }
}

/** Suggested /etc/hosts line for LAN colleagues (Host → your machine). */
export function ingressLanHostsLine(
  projectId: string,
  versionId?: string,
  lanIp = "<你的局域网 IP>",
): string {
  const hosts = versionId
    ? `${previewHost(projectId, versionId)} ${prodHost(projectId)}`
    : prodHost(projectId);
  return `${lanIp} ${hosts}`;
}
/** Human-readable URL for UI labels (not for href). Includes Gateway port when non-default. */
export function ingressDisplayUrl(
  projectId: string,
  versionId?: string,
  apiUrl?: string | null,
): string {
  if (apiUrl?.trim()) return withGatewayPortIfNeeded(apiUrl.trim());
  if (versionId) {
    return withGatewayPortIfNeeded(
      `http://${previewHost(projectId, versionId)}/`,
    );
  }
  return withGatewayPortIfNeeded(`http://${prodHost(projectId)}/`);
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

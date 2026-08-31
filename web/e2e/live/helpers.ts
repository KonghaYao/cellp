import { test } from "@playwright/test";

export function liveApiBase(): string {
  return (process.env.VITE_CELLP_API_URL || "http://127.0.0.1:8790").replace(
    /\/$/,
    "",
  );
}

export function liveAdminToken(): string {
  return (
    process.env.VITE_CELLP_ADMIN_TOKEN ||
    process.env.CELLP_ADMIN_TOKEN ||
    "dev-local-token"
  );
}

export async function resolveLiveProjectId(): Promise<string> {
  const explicit = process.env.CELLP_LIVE_PROJECT?.trim();
  if (explicit) return explicit;

  const base = liveApiBase();
  const token = liveAdminToken();
  const res = await fetch(`${base}/v1/projects?limit=20`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) {
    test.skip(true, `list projects failed ${res.status}`);
  }
  const body = (await res.json()) as { projects?: { id: string }[] };
  const id = body.projects?.[0]?.id;
  if (!id) {
    test.skip(true, "no projects in registry — seed with dev/scripts/seed-demo.sh or seed-commerce-store.sh");
  }
  return id;
}

/** Skip the test when local cellpd is not reachable. */
export async function skipUnlessLiveStack(): Promise<void> {
  const base = liveApiBase();
  const token = liveAdminToken();
  try {
    const res = await fetch(`${base}/v1/health`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
    if (!res.ok) {
      test.skip(true, `cellpd health ${res.status} at ${base}`);
    }
  } catch {
    test.skip(true, `cellpd not reachable at ${base} — run ./dev/scripts/up.sh`);
  }
}

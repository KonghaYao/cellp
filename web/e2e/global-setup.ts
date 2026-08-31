import type { FullConfig } from "@playwright/test";

export default async function globalSetup(_config: FullConfig) {
  const port = process.env.MOCK_CELLP_PORT || "9876";
  const resetUrl = `http://127.0.0.1:${port}/v1/__e2e_reset__`;
  const token = process.env.VITE_CELLP_ADMIN_TOKEN || "test-admin-token";
  let lastError: unknown;
  for (let attempt = 0; attempt < 60; attempt++) {
    try {
      const res = await fetch(resetUrl, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) return;
      lastError = `${res.status} ${await res.text()}`;
    } catch (e) {
      lastError = e;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`mock e2e reset failed: ${String(lastError)}`);
}

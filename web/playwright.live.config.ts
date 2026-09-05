import { defineConfig, devices } from "@playwright/test";

/**
 * Live stack E2E — requires cellpd on :8790 (see web/scripts/test-e2e-live.sh).
 * Does not start mock-api-server.
 */
const API_URL = (process.env.VITE_CELLP_API_URL || "http://127.0.0.1:8790").replace(
  /\/$/,
  "",
);
const PREVIEW_PORT = process.env.CELLP_E2E_PREVIEW_PORT || "4174";

export default defineConfig({
  testDir: "./e2e/live",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: "list",
  timeout: 60_000,
  use: {
    baseURL: `http://127.0.0.1:${PREVIEW_PORT}`,
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "VITE_BASE=/ pnpm run build && pnpm run preview -- --port " + PREVIEW_PORT,
    url: `http://127.0.0.1:${PREVIEW_PORT}`,
    reuseExistingServer: !process.env.CI,
    env: {
      VITE_BASE: "/",
      VITE_CELLP_API_URL: API_URL,
      VITE_CELLP_ADMIN_TOKEN:
        process.env.VITE_CELLP_ADMIN_TOKEN ||
        process.env.CELLP_ADMIN_TOKEN ||
        "dev-local-token",
      VITE_CELLP_PAGE_SIZE: "50",
    },
  },
});

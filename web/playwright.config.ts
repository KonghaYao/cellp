import { defineConfig, devices } from "@playwright/test";

const MOCK_PORT = process.env.MOCK_CELLP_PORT || "9876";
const API_URL = `http://127.0.0.1:${MOCK_PORT}`;
const PREVIEW_PORT = "4173";

export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  testMatch: "**/*.spec.ts",
  testIgnore: "**/live/**",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: `http://127.0.0.1:${PREVIEW_PORT}`,
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "node e2e/mock-api-server.mjs",
      url: `${API_URL}/v1/health`,
      reuseExistingServer: true,
      env: {
        MOCK_CELLP_PORT: MOCK_PORT,
        MOCK_PAGE_SIZE: "2",
        VITE_CELLP_ADMIN_TOKEN: "test-admin-token",
      },
    },
    {
      command: "VITE_BASE=/ pnpm run build && pnpm run preview",
      url: `http://127.0.0.1:${PREVIEW_PORT}`,
      reuseExistingServer: true,
      env: {
        VITE_BASE: "/",
        VITE_CELLP_API_URL: API_URL,
        VITE_CELLP_ADMIN_TOKEN: "test-admin-token",
        VITE_CELLP_PAGE_SIZE: "2",
      },
    },
  ],
});

import { test, expect } from "@playwright/test";

test.describe("Platform health page", () => {
  test("shows metrics and runtime routes", async ({ page }) => {
    await page.goto("/platform");
    await expect(page.getByRole("heading", { name: "Platform health" })).toBeVisible();
    await expect(page.getByText("Pending jobs")).toBeVisible();
    await expect(page.getByText("Active routes")).toBeVisible();
    await expect(page.getByText("Runtime routes")).toBeVisible();
    await expect(page.getByRole("cell", { name: "demo-app" }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});

import { test, expect } from "@playwright/test";

test.describe("Storage hub (TP-UI-7, TP-UI-12)", () => {
  test("ready version row shows d1 / kv / queue / workflow / r2 / cron badges", async ({
    page,
  }) => {
    await page.goto("/projects/demo-app/storage");
    await expect(page.getByRole("heading", { name: "Storage" })).toBeVisible();

    const row = page.locator("#version-v1");
    await expect(row.getByText("v1", { exact: true })).toBeVisible();

    for (const type of ["d1", "kv", "queue", "workflow", "r2", "cron"]) {
      await expect(row.locator(`[data-binding-type="${type}"]`)).toBeVisible();
    }
  });

  test("R2 and Cron are not links; no /storage/:vid/r2 or /cron routes", async ({
    page,
  }) => {
    await page.goto("/projects/demo-app/storage");
    const row = page.locator("#version-v1");
    await expect(row.locator('a:has([data-binding-type="r2"])')).toHaveCount(0);
    await expect(row.locator('a:has([data-binding-type="cron"])')).toHaveCount(0);

    await page.goto("/projects/demo-app/storage/v1/r2");
    await expect(page.getByRole("heading", { name: "KV" })).not.toBeVisible();
    await expect(page.getByRole("heading", { name: "main" })).not.toBeVisible();

    await page.goto("/projects/demo-app/storage/v1/cron");
    await expect(page.getByRole("heading", { name: "KV" })).not.toBeVisible();
    await expect(page.getByRole("heading", { name: "main" })).not.toBeVisible();
  });
});

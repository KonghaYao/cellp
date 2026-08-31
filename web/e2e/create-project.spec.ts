import { test, expect } from "@playwright/test";

test.describe("Create project (TP-UI-14 mock)", () => {
  test("creates project and opens overview", async ({ page }) => {
    const id = `new-proj-${Date.now().toString(36).slice(-6)}`;
    await page.goto("/");
    await page.getByRole("button", { name: "New project" }).click();
    await page.getByLabel("Project id").fill(id);
    await page.getByRole("button", { name: "Create" }).click();
    await expect(page).toHaveURL(new RegExp(`/projects/${id}$`));
    await expect(page.getByRole("heading", { name: id })).toBeVisible();
  });
});

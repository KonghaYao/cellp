import { test, expect } from "@playwright/test";
import {
  resolveLiveProjectId,
  skipUnlessLiveStack,
} from "./helpers";

/**
 * TP-UI-14 — Operator journey against real cellpd (not mock).
 * Read-only navigation; does not promote or destroy.
 */
test.describe("Operator loop (live cellpd)", () => {
  test.beforeEach(async () => {
    await skipUnlessLiveStack();
  });

  test("projects → deployments → version → storage → platform", async ({
    page,
  }) => {
    const projectId = await resolveLiveProjectId();

    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Projects" })).toBeVisible({
      timeout: 15_000,
    });

    const projectLink = page.getByRole("link", { name: projectId, exact: true });
    if (!(await projectLink.isVisible().catch(() => false))) {
      await page.getByPlaceholder("Search projects…").fill(projectId);
      await page.waitForTimeout(400);
      await expect(projectLink).toBeVisible({ timeout: 15_000 });
    }

    await projectLink.click();
    await expect(page.getByRole("heading", { name: projectId })).toBeVisible();

    await page.getByRole("link", { name: "Deployments" }).first().click();
    await expect(
      page.getByRole("heading", { name: "Versions", level: 1 }),
    ).toBeVisible();

    const firstVersionLink = page
      .locator("table tbody tr")
      .first()
      .getByRole("link")
      .first();
    await expect(firstVersionLink).toBeVisible({ timeout: 10_000 });
    const versionId = (await firstVersionLink.textContent())?.trim() ?? "";
    expect(versionId.length).toBeGreaterThan(0);

    await firstVersionLink.click();
    await expect(page.getByRole("heading", { name: versionId })).toBeVisible();

    await page.getByRole("link", { name: "Storage" }).first().click();
    await expect(
      page.getByRole("heading", { name: "Storage", level: 1 }),
    ).toBeVisible();

    await page.goto(`/projects/${projectId}/inspect`);
    await expect(
      page.getByRole("heading", { name: "Inspect", level: 1 }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText("Ready versions")).toBeVisible();

    await page.goto("/platform");
    await expect(
      page.getByRole("heading", { name: "Platform health" }),
    ).toBeVisible();
    await expect(page.getByText("Pending jobs")).toBeVisible();
  });
});

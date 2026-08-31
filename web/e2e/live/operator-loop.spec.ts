import { test, expect } from "@playwright/test";
import {
  liveProjectId,
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
    const projectId = liveProjectId();

    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Projects" })).toBeVisible({
      timeout: 15_000,
    });
    await expect(page.getByText(projectId)).toBeVisible({ timeout: 15_000 });

    await page.getByRole("link", { name: projectId }).click();
    await expect(page.getByRole("heading", { name: projectId })).toBeVisible();

    await page.getByRole("link", { name: "Deployments" }).click();
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

    await page.getByRole("link", { name: "Storage" }).click();
    await expect(
      page.getByRole("heading", { name: "Storage", level: 1 }),
    ).toBeVisible();

    await page.goto(`/projects/${projectId}/storage/${versionId}/browser`);
    await expect(page.getByRole("tab", { name: "Schema" })).toBeVisible({
      timeout: 15_000,
    });

    await page.goto("/platform");
    await expect(
      page.getByRole("heading", { name: "Platform health" }),
    ).toBeVisible();
    await expect(page.getByText("Pending jobs")).toBeVisible();
  });
});

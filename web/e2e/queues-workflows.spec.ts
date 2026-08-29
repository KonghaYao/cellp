import { test, expect } from "@playwright/test";

test.describe("TP-UI-8 queue console", () => {
  test("lists tasks, peeks base64 body, and does not purge without confirm", async ({
    page,
  }) => {
    const purgePosts: string[] = [];
    page.on("request", (req) => {
      if (req.method() === "POST" && req.url().includes("/purge")) {
        purgePosts.push(req.url());
      }
    });

    await page.goto("/projects/demo-app/storage/v1/queues");
    await expect(page.getByRole("heading", { name: "Queues" })).toBeVisible();
    await expect(page.getByText("tasks").first()).toBeVisible();

    const peek = page.getByTestId("queue-peek");
    await expect(peek).toBeVisible();
    await expect(peek).toContainText("aGVsbG8tcXVldWU=");
    await expect(peek).toContainText("hello-queue");

    const info = page.getByTestId("queue-info");
    await expect(info).toContainText("Backlog");
    await expect(info).toContainText("Delivery");
    await expect(info.getByText("Delivering")).toBeVisible();

    await expect(page.getByRole("button", { name: /^Pull$/i })).toHaveCount(0);

    await page.getByRole("button", { name: "Purge" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("heading", { name: "Purge queue?" })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Purge with force" })).toBeDisabled();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toHaveCount(0);

    await expect(peek).toContainText("hello-queue");
    expect(purgePosts).toHaveLength(0);
  });

  test("preview queue shows AD-7 banner and empty peek", async ({ page }) => {
    await page.goto("/projects/demo-app/storage/v2/queues");
    await expect(page.getByTestId("ad7-banner")).toBeVisible();
    await expect(page.getByTestId("ad7-banner")).toContainText(
      "do not inherit Production",
    );
    await expect(page.getByTestId("queue-peek")).toContainText(
      "No messages in the peek window.",
    );
  });
});

test.describe("TP-UI-9 workflow instances", () => {
  test("lists report-builder with a read-only instance row", async ({ page }) => {
    await page.goto("/projects/demo-app/storage/v1/workflows");
    await expect(page.getByRole("heading", { name: "Workflows" })).toBeVisible();
    await expect(page.getByText("report-builder").first()).toBeVisible();

    const table = page.getByTestId("workflow-instances");
    await expect(table).toBeVisible();
    await expect(table).toContainText("aaaaaaaaaaaaaaaa");
    await expect(table).toContainText("running");

    await expect(page.getByRole("button", { name: /^Pause$/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Resume$/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Restart$/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Delete$/i })).toHaveCount(0);
  });

  test("shows limitation when the API returns it", async ({ page }) => {
    await page.goto("/projects/demo-app/storage/v2/workflows");
    await expect(page.getByTestId("workflow-limitation")).toBeVisible();
    await expect(page.getByTestId("workflow-limitation")).toContainText(
      "无法按 workflow 名精确过滤",
    );
    await expect(page.getByTestId("workflow-instances")).toContainText("No instances");
  });
});

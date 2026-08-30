import { test, expect } from "@playwright/test";

const kvPage = (vid: string) => `/projects/demo-app/storage/${vid}/kv`;

test.describe("KV browser (TP-UI-7)", () => {
  test("v1 lists greeting, get shows value, and put adds a key", async ({
    page,
  }) => {
    await page.goto(kvPage("v1"));
    await expect(page.getByRole("heading", { name: "KV" })).toBeVisible();
    const list = page.getByTestId("kv-key-list");
    await expect(list.getByRole("button", { name: "greeting", exact: true })).toBeVisible();
    await expect(page.getByTestId("kv-info")).toContainText("Live");

    await list.getByRole("button", { name: "greeting", exact: true }).click();
    const editor = page.getByTestId("kv-key-editor");
    await expect(editor.getByRole("textbox", { name: "Key value" })).toHaveValue(
      "hello-prod",
    );
    await expect(editor.getByText("utf-8", { exact: true })).toBeVisible();

    const put = page.getByTestId("kv-put-form");
    await put.getByLabel("Key name").fill("e2e-put");
    await put.getByRole("textbox", { name: "Put value" }).fill("hello-e2e");
    await put.getByRole("button", { name: "Put key" }).click();
    await expect(list.getByRole("button", { name: "e2e-put", exact: true })).toBeVisible();
    await expect(editor.getByRole("textbox", { name: "Key value" })).toHaveValue(
      "hello-e2e",
    );
  });

  test("prefix filter and cursor pagination", async ({ page }) => {
    await page.goto(kvPage("v1"));
    const list = page.getByTestId("kv-key-list");
    await expect(list.getByRole("button", { name: "greeting" })).toBeVisible();
    await expect(list.getByRole("button", { name: "item-2" })).not.toBeVisible();

    await list.getByRole("button", { name: "Next" }).click();
    await expect(list.getByRole("button", { name: "item-2" })).toBeVisible();
    await expect(list.getByRole("button", { name: "greeting" })).not.toBeVisible();

    await list.getByRole("button", { name: "Previous" }).click();
    await expect(list.getByRole("button", { name: "greeting" })).toBeVisible();

    await page.getByLabel("Prefix").fill("item");
    await page.getByRole("button", { name: "Filter" }).click();
    await expect(list.getByRole("button", { name: "item-1" })).toBeVisible();
    await expect(list.getByRole("button", { name: "greeting" })).not.toBeVisible();
  });

  test("v2 inherits prod greeting via branch; sibling writes stay isolated (TP-UI-11)", async ({
    page,
  }) => {
    await page.goto(kvPage("v2"));
    await expect(page.getByTestId("ad7-banner")).toBeVisible();
    await expect(page.getByTestId("ad7-banner")).toContainText("inherit");

    const list = page.getByTestId("kv-key-list");
    await expect(list.getByRole("button", { name: "greeting", exact: true })).toBeVisible();

    await list.getByRole("button", { name: "greeting", exact: true }).click();
    await expect(
      page.getByTestId("kv-key-editor").getByRole("textbox", { name: "Key value" }),
    ).toHaveValue("hello-prod");

    const put = page.getByTestId("kv-put-form");
    await put.getByLabel("Key name").fill("v2-only");
    await put.getByRole("textbox", { name: "Put value" }).fill("sibling-isolated");
    await put.getByRole("button", { name: "Put key" }).click();
    const editor = page.getByTestId("kv-key-editor");
    await expect(editor.getByRole("textbox", { name: "Key value" })).toHaveValue(
      "sibling-isolated",
    );

    await page.goto(kvPage("v1"));
    await page.getByLabel("Prefix").fill("v2-only");
    await page.getByRole("button", { name: "Filter" }).click();
    await expect(page.getByText("No keys match this prefix")).toBeVisible();
  });

  test("version switcher stays on the KV page", async ({ page }) => {
    await page.goto(kvPage("v1"));
    await page.locator('[aria-haspopup="listbox"]').click();
    await page.getByRole("listbox").getByRole("option", { name: /^v2\b/ }).click();
    await expect(page).toHaveURL(/\/storage\/v2\/kv/);
    await expect(page.getByTestId("ad7-banner")).toBeVisible();
  });

  test("version_not_ready shows the not-ready empty state", async ({ page }) => {
    await page.goto("/projects/extra-app/storage/v-pending/kv");
    await expect(
      page.getByRole("heading", { name: "Deployment not ready" }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "Back to storage" })).toBeVisible();
  });
});

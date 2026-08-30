import { test, expect } from "@playwright/test";

const storageBrowser = (vid: string) =>
  `/projects/demo-app/storage/${vid}/browser`;

test.describe("cellp dashboard smoke (TP-UI-1..5)", () => {
  test("project list renders", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Projects" })).toBeVisible();
    await expect(page.getByText("demo-app")).toBeVisible();
    await expect(page.getByText("5 deployments")).toBeVisible();
  });

  test("project list cursor pagination", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText("demo-app")).toBeVisible();
    await expect(page.getByText("extra-app")).toBeVisible();
    await expect(page.getByText("third-app")).not.toBeVisible();

    await page.getByRole("button", { name: "Load more" }).click();
    await expect(page.getByText("third-app")).toBeVisible();
  });

  test("version list for project", async ({ page }) => {
    await page.goto("/projects/demo-app/deployments");
    await expect(
      page.getByRole("heading", { name: "Versions", level: 1 }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "v5" })).toBeVisible();
    await expect(page.getByRole("link", { name: "v4" })).toBeVisible();
    await expect(page.getByRole("link", { name: "v1" })).not.toBeVisible();

    await page.getByRole("button", { name: "Load more" }).click();
    await page.getByRole("button", { name: "Load more" }).click();
    await expect(page.getByRole("link", { name: "v1" })).toBeVisible();
    await expect(
      page
        .locator("tr")
        .filter({ has: page.getByRole("link", { name: "v1", exact: true }) })
        .getByText("Production"),
    ).toBeVisible();
  });

  test("version list cursor pagination", async ({ page }) => {
    await page.goto("/projects/demo-app/deployments");
    await expect(page.getByRole("link", { name: "v5" })).toBeVisible();
    await expect(page.getByRole("link", { name: "v1" })).not.toBeVisible();

    await page.getByRole("button", { name: "Load more" }).click();
    await expect(page.getByRole("link", { name: "v3" })).toBeVisible();
    await page.getByRole("button", { name: "Load more" }).click();
    await expect(page.getByRole("link", { name: "v1" })).toBeVisible();
  });

  test("version detail shows actions", async ({ page }) => {
    await page.goto("/projects/demo-app/versions/v2");
    await expect(page.getByRole("heading", { name: "v2" })).toBeVisible();
    await expect(page.getByText("Ready").first()).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Promote to prod" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Destroy" })).toBeVisible();
    await expect(page.getByTestId("worker-env-editor")).toBeVisible();
  });

  test("settings edits worker env", async ({ page }) => {
    await page.goto("/projects/demo-app/settings");
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    const editor = page.getByTestId("worker-env-editor");
    await expect(editor).toBeVisible();
    await expect(editor.locator('input[value="PROJECT_ID"]')).toBeVisible();
    await editor.getByRole("button", { name: "Add variable" }).click();
    const keys = editor.getByLabel(/Env key/);
    await keys.last().fill("DASH_KEY");
    const values = editor.getByLabel(/Env value/);
    await values.last().fill("from-dashboard");
    await editor.getByRole("button", { name: "Save" }).click();
    await expect(editor.getByText("Saved.")).toBeVisible();
    await expect(editor.locator('input[value="DASH_KEY"]')).toBeVisible();
  });

  test("promote updates prod pointer", async ({ page }) => {
    await page.goto("/projects/demo-app/versions/v2");
    await page.getByRole("button", { name: "Promote to prod" }).click();
    await expect(page.getByRole("heading", { name: "Promote to production?" })).toBeVisible();
    await page.getByRole("button", { name: "Promote", exact: true }).click();
    await expect(page.getByText("Current production version")).toBeVisible();
    await page.goto("/projects/demo-app");
    await expect(page.getByText("v2").first()).toBeVisible();
    await expect(page.getByText("Production").first()).toBeVisible();
  });

  test("destroy confirm dialog", async ({ page }) => {
    await page.goto("/projects/demo-app/versions/v1");
    await page.getByRole("button", { name: "Destroy" }).click();
    await expect(page.getByRole("heading", { name: "Destroy version?" })).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByRole("heading", { name: "Destroy version?" })).not.toBeVisible();
  });
});

test.describe("database management (Neon-like)", () => {
  test("database page shows schema browser and tables", async ({ page }) => {
    await page.goto(storageBrowser("v1"));
    await expect(page.getByRole("heading", { name: "main" })).toBeVisible();
    await page.getByRole("tab", { name: "Data Editor" }).click();
    await expect(page.getByText("users")).toBeVisible();
    await expect(page.getByText("posts")).toBeVisible();
    await expect(page.getByText("comments")).toBeVisible();
  });

  test("table data viewer loads rows", async ({ page }) => {
    await page.goto(storageBrowser("v1"));
    await page.getByRole("tab", { name: "Data Editor" }).click();
    await page.getByRole("button", { name: "users" }).click();
    await expect(page.getByRole("columnheader", { name: "email" })).toBeVisible();
    await expect(page.getByText("alice@example.com")).toBeVisible();
  });

  test("sql editor runs query", async ({ page }) => {
    await page.goto(storageBrowser("v1"));
    await page.getByRole("tab", { name: "Query" }).click();
    await page.getByLabel("SQL query").fill("SELECT * FROM users LIMIT 5");
    await page.getByRole("button", { name: "Run" }).click();
    await expect(page.getByText(/row/)).toBeVisible();
    await expect(page.getByText(/\d+ ms/)).toBeVisible();
  });

  test("branch tree visible on branches tab", async ({ page }) => {
    await page.goto(storageBrowser("v2"));
    await page.getByRole("tab", { name: "Branches" }).click();
    await expect(page.getByText("Branch hierarchy")).toBeVisible();
    await expect(page.getByRole("link", { name: /v1.*main/i })).toBeVisible();
  });

  test("version switcher changes branch", async ({ page }) => {
    await page.goto(storageBrowser("v1"));
    await page.locator('[aria-haspopup="listbox"]').click();
    await page.getByRole("listbox").getByRole("option", { name: /^v5\b/ }).click();
    await expect(page).toHaveURL(/\/storage\/v5\/browser/);
  });

  test("project page links to database", async ({ page }) => {
    await page.goto("/projects/demo-app");
    await page.getByRole("link", { name: "Browse storage bindings" }).click();
    await expect(page).toHaveURL(/\/storage\/v\d+\/browser/);
  });

  test("create branch from database UI", async ({ page }) => {
    await page.goto(storageBrowser("v1"));
    await page.getByRole("tab", { name: "Branches" }).click();
    await page.getByRole("button", { name: "New branch" }).click();
    await expect(page.getByRole("heading", { name: "Create branch" })).toBeVisible();
    await page.getByPlaceholder("v-feature-x").fill("v-test-branch");
    await page.getByRole("button", { name: "Create branch", exact: true }).click();
    await expect(page).toHaveURL(/\/storage\/v-test-branch\/browser/);
    await expect(page.getByRole("heading", { name: "main" })).toBeVisible();
  });

  test("delete branch from branch list", async ({ page }) => {
    await page.goto(storageBrowser("v5"));
    await page.getByRole("tab", { name: "Branches" }).click();
    const row = page.locator("tbody tr").filter({
      has: page.getByRole("link", { name: "v5", exact: true }),
    });
    await row.getByRole("button", { name: "Delete" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("heading", { name: "Delete branch?" })).toBeVisible();
    await dialog.getByRole("button", { name: "Delete", exact: true }).click();
    await expect(row).not.toBeVisible();
  });

  test("legacy database URL redirects to storage browser", async ({ page }) => {
    await page.goto("/projects/demo-app/versions/v1/database");
    await expect(page).toHaveURL(/\/storage\/v1\/browser/);
  });
});

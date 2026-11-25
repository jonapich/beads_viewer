import { expect, row, test } from "./fixtures";

test.describe("issue list", () => {
  test.beforeEach(async ({ page, rw }) => {
    await rw.signIn(page);
    await page.goto(rw.base + "/issues");
  });

  test("tree view expands and collapses", async ({ page, tracker: { ids } }) => {
    await expect(row(page, ids.epic)).toBeVisible();
    await expect(page.getByRole("button", { name: "Tree" })).toHaveAttribute("aria-pressed", "true");

    await page.getByRole("button", { name: "Expand all" }).click();
    await expect(row(page, ids.cart)).toBeVisible();
    await expect(row(page, ids.payform)).toBeVisible();

    await page.getByRole("button", { name: "Collapse all" }).click();
    await expect(row(page, ids.cart)).toBeHidden();
    await expect(row(page, ids.epic)).toBeVisible();
  });

  test("flat view lists every active issue without expanding", async ({ page, tracker: { ids } }) => {
    const flat = page.getByRole("button", { name: "Flat" });
    await flat.click();
    await expect(flat).toHaveAttribute("aria-pressed", "true");
    await expect(row(page, ids.cart)).toBeVisible();
    await expect(row(page, ids.bug)).toBeVisible();
    // The expand controls belong to the tree.
    await expect(page.getByRole("button", { name: "Expand all" })).toHaveCount(0);
  });

  test("the state filter shows and hides closed issues", async ({ page, tracker: { ids } }) => {
    // Active (the default) leaves out closed issues that need no sign-off.
    await expect(row(page, ids.bug)).toBeVisible();
    await expect(row(page, ids.oldtask)).toBeHidden();

    const chip = page.locator('[data-chip="state"]');
    const state = page.getByRole("group", { name: "State filter" });
    await chip.click();
    await state.getByRole("radio", { name: "All" }).check();
    await expect(row(page, ids.oldtask)).toBeVisible();
    await expect(row(page, ids.bug)).toBeVisible();

    await state.getByRole("radio", { name: "Closed" }).check();
    await expect(row(page, ids.oldtask)).toBeVisible();
    await expect(row(page, ids.bug)).toBeHidden();

    await state.getByRole("radio", { name: "Active" }).check();
    await expect(row(page, ids.oldtask)).toBeHidden();
    await expect(row(page, ids.bug)).toBeVisible();
  });

  test("search narrows the list and Clear search brings it back", async ({ page, tracker: { ids } }) => {
    await expect(row(page, ids.epic)).toBeVisible();

    await page.getByRole("textbox", { name: "Search issues" }).fill("rounding");
    await expect(page.locator("[data-row]")).toHaveCount(1);
    await expect(row(page, ids.bug)).toBeVisible();

    await page.getByRole("button", { name: "Clear search" }).click();
    await expect(row(page, ids.epic)).toBeVisible();
    await expect(page.getByRole("textbox", { name: "Search issues" })).toHaveValue("");
  });

  test("the peek drawer opens from a row and closes with Escape", async ({ page, tracker: { ids } }) => {
    await row(page, ids.bug).locator(".t-btn").click();
    const drawer = page.getByRole("complementary", { name: "Issue details" });
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("Tax rounding off by a cent");
    // The id is the link to the full page (there is no separate "Open full view" link).
    await expect(drawer.getByRole("link", { name: ids.bug })).toHaveAttribute("href", `/issues/${ids.bug}`);
    await expect(page.getByRole("link", { name: "Open full view" })).toHaveCount(0);

    await page.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
  });

  test("the drawer's id opens the full issue page, and the copy button beside it does not", async ({ page, tracker: { ids } }) => {
    await row(page, ids.bug).locator(".t-btn").click();
    const drawer = page.getByRole("complementary", { name: "Issue details" });
    await expect(drawer).toBeVisible();

    await drawer.getByRole("button", { name: "Copy issue id" }).click();
    await expect(page).toHaveURL(/\/issues(\?|$)/); // still on the list

    await drawer.getByRole("link", { name: ids.bug }).click();
    await expect(page).toHaveURL(new RegExp(`/issues/${ids.bug}$`));
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Tax rounding off by a cent");
  });
});

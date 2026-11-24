import { expect, row, test } from "./fixtures";

// The read-only instance serves a copy of the exported JSONL with no bd database beside it, so
// bv has no write route for it.
test.describe("read-only tracker", () => {
  test.beforeEach(async ({ page, ro }) => {
    await ro.signIn(page);
    const meta = await (await page.request.get(ro.base + "/api/v1/meta")).json();
    // Fail loudly if the fixture is wrong: otherwise every assertion below would pass for the wrong reason.
    expect(meta.capabilities.writes).toBe(false);
  });

  test("the detail page explains it and offers no editing", async ({ page, ro, tracker: { ids } }) => {
    await page.goto(`${ro.base}/issues/${ids.payform}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Payment form");

    await expect(page.getByRole("note")).toBeVisible();
    await expect(page.getByRole("button", { name: "Edit title" })).toHaveCount(0);
    await expect(page.locator("button[data-status-anchor]")).toHaveCount(0);
    await expect(page.locator('[data-pop-anchor="prio"]')).toBeDisabled();
  });

  test("the list's priority pills are disabled", async ({ page, ro, tracker: { ids } }) => {
    await page.goto(ro.base + "/issues");
    await expect(row(page, ids.bug)).toBeVisible();
    await expect(row(page, ids.bug).locator('[data-pop-anchor="prio"]')).toBeDisabled();
  });
});

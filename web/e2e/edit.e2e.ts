import { expect, test, writeResponse } from "./fixtures";

test.beforeEach(async ({ page, rw }) => {
  await rw.signIn(page);
});

test("inline title edit saves through PATCH and bd has the new title", async ({ page, rw, tracker }) => {
  const { edit } = tracker.ids;
  await page.goto(`${rw.base}/issues/${edit}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Edit target");

  await page.getByRole("button", { name: "Edit title" }).click();
  const input = page.getByRole("textbox", { name: "Title" });
  await input.fill("Edit target renamed");

  const patch = writeResponse(page, "PATCH", edit);
  await input.press("Enter");
  expect((await patch).status()).toBe(200);

  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Edit target renamed");
  expect(tracker.show(edit).title).toBe("Edit target renamed");
});

test("an edit of a title that changed meanwhile is refused with a 409 and shows both versions", async ({ page, rw, tracker, gate }) => {
  const { conflict } = tracker.ids;
  // The browser logs every non-2xx fetch as a console error; this 409 is the point of the test.
  gate.allow(/status of 409/);
  await page.goto(`${rw.base}/issues/${conflict}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Conflict target");

  await page.getByRole("button", { name: "Edit title" }).click();
  const input = page.getByRole("textbox", { name: "Title" });
  await expect(input).toBeVisible();

  // Someone else changes the title with bd while the editor is open (bd has finished when this returns).
  tracker.bd("update", conflict, "--title", "Changed by bd meanwhile");

  await input.fill("Mine");
  const patch = writeResponse(page, "PATCH", conflict);
  await input.press("Enter");
  expect((await patch).status()).toBe(409);

  const alert = page.getByRole("alert").filter({ hasText: "changed since you opened it" });
  await expect(alert).toBeVisible();
  await expect(alert).toContainText("Changed by bd meanwhile");
  await expect(alert).toContainText("Mine");
  // Nothing was written over bd's change.
  expect(tracker.show(conflict).title).toBe("Changed by bd meanwhile");
});

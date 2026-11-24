import { expect, row, test } from "./fixtures";

test("a change made with bd outside the page shows up without a reload", async ({ page, rw, tracker }) => {
  const { live } = tracker.ids;
  await rw.signIn(page);

  // Live updates need the event stream; without it the page would only poll.
  const meta = await (await page.request.get(rw.base + "/api/v1/meta")).json();
  expect(meta.capabilities.events).toBe(true);

  await page.goto(rw.base + "/issues");
  await expect(row(page, live)).toContainText("Live target");
  // A marker that a page reload would wipe.
  await page.evaluate(() => {
    (window as unknown as { __e2e: number }).__e2e = 1;
  });

  tracker.bd("update", live, "--title", "Live target (renamed by bd)");
  tracker.export();

  await expect(row(page, live)).toContainText("renamed by bd", { timeout: 15_000 });
  expect(await page.evaluate(() => (window as unknown as { __e2e?: number }).__e2e)).toBe(1);
});

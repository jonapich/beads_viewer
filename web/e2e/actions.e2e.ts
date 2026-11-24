import { expect, row, test, writeResponse } from "./fixtures";

test.beforeEach(async ({ page, rw }) => {
  await rw.signIn(page);
});

test("priority quick action: the pill picks P0 and bd has it", async ({ page, rw, tracker }) => {
  const { bug } = tracker.ids;
  await page.goto(rw.base + "/issues");
  await expect(row(page, bug)).toBeVisible();
  expect(tracker.show(bug).priority).toBe(2);

  const put = writeResponse(page, "PUT", bug, "/priority");
  await row(page, bug).locator('[data-pop-anchor="prio"]').click();
  await page.getByRole("dialog", { name: `Actions for ${bug}` }).getByRole("button", { name: "Priority P0" }).click();
  expect((await put).status()).toBe(200);

  await expect(row(page, bug).locator(".pp")).toHaveText("P0");
  // bd finished before the server answered, so no polling is needed.
  expect(tracker.show(bug).priority).toBe(0);
});

test("sign-off approve: the card goes away and the needs-signoff label is removed", async ({ page, rw, tracker }) => {
  const { signoff } = tracker.ids;
  await page.goto(`${rw.base}/issues/${signoff}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Ship the cart");
  expect(tracker.show(signoff).labels).toContain("needs-signoff");

  const post = writeResponse(page, "POST", signoff, "/signoff");
  await page.getByRole("button", { name: "Approve" }).click();
  expect((await post).status()).toBe(200);

  await expect(page.getByRole("button", { name: "Approve" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Dispute/ })).toHaveCount(0);
  expect(tracker.show(signoff).labels ?? []).not.toContain("needs-signoff");
});

test("decision answer: the picked option and note are posted and the label is removed", async ({ page, rw, tracker }) => {
  const { decision } = tracker.ids;
  await page.goto(`${rw.base}/issues/${decision}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Pick a payment provider");
  expect(tracker.show(decision).labels).toContain("needs-decision");

  await page.getByRole("radiogroup", { name: "Options" }).getByRole("radio", { name: /Stripe/ }).click();
  await page.getByLabel("Add a note (optional)").fill("e2e answer");

  const post = writeResponse(page, "POST", decision, "/answer");
  await page.getByRole("button", { name: "Post answer" }).click();
  expect((await post).status()).toBe(200);

  await expect(page.getByRole("button", { name: "Post answer" })).toHaveCount(0);
  expect(tracker.show(decision).labels ?? []).not.toContain("needs-decision");
  const comments = tracker.json<{ text: string }[]>("comments", decision, "--json");
  expect(comments.map((c) => c.text).join("\n")).toContain("e2e answer");
});

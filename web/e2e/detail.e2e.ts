import { expect, test } from "./fixtures";

test("the detail page loads straight from its URL", async ({ page, rw, tracker: { ids } }) => {
  await rw.signIn(page);
  // A full page load of a deep link: the server answers it with the app (the SPA fallback).
  await page.goto(`${rw.base}/issues/${ids.payform}`);

  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Payment form");
  await expect(page.getByRole("navigation", { name: "Parent" })).toContainText("Checkout redesign");
  await expect(page.getByRole("complementary", { name: "Details" })).toContainText("Alice");
  // A kinded comment is open by default.
  await expect(page.getByText("Brief: the payment form must support saved cards.")).toBeVisible();
});

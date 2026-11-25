import { expect, test } from "./fixtures";

test("the API refuses a visitor without the session, and signing in opens the app", async ({ page, rw }) => {
  // A fresh browser context has no cookie.
  const meta = await page.request.get(rw.base + "/api/v1/meta");
  expect(meta.status()).toBe(401);
  expect((await meta.json()).error.code).toBe("unauthorized");

  await rw.signIn(page);
  await expect(page.getByRole("heading", { level: 1, name: "Agent desk" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Overview" })).toHaveCount(0);

  // The session works for the API too.
  const signedIn = await page.request.get(rw.base + "/api/v1/meta");
  expect(signedIn.status()).toBe(200);
});

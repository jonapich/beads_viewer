import { readFileSync } from "node:fs";
import type { Page } from "@playwright/test";
import { expect, test, writeResponse } from "./fixtures";

// The agent desk (beads-c2o.11). The real tracker covers the flows; page.route mocks cover what a
// small fixture cannot reach (a stale claim, an empty metrics report). The desk beads in the
// fixture manifest (deskA, deskB, deskSignoff, claimed, sliced) belong to this file; the actions
// e2e owns the older decision and signoff beads, so the two never race.

const fixture = (name: string): string => readFileSync(new URL(`../src/lib/api/__fixtures__/${name}`, import.meta.url), "utf8");
const pipelineFixture = fixture("pipeline.json");
const sliceMetricsEmpty = fixture("slice-metrics-empty.json");

const json = (body: string, etag: string) => ({ status: 200, contentType: "application/json", headers: { ETag: etag }, body });

/** meta.counts.awaiting + signoff_pending, as the nav badge counts them. */
async function badgeTotal(page: Page, base: string): Promise<number> {
  const meta = (await (await page.request.get(base + "/api/v1/meta")).json()) as { counts: { awaiting: number; signoff_pending: number } };
  return meta.counts.awaiting + meta.counts.signoff_pending;
}

const deskLink = (page: Page) => page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: /Agent desk/ });

test.describe("agent desk", () => {
  test.beforeEach(async ({ page, rw }) => {
    await rw.signIn(page);
  });

  test("the nav badge counts questions plus sign-offs, and the desk opens on the Inbox", async ({ page, rw, tracker: { ids } }) => {
    const total = await badgeTotal(page, rw.base);
    expect(total).toBeGreaterThanOrEqual(3); // deskA, deskB and deskSignoff at least
    await expect(deskLink(page).locator(".badge")).toHaveText(String(total));

    await deskLink(page).click();
    await expect(page).toHaveURL(/\/desk\?tab=inbox&id=/);
    await expect(page.getByRole("heading", { level: 1, name: "Agent desk" })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Inbox/ })).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator(`[data-row="${ids.deskA}"]`)).toBeVisible();
    await expect(page.locator(`[data-row="${ids.deskB}"]`)).toBeVisible();
    // The selected row's pane has the answer form.
    await expect(page.locator('[data-desk-list] [aria-current="true"]')).toHaveCount(1);
    await expect(page.getByRole("button", { name: "Post answer" })).toBeVisible();
  });

  test("Inbox: answering marks the row and advances at once, and the row leaves after the linger", async ({ page, rw, tracker }) => {
    const { deskA } = tracker.ids;
    await page.goto(`${rw.base}/desk?tab=inbox&id=${deskA}`);
    const pane = page.locator(".pane");
    await expect(pane).toHaveAttribute("data-pane", deskA);
    await expect(pane.getByRole("heading", { level: 2 })).toHaveText("Desk question one");
    expect(tracker.show(deskA).labels).toContain("needs-decision");

    await pane.getByLabel("Add a note (optional)").fill("e2e desk answer");
    const post = writeResponse(page, "POST", deskA, "/answer");
    await page.getByRole("button", { name: "Post answer" }).click();

    // Optimistic: the row says Answered and the selection has moved on while bd is still working, i.e.
    // before the POST has an answer. (bd takes a second or more per step; these assertions need not wait for it.)
    const answered = page.locator(`[data-row="${deskA}"]`);
    await expect(answered).toContainText("Answered");
    await expect(answered).toContainText("(a) Go ahead");
    await expect(answered).not.toHaveAttribute("aria-current", "true");
    const selected = page.locator('[data-desk-list] [aria-current="true"]');
    await expect(selected).toHaveCount(1);
    await expect(selected).not.toHaveAttribute("data-row", deskA);
    await expect(selected).not.toContainText("Answered");
    const id = await selected.getAttribute("data-row");
    expect(id).toBeTruthy();
    expect(new URL(page.url()).searchParams.get("id")).toBe(id);
    await expect(pane).toHaveAttribute("data-pane", id ?? "");

    expect((await post).status()).toBe(200);
    // bd has the answer; the handled row lingers about two seconds, muted, and then leaves the list.
    expect(tracker.show(deskA).labels ?? []).not.toContain("needs-decision");
    const comments = tracker.json<{ text: string }[]>("comments", deskA, "--json");
    expect(comments.map((c) => c.text).join("\n")).toContain("e2e desk answer");
    await expect(answered).toHaveCount(0);
    await expect(selected).toHaveCount(1);
  });

  test("Sign-off: approving marks the row at once, removes the label and drops the nav badge", async ({ page, rw, tracker }) => {
    const { deskSignoff } = tracker.ids;
    const before = await badgeTotal(page, rw.base);
    await page.goto(`${rw.base}/desk?tab=signoff&id=${deskSignoff}`);
    await expect(page.locator(".pane")).toHaveAttribute("data-pane", deskSignoff);
    await expect(page.getByText("Keep carts 30 days")).toBeVisible(); // the call made on the user's behalf
    expect(tracker.show(deskSignoff).labels).toContain("needs-signoff");

    const post = writeResponse(page, "POST", deskSignoff, "/signoff");
    await page.getByRole("button", { name: "Approve", exact: true }).click();

    // Before the POST has an answer: the row and the pane already show the result.
    await expect(page.locator(`[data-row="${deskSignoff}"]`)).toContainText("Approved");
    await expect(page.getByText("Approved just now")).toBeVisible();
    await expect(page.getByRole("button", { name: "Approve", exact: true })).toHaveCount(0);

    expect((await post).status()).toBe(200);
    expect(tracker.show(deskSignoff).labels ?? []).not.toContain("needs-signoff");
    await expect(page.locator(`[data-row="${deskSignoff}"]`)).toHaveCount(0); // lingered, then left

    if (before - 1 === 0) await expect(deskLink(page).locator(".badge")).toHaveCount(0);
    else await expect(deskLink(page).locator(".badge")).toHaveText(String(before - 1));
  });
  test("Pipeline: the claimed bead is listed with its stage, a row opens the drawer, and it refetches every minute", async ({ page, rw, tracker }) => {
    const { claimed } = tracker.ids;
    const requests: string[] = [];
    page.on("request", (r) => {
      if (new URL(r.url()).pathname === "/api/v1/pipeline") requests.push(r.url());
    });
    await page.clock.install();
    await page.goto(`${rw.base}/desk?tab=pipeline`);
    const tr = page.locator(`tr[data-id="${claimed}"]`);
    await expect(tr).toBeVisible();
    await expect(tr.locator(".tag")).toHaveText("code");
    await expect(tr).toContainText("Claimed target");
    await expect(page.getByText(/\d+ in progress/).first()).toBeVisible();
    await expect(page.getByRole("button", { name: /^Pipeline/ })).toHaveAttribute("aria-pressed", "true");

    // Staying on the tab, the pipeline is fetched again a minute later (staleness depends on the clock).
    const seen = requests.length;
    await page.clock.fastForward("01:05");
    await expect.poll(() => requests.length).toBeGreaterThan(seen);

    await tr.getByRole("button", { name: "Claimed target" }).click();
    const drawer = page.getByRole("complementary", { name: "Issue details" });
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("Claimed target");
    await page.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
  });

  test("Pipeline: a stale claim is flagged with its reason", async ({ page, rw }) => {
    await page.route("**/api/v1/pipeline", (r) => r.fulfill(json(pipelineFixture, '"mock-pipeline"')));
    await page.goto(`${rw.base}/desk?tab=pipeline`);
    const stale = page.locator('tr[data-id="fx-c"]');
    await expect(stale).toHaveClass(/stale/);
    await expect(stale).toContainText("⚠ no activity for 8d2h");
    await expect(page.locator('tr[data-id="fx-a"]')).not.toHaveClass(/stale/);
    await expect(page.locator(".psum")).toContainText("3 in progress");
    await expect(page.locator(".psum")).toContainText("1 stale");
  });

  test("Metrics: the logged slice is summed per epic with a total row", async ({ page, rw }) => {
    await page.goto(`${rw.base}/desk?tab=metrics`);
    await expect(page.getByRole("button", { name: /^Metrics/ })).toHaveAttribute("aria-pressed", "true");
    // The fixture's one slice has no epic: Slices, ✓/✗, Rev/slice, 1st-pass, Tests, Red, Min/slice, Tokens k, Esc.
    const cells = page.locator('tr[data-metrics-row=""] td');
    await expect(cells).toHaveText(["(no epic)", "", "1", "1/0", "1.0", "100%", "3", "1", "20", "12/3/0", "0"]);
    await expect(page.locator("tr[data-metrics-total]")).toContainText("All · 1 epic");
  });

  test("Metrics: no slice metadata shows the empty state", async ({ page, rw }) => {
    await page.route("**/api/v1/slice-metrics", (r) => r.fulfill(json(sliceMetricsEmpty, '"mock-metrics"')));
    await page.goto(`${rw.base}/desk?tab=metrics`);
    await expect(page.getByText("No slice metrics")).toBeVisible();
    await expect(page.getByText("No bead carries slice_outcome metadata.")).toBeVisible();
  });
});

test.describe("agent desk, read-only tracker", () => {
  test("the form is disabled and the page says why", async ({ page, ro, tracker: { ids } }) => {
    await ro.signIn(page);
    await page.goto(`${ro.base}/desk?tab=inbox&id=${ids.deskB}`);
    await expect(page.locator(".pane")).toHaveAttribute("data-pane", ids.deskB);
    await expect(page.getByRole("button", { name: "Post answer" })).toBeDisabled();
    await expect(page.locator("div.notice")).toBeVisible();
  });
});

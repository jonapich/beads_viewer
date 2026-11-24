import { readFileSync } from "node:fs";
import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";

// The Triage, Plan and Insights views (beads-c2o.10). The real tracker covers the normal and
// read-only cases; page.route mocks cover the states a small fixture never reaches (503, 500,
// phase 2 skipped, offline). A mocked failing response logs "Failed to load resource" in the
// console, which the gate must allow for that test only.

const READ_ONLY_TITLE = "Read-only: this tracker can't be changed from bv";
const FAILED_RESOURCE = /Failed to load resource/;

const insightsFixture = readFileSync(new URL("../src/lib/api/__fixtures__/insights.json", import.meta.url), "utf8");

interface PlanBody {
  tracks: Array<{ items: unknown[] }>;
  summary: { highest_impact: string; unblocks_count: number };
}

const json = (status: number, body: unknown, headers: Record<string, string> = {}) => ({
  status,
  contentType: "application/json",
  headers,
  body: typeof body === "string" ? body : JSON.stringify(body),
});

async function open(page: Page, base: string, path: string): Promise<void> {
  await page.goto(base + path);
}

test.describe("Triage, Plan and Insights", () => {
  test.beforeEach(async ({ page, rw }) => {
    await rw.signIn(page);
  });

  test("the nav lists the views in order", async ({ page }) => {
    const links = page.getByRole("navigation", { name: "Main" }).getByRole("link");
    // The Agent desk link carries a count badge while something awaits the user.
    await expect(links).toHaveText(["Overview", "Issues", /^Agent desk/, "Triage", "Plan", "Insights"]);
  });

  test("Triage shows Start here cards and recommendations, and a row opens the drawer", async ({ page, rw }) => {
    await open(page, rw.base, "/triage");
    await expect(page.getByRole("heading", { level: 1, name: "Triage" })).toBeVisible();
    await expect(page.locator("article.pick").first()).toBeVisible();
    const rows = page.getByRole("list", { name: "Recommendations" }).locator("[data-row]");
    await expect(rows.first()).toBeVisible();

    await rows.first().locator(".t-btn").click();
    const drawer = page.getByRole("complementary", { name: "Issue details" });
    await expect(drawer).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
  });

  test("Plan: the banner and the track cards match GET /api/v1/plan", async ({ page, rw }) => {
    const plan = (await (await page.request.get(rw.base + "/api/v1/plan")).json()) as PlanBody;
    await open(page, rw.base, "/plan");
    await expect(page.getByRole("heading", { level: 1, name: "Plan" })).toBeVisible();

    // Multi-item tracks first, then at most six single-item tracks; the rest fold. Waiting for the
    // cards means the plan has rendered before anything is asserted to be absent.
    expect(plan.tracks.length).toBeGreaterThan(0);
    const singles = plan.tracks.filter((t) => t.items.length === 1).length;
    const multi = plan.tracks.length - singles;
    await expect(page.locator("section.tcard")).toHaveCount(multi + Math.min(singles, 6));
    if (singles > 6) {
      await page.getByRole("button", { name: /more single-item track/ }).click();
      await expect(page.locator("section.tcard")).toHaveCount(plan.tracks.length);
    }

    // The fixture's blocking edges decide whether a banner shows: read what the server says.
    const banner = page.getByRole("status").filter({ hasText: "Start with" });
    if (plan.summary.unblocks_count > 0) {
      await expect(banner).toContainText(plan.summary.highest_impact);
      await expect(banner).toContainText(`unblocks ${plan.summary.unblocks_count}`);
    } else {
      await expect(banner).toHaveCount(0);
    }
  });

  test("Insights renders the nine metric cards and the health strip", async ({ page, rw }) => {
    await open(page, rw.base, "/insights");
    await expect(page.getByRole("heading", { level: 1, name: "Insights" })).toBeVisible();
    await expect(page.locator("[data-metric]")).toHaveCount(9);
    await expect(page.getByRole("list", { name: "Project health" })).toBeVisible();

    // The "i" toggle shows the explanation.
    const card = page.locator('[data-metric="bottlenecks"]');
    await card.getByRole("button", { name: "About Bottlenecks" }).click();
    await expect(card).toContainText("Formula:");
  });

  test("Triage: 503 analysis_pending shows Computing triage, then the data once it is ready", async ({ page, rw, gate }) => {
    gate.allow(FAILED_RESOURCE);
    await page.route("**/api/v1/triage", (r) =>
      r.fulfill(json(503, { error: { code: "analysis_pending", message: "triage is still being computed" } }, { "Retry-After": "2" })),
    );
    await open(page, rw.base, "/triage");
    await expect(page.getByText("Computing triage…")).toBeVisible();
    await expect(page.locator("article.pick")).toHaveCount(0);

    await page.unroute("**/api/v1/triage");
    await expect(page.locator("article.pick").first()).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText("Computing triage…")).toHaveCount(0);
  });

  test("Triage: 500 analysis_failed shows the failure with Retry", async ({ page, rw, gate }) => {
    gate.allow(FAILED_RESOURCE);
    await page.route("**/api/v1/triage", (r) =>
      r.fulfill(json(500, { error: { code: "analysis_failed", message: "triage analysis failed: boom" } })),
    );
    await open(page, rw.base, "/triage");
    const alert = page.getByRole("alert").filter({ hasText: "Can't load triage" });
    await expect(alert).toContainText("triage analysis failed: boom");

    await page.unroute("**/api/v1/triage");
    await alert.getByRole("button", { name: "Retry" }).click();
    await expect(page.locator("article.pick").first()).toBeVisible();
  });

  test("Insights: skipped metrics name their reason and cycles are chains", async ({ page, rw }) => {
    await page.route(/\/api\/v1\/insights/, (r) => r.fulfill(json(200, insightsFixture, { ETag: '"mock-insights"' })));
    await open(page, rw.base, "/insights");
    await expect(page.locator('[data-metric="keystones"]')).toContainText("Skipped:");
    await expect(page.locator('[data-metric="slack"]')).toContainText("Skipped:");
    const cycles = page.locator('[data-metric="cycles"]');
    // The server reports a cycle as a closed path (fx-cy1, fx-cy2, fx-cy1), so the first id shows twice.
    await expect(cycles.getByRole("button", { name: "fx-cy1" }).first()).toBeVisible();
    await expect(cycles.getByRole("button", { name: "fx-cy2" })).toHaveCount(1);
    await expect(cycles).toContainText("1 cycle");
  });

  test("Insights: phase 2 skipped is one notice and no cards", async ({ page, rw }) => {
    const body = { ...(JSON.parse(insightsFixture) as object), phase2_skipped: true, phase2_ready: false, skipped: {} };
    await page.route(/\/api\/v1\/insights/, (r) => r.fulfill(json(200, body, { ETag: '"mock-skipped"' })));
    await open(page, rw.base, "/insights");
    await expect(page.getByText(/Graph metrics are off for trackers this large/)).toBeVisible();
    await expect(page.locator("[data-metric]")).toHaveCount(0);
  });

  test("offline: the banner shows and the data stays, dimmed", async ({ page, rw, gate }) => {
    gate.allow(/Failed to load resource|net::ERR/);
    await open(page, rw.base, "/triage");
    await expect(page.locator("article.pick").first()).toBeVisible();
    // Another view first, so Triage is cached when it is opened again with the API gone.
    const nav = page.getByRole("navigation", { name: "Main" });
    await nav.getByRole("link", { name: "Plan" }).click();
    await expect(page.locator("section.tcard").first()).toBeVisible();

    await page.route("**/api/**", (r) => r.abort());
    await nav.getByRole("link", { name: "Triage" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Can't reach bv serve" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Triage" })).toHaveClass(/offline/);
    // The cached data stays on show.
    await expect(page.locator("article.pick").first()).toBeVisible();
  });
});
test.describe("read-only tracker", () => {
  test.beforeEach(async ({ page, ro }) => {
    await ro.signIn(page);
    const meta = await (await page.request.get(ro.base + "/api/v1/meta")).json();
    expect(meta.capabilities.writes).toBe(false);
  });

  test("Triage priority pills are disabled with the read-only title", async ({ page, ro }) => {
    // writes is false until /meta arrives: wait for it, so it is the capability that disables the pill.
    const meta = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/v1/meta");
    await open(page, ro.base, "/triage");
    await meta;
    const pill = page.locator('article.pick [data-pop-anchor="prio"]').first();
    await expect(pill).toBeDisabled();
    await expect(pill).toHaveAttribute("title", READ_ONLY_TITLE);
    await expect(page.getByRole("list", { name: "Recommendations" }).locator('[data-pop-anchor="prio"]').first()).toBeDisabled();
  });

  test("Plan priority pills are disabled with the read-only title", async ({ page, ro }) => {
    const meta = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/v1/meta");
    await open(page, ro.base, "/plan");
    await meta;
    const pill = page.locator('[data-row] [data-pop-anchor="prio"]').first();
    await expect(pill).toBeDisabled();
    await expect(pill).toHaveAttribute("title", READ_ONLY_TITLE);
  });
});

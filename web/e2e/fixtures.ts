import { expect, test as base, type Locator, type Page } from "@playwright/test";
import { readState, type State } from "./lib/state";
import { bd as runBd, bdJson, bdShow, exportJsonl, type BdIssue, type Manifest } from "./lib/tracker";

/** A running bv serve, as a test uses it. */
export interface Site {
  base: string;
  /** Signs in the way a user does: opens the printed URL and lands on the app. */
  signIn(page: Page): Promise<void>;
}

/** The console gate: any console error or uncaught page error fails the test, unless allowed. */
export interface ConsoleGate {
  allow(pattern: RegExp): void;
}

/** The writable tracker, driven with the real bd the way an outside change would be. */
export interface Tracker {
  ids: Manifest;
  /** The bd executable's output for `bd <args>` in the writable tracker. */
  bd(...args: string[]): string;
  json<T>(...args: string[]): T;
  show(id: string): BdIssue;
  /** Writes the JSONL bv watches (bd's auto-export is off), so bv sees a change made with bd. */
  export(): void;
}

interface TestFixtures {
  skipGuard: undefined;
  gate: ConsoleGate;
  rw: Site;
  ro: Site;
  tracker: Tracker;
}

interface WorkerFixtures {
  state: State;
}

function site(inst: State["rw"]): Site {
  return {
    base: inst.base,
    async signIn(page) {
      await page.goto(inst.signIn);
      await expect(page).toHaveURL(inst.base + "/");
      await expect(page.getByRole("region", { name: "Server health" })).toBeVisible();
    },
  };
}

export const test = base.extend<TestFixtures, WorkerFixtures>({
  state: [
    async ({}, use) => {
      await use(readState());
    },
    { scope: "worker" },
  ],

  // Skips the test (before it needs a browser) when global setup found a tool missing.
  skipGuard: [
    async ({}, use, testInfo) => {
      const reason = process.env["BV_E2E_SKIP"];
      testInfo.skip(reason !== undefined, reason ?? "");
      await use(undefined);
    },
    { auto: true },
  ],

  // Overridden so the skip guard runs before the browser is launched.
  page: async ({ page, skipGuard }, use) => {
    void skipGuard;
    await use(page);
  },

  gate: [
    async ({ page }, use) => {
      const seen: string[] = [];
      const allowed: RegExp[] = [];
      page.on("console", (m) => {
        if (m.type() === "error") seen.push(`console.error: ${m.text()}`);
      });
      page.on("pageerror", (e) => seen.push(`pageerror: ${e.message}`));
      await use({ allow: (re) => allowed.push(re) });
      expect(seen.filter((s) => !allowed.some((re) => re.test(s))), "console errors").toEqual([]);
    },
    { auto: true },
  ],

  rw: async ({ state }, use) => {
    await use(site(state.rw));
  },
  ro: async ({ state }, use) => {
    await use(site(state.ro));
  },
  tracker: async ({ state }, use) => {
    const { bd, rw, ids } = state;
    await use({
      ids,
      bd: (...args) => runBd(bd, rw.dir, ...args),
      json: <T>(...args: string[]) => bdJson<T>(bd, rw.dir, ...args),
      show: (id) => bdShow(bd, rw.dir, id),
      export: () => void exportJsonl(bd, rw.dir),
    });
  },
});

export { expect };

/** The list row of an issue (tree or flat). */
export function row(page: Page, id: string): Locator {
  return page.locator(`[data-row][data-id="${id}"]`);
}

/**
 * Starts waiting for the write request of an action, so the response is not missed. Call it
 * BEFORE the click, await the result after.
 */
export function writeResponse(page: Page, method: string, id: string, suffix = "") {
  return page.waitForResponse(
    (r) => r.request().method() === method && new URL(r.url()).pathname === `/api/v1/issues/${id}${suffix}`,
  );
}

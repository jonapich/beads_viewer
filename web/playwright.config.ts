import { defineConfig } from "@playwright/test";

// The GUI e2e suite (npm run e2e): drives the installed Edge against real `bv serve` instances
// started by e2e/global-setup.ts. One worker, no retries: a retry would hide a flaky test.
export default defineConfig({
  testDir: "./e2e",
  testMatch: /.*\.e2e\.ts$/,
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: "list",
  outputDir: "./test-results",
  use: {
    channel: process.env["BV_E2E_CHANNEL"] ?? "msedge",
    headless: true,
    viewport: { width: 1280, height: 900 },
    colorScheme: "light",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});

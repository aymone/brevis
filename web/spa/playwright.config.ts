import { defineConfig, devices } from "@playwright/test";

// The suite runs against a RUNNING engine (it tests the embedded dist, the real API and the
// real event stream, which no mock can stand in for): `BASE_URL` says where, and the default
// is what `make up` serves. See README.md for the three commands that bring one up.
const CI = !!process.env.CI;

export default defineConfig({
  testDir: "e2e",
  // One worker, in order: several specs count the requests a screen makes and watch a run
  // appear, so a second worker triggering runs would show up in the first one's numbers.
  workers: 1,
  fullyParallel: false,
  // A retry is for timing (a run that was a beat late), never for logic: the thresholds in the
  // specs are fixed, and a retried pass is reported as flaky, not green.
  retries: CI ? 1 : 0,
  timeout: 45_000,
  expect: { timeout: 8_000 },
  forbidOnly: CI,
  reporter: CI ? [["list"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:8080",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } }],
});

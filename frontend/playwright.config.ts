import { defineConfig, devices } from "@playwright/test";
const live = process.env.MARKET_MOOD_TEST_LIVE === "1";
export default defineConfig({
  testDir: "./tests",
  grepInvert: live ? undefined : /@live/,
  use: {
    baseURL: live
      ? (process.env.MARKET_MOOD_TEST_URL ?? "http://127.0.0.1:8080")
      : "http://127.0.0.1:4173",
    ...devices["iPhone 13"],
    defaultBrowserType: "chromium",
    trace: "retain-on-failure",
    launchOptions: process.env.MARKET_MOOD_BROWSER_PATH
      ? { executablePath: process.env.MARKET_MOOD_BROWSER_PATH }
      : undefined,
    channel: process.env.MARKET_MOOD_BROWSER_CHANNEL,
  },
  webServer: live
    ? undefined
    : {
        command: "node scripts/ui-fixture-server.mjs",
        cwd: "..",
        url: "http://127.0.0.1:4173/api/dashboard",
        reuseExistingServer: false,
      },
  reporter: "list",
});

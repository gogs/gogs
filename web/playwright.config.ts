import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.GOGS_E2E_PORT ?? 3900);

// The browser tests run the admin console in the engines of the browsers
// people use, on a phone with touch input, and in another language, against
// a throwaway Gogs that `e2e/server.sh` starts.
export default defineConfig({
  testDir: "e2e",
  outputDir: "e2e/results",
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  use: {
    baseURL: `http://localhost:${port}`,
    trace: "retain-on-failure",
  },
  webServer: {
    command: "bash e2e/server.sh",
    // Answers 404 until the server has its admin.
    url: `http://127.0.0.1:${port}/api/v1/users/e2e-admin`,
    reuseExistingServer: !process.env.CI,
    env: { GOGS_E2E_PORT: String(port) },
    // Lets the script stop the server and remove its directory.
    gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
  },
  projects: [
    { name: "chromium", testIgnore: "locale.spec.ts", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", testIgnore: "locale.spec.ts", use: { ...devices["Desktop Firefox"] } },
    { name: "webkit", testIgnore: "locale.spec.ts", use: { ...devices["Desktop Safari"] } },
    { name: "phone", testIgnore: "locale.spec.ts", use: { ...devices["iPhone 14"] } },
    // German has long words, which is what breaks a layout made for English.
    { name: "german", testMatch: "locale.spec.ts", use: { ...devices["iPhone 14"], locale: "de-DE" } },
  ],
});

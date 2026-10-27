import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "@playwright/test";

import { mainNode, nodeEnv, nodeURL, serverBinary } from "./e2e/nodes";
import { ownerStorageState } from "./e2e/owner-account";

const webRoot = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(webRoot, "..");

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  outputDir: path.join(webRoot, "test-results"),
  // Include retries in JUnit reports so Codecov can identify flaky tests instead of seeing only the final pass.
  reporter: process.env.CI
    ? [
        ["html", { open: "never" }],
        ["list"],
        [
          "junit",
          {
            outputFile: path.join(repoRoot, "reports", "e2e-junit.xml"),
            includeRetries: true,
          },
        ],
      ]
    : "list",
  use: {
    baseURL: "http://127.0.0.1:15173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  // Onboarding needs an empty library and runs after account setup, before the other tests.
  projects: [
    {
      name: "owner-setup",
      testMatch: "auth.setup.ts",
    },
    {
      name: "onboarding",
      testMatch: "onboarding.e2e.ts",
      dependencies: ["owner-setup"],
      use: { storageState: ownerStorageState },
    },
    {
      name: "e2e",
      testMatch: "**/*.e2e.ts",
      testIgnore: "onboarding.e2e.ts",
      dependencies: ["owner-setup"],
      use: { storageState: ownerStorageState },
    },
  ],
  // The second node (e2e/nodes.ts) is not a web server here: the pairing spec stops it mid-test, which
  // Playwright cannot do to a server it started.
  webServer: [
    {
      command: JSON.stringify(serverBinary),
      cwd: repoRoot,
      env: nodeEnv(mainNode),
      url: `${nodeURL(mainNode)}/api/health`,
      reuseExistingServer: false,
      timeout: 180_000,
    },
    {
      command: "npm run dev -- --host 127.0.0.1 --port 15173 --strictPort",
      cwd: webRoot,
      env: {
        MDM_API_TARGET: nodeURL(mainNode),
      },
      url: "http://127.0.0.1:15173/settings",
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});

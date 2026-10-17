import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "@playwright/test";

import { ownerStorageState } from "./e2e/owner-account";

const webRoot = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(webRoot, "..");
const runRoot = process.env.MDM_E2E_RUN_ROOT;
if (runRoot === undefined) throw new Error("MDM_E2E_RUN_ROOT is not configured");
const dataDir = path.join(runRoot, "data");
const serverBinary = path.join(
  runRoot,
  "bin",
  process.platform === "win32" ? "mdm.exe" : "mdm",
);

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  outputDir: path.join(webRoot, "test-results"),
  // CI では JUnit の結果も reports/ に書き、ci.yml が Codecov へ送る。
  // includeRetries が無いと、リトライで通ったテストは成功だけが残り、不安定だと分からない。
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
  // プロジェクトは依存の順に走る。未設定のサーバーでの確かめ → アカウントを作って
  // ログインした状態を保存 → その状態で既存の e2e すべて。
  // onboarding は動画のないライブラリから始めるので（親 #1140 受け入れ条件 3）、ほかの e2e が
  // フォルダを足す前に走らせる。e2e より前に並べ、ファイルで絞ったときはその組にだけ入る。
  projects: [
    {
      name: "unconfigured",
      testMatch: "unconfigured.setup.ts",
    },
    {
      name: "owner-setup",
      testMatch: "auth.setup.ts",
      dependencies: ["unconfigured"],
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
  webServer: [
    {
      command: JSON.stringify(serverBinary),
      cwd: repoRoot,
      env: {
        MDM_ADDR: "127.0.0.1:18080",
        MDM_DATA_DIR: dataDir,
        MDM_LOG_LEVEL: "debug",
      },
      url: "http://127.0.0.1:18080/api/health",
      reuseExistingServer: false,
      timeout: 180_000,
    },
    {
      command: "npm run dev -- --host 127.0.0.1 --port 15173 --strictPort",
      cwd: webRoot,
      env: {
        MDM_API_TARGET: "http://127.0.0.1:18080",
      },
      url: "http://127.0.0.1:15173/settings",
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});

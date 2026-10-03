import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "@playwright/test";

// タグ管理画面の規模の計測（specs/036-tag-admin-scale/research.md R-9）。
// `go run ./scripts/tagsbench` が規模のデータで本番ビルドを起動し、その URL を
// TAGSBENCH_BASE_URL で渡して呼ぶ。web/e2e/ の試験とは別の設定で、task test-e2e と
// CI には入れない。手順は docs/how-to/tags-admin-benchmark.md。

const benchRoot = path.dirname(fileURLToPath(import.meta.url));
const baseURL = process.env.TAGSBENCH_BASE_URL;
if (baseURL === undefined) {
  throw new Error(
    "TAGSBENCH_BASE_URL is not configured (run go run ./scripts/tagsbench)",
  );
}
const chromium = process.env.TAGSBENCH_CHROMIUM;

export default defineConfig({
  testDir: benchRoot,
  testMatch: "*.bench.ts",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  // 3,000 個の規模で全場面を回すと数分かかる。
  timeout: 20 * 60_000,
  outputDir: path.join(benchRoot, "..", "test-results", "bench"),
  reporter: "list",
  use: {
    baseURL,
    browserName: "chromium",
    headless: true,
    viewport: { width: 1280, height: 800 },
    launchOptions:
      chromium === undefined || chromium === "" ? {} : { executablePath: chromium },
  },
});

import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "@playwright/test";

// Benchmark the production build started by scripts/tagsbench via TAGSBENCH_BASE_URL, outside e2e and CI; see docs/how-to/tags-admin-benchmark.md.

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
  // Scrolling all 30,000 tags can take tens of minutes.
  timeout: 90 * 60_000,
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

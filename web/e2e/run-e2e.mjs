import { mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import { generateMediaFixtures } from "./media-fixtures.mjs";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = path.resolve(webRoot, "..");
const runRoot = path.join(repoRoot, ".local", "e2e", `${Date.now()}-${process.pid}`);
const outputDir = path.join(runRoot, "bin");
const output = path.join(outputDir, process.platform === "win32" ? "mdm.exe" : "mdm");
const mediaDir = path.join(runRoot, "media");
const settingsMediaDir = path.join(runRoot, "settings-media");

function run() {
  mkdirSync(outputDir, { recursive: true });
  try {
    generateMediaFixtures(mediaDir);
    const mediaContract = spawnSync(
      "go",
      ["test", "./internal/media", "-run", "^TestVideoEncode.*WithFFmpeg$", "-count=1"],
      {
        cwd: repoRoot,
        env: { ...process.env, GOCACHE: path.join(runRoot, "go-build") },
        stdio: "inherit",
      },
    );
    if (mediaContract.error !== undefined) throw mediaContract.error;
    if (mediaContract.status !== 0) return mediaContract.status ?? 1;
    const build = spawnSync(
      "go",
      ["build", "-buildvcs=false", "-o", output, "./cmd/mdm"],
      {
        cwd: repoRoot,
        env: { ...process.env, GOCACHE: path.join(runRoot, "go-build") },
        stdio: "inherit",
      },
    );
    if (build.error !== undefined) throw build.error;
    if (build.status !== 0) return build.status ?? 1;

    const playwrightCLI = path.join(
      webRoot,
      "node_modules",
      "@playwright",
      "test",
      "cli.js",
    );
    const test = spawnSync(process.execPath, [playwrightCLI, "test"], {
      cwd: webRoot,
      env: {
        ...process.env,
        MDM_E2E_RUN_ROOT: runRoot,
        MDM_E2E_MEDIA_DIR: mediaDir,
        MDM_E2E_SETTINGS_MEDIA_DIR: settingsMediaDir,
      },
      stdio: "inherit",
    });
    if (test.error !== undefined) throw test.error;
    return test.status ?? 1;
  } finally {
    rmSync(runRoot, { recursive: true, force: true });
  }
}

process.exit(run());

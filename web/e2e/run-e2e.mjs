import { mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import {
  generateFolderFixtures,
  generateFolderSearchFixtures,
  generateMediaFixtures,
  generateSearchFixtures,
  generateTagsFixtures,
} from "./media-fixtures.mjs";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = path.resolve(webRoot, "..");
const runRoot = path.join(repoRoot, ".local", "e2e", `${Date.now()}-${process.pid}`);
const outputDir = path.join(runRoot, "bin");
const output = path.join(outputDir, process.platform === "win32" ? "mdm.exe" : "mdm");
const mediaDir = path.join(runRoot, "media");
const settingsMediaDir = path.join(runRoot, "settings-media");
const foldersMediaDir = path.join(runRoot, "folders-media");
const searchMediaDir = path.join(runRoot, "search-media");
const foldersSearchMediaDir = path.join(runRoot, "folders-search-media");
const tagsMediaDir = path.join(runRoot, "tags-media");
// Go のビルドキャッシュは実行をまたいで使い回す。実行ごとの runRoot に置くと
// 毎回ゼロからのコンパイルになる。既定の置き場（GOCACHE）が決まっていれば
// それに従い（CI は setup-go が復元した場所を渡す）、無ければ作業ツリーの中の
// .local に置く。作業ツリーの外に書けない環境でも動くようにするためである。
const goCache = process.env.GOCACHE ?? path.join(repoRoot, ".local", "go-build");

function run() {
  mkdirSync(outputDir, { recursive: true });
  try {
    generateMediaFixtures(mediaDir);
    generateFolderFixtures(foldersMediaDir);
    generateSearchFixtures(searchMediaDir);
    generateFolderSearchFixtures(foldersSearchMediaDir);
    generateTagsFixtures(tagsMediaDir);
    const mediaContract = spawnSync(
      "go",
      ["test", "./internal/media", "-run", "^TestVideoEncode.*WithFFmpeg$", "-count=1"],
      {
        cwd: repoRoot,
        env: { ...process.env, GOCACHE: goCache },
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
        env: { ...process.env, GOCACHE: goCache },
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
        MDM_E2E_FOLDERS_MEDIA_DIR: foldersMediaDir,
        MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR: foldersSearchMediaDir,
        MDM_E2E_SEARCH_MEDIA_DIR: searchMediaDir,
        MDM_E2E_TAGS_MEDIA_DIR: tagsMediaDir,
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

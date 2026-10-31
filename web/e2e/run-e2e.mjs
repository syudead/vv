import { createHash } from "node:crypto";
import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import {
  generateAssistantFixtures,
  generateDragOrganizeFixtures,
  generateFolderFixtures,
  generateHistoryFixtures,
  generateHomeFixtures,
  generateOnboardingFixtures,
  generateMediaFixtures,
  generatePeerFixtures,
  generateSearchFixtures,
  generateTagsFixtures,
  generateVideoTabsFixtures,
  generateWorkInfoFixtures,
  prepareSearchFixtures,
} from "./media-fixtures.mjs";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = path.resolve(webRoot, "..");
const runRoot = path.join(repoRoot, ".local", "e2e", `${Date.now()}-${process.pid}`);
const outputDir = path.join(runRoot, "bin");
const output = path.join(outputDir, process.platform === "win32" ? "mdm.exe" : "mdm");
// Keep generated cache and per-run fixture directory layouts identical.
function fixtureDirs(root) {
  return {
    mediaDir: path.join(root, "media"),
    foldersMediaDir: path.join(root, "folders-media"),
    searchMediaDir: path.join(root, "search-media"),
    tagsMediaDir: path.join(root, "tags-media"),
    dragOrganizeMediaDir: path.join(root, "drag-organize-media"),
    historyMediaDir: path.join(root, "history-media"),
    homeMediaDir: path.join(root, "home-media"),
    onboardingMediaDir: path.join(root, "onboarding-media"),
    workInfoMediaDir: path.join(root, "work-info-media"),
    videoTabsMediaDir: path.join(root, "video-tabs-media"),
    assistantMediaDir: path.join(root, "assistant-media"),
    peerMediaDir: path.join(root, "peer-media"),
  };
}

function generateFixtures(dirs) {
  generateMediaFixtures(dirs.mediaDir);
  generateFolderFixtures(dirs.foldersMediaDir);
  generateSearchFixtures(dirs.searchMediaDir);
  generateTagsFixtures(dirs.tagsMediaDir);
  generateDragOrganizeFixtures(dirs.dragOrganizeMediaDir);
  generateHistoryFixtures(dirs.historyMediaDir);
  generateHomeFixtures(dirs.homeMediaDir);
  generateOnboardingFixtures(dirs.onboardingMediaDir);
  generateWorkInfoFixtures(dirs.workInfoMediaDir);
  generateVideoTabsFixtures(dirs.videoTabsMediaDir);
  generateAssistantFixtures(dirs.assistantMediaDir);
  generatePeerFixtures(dirs.peerMediaDir);
}

// Cache FFmpeg fixtures by generator sources, directory layout and FFmpeg version; CI preserves this cache between runs.
const fixtureCacheRoot =
  process.env.MDM_E2E_FIXTURE_CACHE ?? path.join(repoRoot, ".local", "e2e-fixtures");

function fixtureKey() {
  const hash = createHash("sha256");
  hash.update(readFileSync(path.join(webRoot, "e2e", "media-fixtures.mjs")));
  hash.update(readFileSync(fileURLToPath(import.meta.url)));
  const version = spawnSync("ffmpeg", ["-version"], { encoding: "utf8" });
  if (version.error !== undefined) throw version.error;
  hash.update(version.stdout.split("\n")[0] ?? "");
  return hash.digest("hex").slice(0, 16);
}

// Copy cached fixtures into runRoot, then refresh time-dependent metadata.
function prepareFixtures(dirs) {
  const cached = path.join(fixtureCacheRoot, fixtureKey());
  const complete = path.join(cached, ".complete");
  if (!existsSync(complete)) {
    const staging = `${cached}.tmp-${String(process.pid)}`;
    rmSync(staging, { recursive: true, force: true });
    generateFixtures(fixtureDirs(staging));
    writeFileSync(path.join(staging, ".complete"), "");
    rmSync(cached, { recursive: true, force: true });
    renameSync(staging, cached);
  } else {
    console.log(`e2e: テストの動画をキャッシュから写す（${cached}）`);
  }
  cpSync(cached, runRoot, {
    recursive: true,
    preserveTimestamps: true,
    filter: (source) => source !== complete,
  });
  prepareSearchFixtures(dirs.searchMediaDir);
}

const fixtures = fixtureDirs(runRoot);
// Reuse GOCACHE across runs; otherwise use .local so restricted workspaces need no writes outside the checkout.
const goCache = process.env.GOCACHE ?? path.join(repoRoot, ".local", "go-build");

function run() {
  mkdirSync(outputDir, { recursive: true });
  try {
    prepareFixtures(fixtures);
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
    const test = spawnSync(
      process.execPath,
      [playwrightCLI, "test", ...process.argv.slice(2)],
      {
        cwd: webRoot,
        env: {
          ...process.env,
          MDM_E2E_RUN_ROOT: runRoot,
          MDM_E2E_MEDIA_DIR: fixtures.mediaDir,
          MDM_E2E_FOLDERS_MEDIA_DIR: fixtures.foldersMediaDir,
          MDM_E2E_SEARCH_MEDIA_DIR: fixtures.searchMediaDir,
          MDM_E2E_TAGS_MEDIA_DIR: fixtures.tagsMediaDir,
          MDM_E2E_DRAG_ORGANIZE_MEDIA_DIR: fixtures.dragOrganizeMediaDir,
          MDM_E2E_HISTORY_MEDIA_DIR: fixtures.historyMediaDir,
          MDM_E2E_HOME_MEDIA_DIR: fixtures.homeMediaDir,
          MDM_E2E_ONBOARDING_MEDIA_DIR: fixtures.onboardingMediaDir,
          MDM_E2E_WORK_INFO_MEDIA_DIR: fixtures.workInfoMediaDir,
          MDM_E2E_VIDEO_TABS_MEDIA_DIR: fixtures.videoTabsMediaDir,
          MDM_E2E_ASSISTANT_MEDIA_DIR: fixtures.assistantMediaDir,
          MDM_E2E_PEER_MEDIA_DIR: fixtures.peerMediaDir,
        },
        stdio: "inherit",
      },
    );
    if (test.error !== undefined) throw test.error;
    return test.status ?? 1;
  } finally {
    rmSync(runRoot, { recursive: true, force: true });
  }
}

process.exit(run());

import { mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import {
  generateDragOrganizeFixtures,
  generateFolderFixtures,
  generateFolderSearchFixtures,
  generateGroupsFixtures,
  generateGuestFixtures,
  generateHistoryFixtures,
  generateHomeFixtures,
  generateOnboardingFixtures,
  generateChapterFixtures,
  generateMediaFixtures,
  generateMediaInfoFixtures,
  generatePlaylistsFixtures,
  generateScanIssueFixtures,
  generateScanProgressFixtures,
  generateSearchFixtures,
  generateSmartFoldersFixtures,
  generateTagsFixtures,
  generateThumbnailEditFixtures,
  generateThumbnailUploadFixtures,
  generateWorkInfoFixtures,
  generateWorkInfoPluginFixtures,
} from "./media-fixtures.mjs";
import { buildWorkInfoPlugin } from "./work-info-plugin.mjs";
import { FAKE_COMFYUI_PORT } from "./fake-comfyui.mjs";

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
const guestMediaDir = path.join(runRoot, "guest-media");
const groupsMediaDir = path.join(runRoot, "groups-media");
// 再スキャンの前にテストが写す 1 本は、登録フォルダの外に置く。
const groupsSpareDir = path.join(runRoot, "groups-spare");
const dragOrganizeMediaDir = path.join(runRoot, "drag-organize-media");
const historyMediaDir = path.join(runRoot, "history-media");
const homeMediaDir = path.join(runRoot, "home-media");
const onboardingMediaDir = path.join(runRoot, "onboarding-media");
const smartFoldersMediaDir = path.join(runRoot, "smart-folders-media");
const playlistsMediaDir = path.join(runRoot, "playlists-media");
const scanProgressMediaDir = path.join(runRoot, "scan-progress-media");
const scanIssuesMediaDir = path.join(runRoot, "scan-issues-media");
const thumbnailEditMediaDir = path.join(runRoot, "thumbnail-edit-media");
const workInfoMediaDir = path.join(runRoot, "work-info-media");
const mediaInfoMediaDir = path.join(runRoot, "media-info-media");
const workInfoPluginsMediaDir = path.join(runRoot, "work-info-plugins-media");
// work-info-plugins.e2e.ts が設定画面から上げる作品情報プラグイン。動画のフォルダの外に置く。
const workInfoPlugin = path.join(runRoot, "plugins", "e2e-works.wasm");
// ページから上げる画像は動画のフォルダの外に置き、スキャンが拾わないようにする。
const thumbnailUploadDir = path.join(runRoot, "thumbnail-uploads");
// Go のビルドキャッシュは実行をまたいで使い回す。実行ごとの runRoot に置くと
// 毎回ゼロからのコンパイルになる。既定の置き場（GOCACHE）が決まっていれば
// それに従い（CI は setup-go が復元した場所を渡す）、無ければ作業ツリーの中の
// .local に置く。作業ツリーの外に書けない環境でも動くようにするためである。
const goCache = process.env.GOCACHE ?? path.join(repoRoot, ".local", "go-build");

function run() {
  mkdirSync(outputDir, { recursive: true });
  // 偽の ComfyUI（thumbnail-edit.e2e.ts の接続先）は別のプロセスで動かす。Playwright を
  // spawnSync で待つ間、このプロセスは要求に答えられないためである。標準入力が閉じると
  // 終わるので、このプロセスが落ちても残らない。
  const fakeComfyUI = spawn(
    process.execPath,
    [path.join(webRoot, "e2e", "fake-comfyui.mjs")],
    { stdio: ["pipe", "inherit", "inherit"] },
  );
  try {
    generateMediaFixtures(mediaDir);
    generateFolderFixtures(foldersMediaDir);
    generateSearchFixtures(searchMediaDir);
    generateFolderSearchFixtures(foldersSearchMediaDir);
    generateTagsFixtures(tagsMediaDir);
    generateGuestFixtures(guestMediaDir);
    generateGroupsFixtures(groupsMediaDir, groupsSpareDir);
    generateDragOrganizeFixtures(dragOrganizeMediaDir);
    generateHistoryFixtures(historyMediaDir);
    generateHomeFixtures(homeMediaDir);
    generateOnboardingFixtures(onboardingMediaDir);
    generateSmartFoldersFixtures(smartFoldersMediaDir);
    generatePlaylistsFixtures(playlistsMediaDir);
    generateScanProgressFixtures(scanProgressMediaDir);
    generateScanIssueFixtures(scanIssuesMediaDir);
    generateThumbnailEditFixtures(thumbnailEditMediaDir);
    generateThumbnailUploadFixtures(thumbnailUploadDir);
    generateWorkInfoFixtures(workInfoMediaDir);
    generateMediaInfoFixtures(mediaInfoMediaDir);
    generateChapterFixtures(mediaInfoMediaDir);
    generateWorkInfoPluginFixtures(workInfoPluginsMediaDir);
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
    mkdirSync(path.dirname(workInfoPlugin), { recursive: true });
    buildWorkInfoPlugin(repoRoot, workInfoPlugin, output, {
      ...process.env,
      GOCACHE: goCache,
    });

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
          MDM_E2E_MEDIA_DIR: mediaDir,
          MDM_E2E_SETTINGS_MEDIA_DIR: settingsMediaDir,
          MDM_E2E_FOLDERS_MEDIA_DIR: foldersMediaDir,
          MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR: foldersSearchMediaDir,
          MDM_E2E_SEARCH_MEDIA_DIR: searchMediaDir,
          MDM_E2E_TAGS_MEDIA_DIR: tagsMediaDir,
          MDM_E2E_GUEST_MEDIA_DIR: guestMediaDir,
          MDM_E2E_GROUPS_MEDIA_DIR: groupsMediaDir,
          MDM_E2E_GROUPS_SPARE_DIR: groupsSpareDir,
          MDM_E2E_DRAG_ORGANIZE_MEDIA_DIR: dragOrganizeMediaDir,
          MDM_E2E_HISTORY_MEDIA_DIR: historyMediaDir,
          MDM_E2E_HOME_MEDIA_DIR: homeMediaDir,
          MDM_E2E_ONBOARDING_MEDIA_DIR: onboardingMediaDir,
          MDM_E2E_SMART_FOLDERS_MEDIA_DIR: smartFoldersMediaDir,
          MDM_E2E_PLAYLISTS_MEDIA_DIR: playlistsMediaDir,
          MDM_E2E_SCAN_PROGRESS_MEDIA_DIR: scanProgressMediaDir,
          MDM_E2E_SCAN_ISSUES_MEDIA_DIR: scanIssuesMediaDir,
          MDM_E2E_THUMBNAIL_EDIT_MEDIA_DIR: thumbnailEditMediaDir,
          MDM_E2E_THUMBNAIL_UPLOAD_DIR: thumbnailUploadDir,
          MDM_E2E_WORK_INFO_MEDIA_DIR: workInfoMediaDir,
          MDM_E2E_MEDIA_INFO_MEDIA_DIR: mediaInfoMediaDir,
          MDM_E2E_WORK_INFO_PLUGINS_MEDIA_DIR: workInfoPluginsMediaDir,
          MDM_E2E_WORK_INFO_PLUGIN: workInfoPlugin,
          MDM_E2E_COMFYUI_URL: `http://127.0.0.1:${String(FAKE_COMFYUI_PORT)}`,
        },
        stdio: "inherit",
      },
    );
    if (test.error !== undefined) throw test.error;
    return test.status ?? 1;
  } finally {
    fakeComfyUI.kill();
    rmSync(runRoot, { recursive: true, force: true });
  }
}

process.exit(run());

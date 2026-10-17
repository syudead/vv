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
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import {
  generateActivityFixtures,
  generateActivityVersionFixtures,
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
  prepareScanIssueFixtures,
  prepareSearchFixtures,
} from "./media-fixtures.mjs";
import { buildWorkInfoPlugin } from "./work-info-plugin.mjs";
import { FAKE_COMFYUI_PORT } from "./fake-comfyui.mjs";

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repoRoot = path.resolve(webRoot, "..");
const runRoot = path.join(repoRoot, ".local", "e2e", `${Date.now()}-${process.pid}`);
const outputDir = path.join(runRoot, "bin");
const output = path.join(outputDir, process.platform === "win32" ? "mdm.exe" : "mdm");
// fixtureDirs は、テストの動画を置くフォルダを root の下に並べる。生成はキャッシュの下へ、
// 実行は runRoot の下へ、同じ並びで置く。
function fixtureDirs(root) {
  return {
    mediaDir: path.join(root, "media"),
    foldersMediaDir: path.join(root, "folders-media"),
    searchMediaDir: path.join(root, "search-media"),
    foldersSearchMediaDir: path.join(root, "folders-search-media"),
    tagsMediaDir: path.join(root, "tags-media"),
    guestMediaDir: path.join(root, "guest-media"),
    groupsMediaDir: path.join(root, "groups-media"),
    // 再スキャンの前にテストが写す 1 本は、登録フォルダの外に置く。
    groupsSpareDir: path.join(root, "groups-spare"),
    dragOrganizeMediaDir: path.join(root, "drag-organize-media"),
    historyMediaDir: path.join(root, "history-media"),
    homeMediaDir: path.join(root, "home-media"),
    onboardingMediaDir: path.join(root, "onboarding-media"),
    activityMediaDir: path.join(root, "activity-media"),
    activityVersionsMediaDir: path.join(root, "activity-versions-media"),
    smartFoldersMediaDir: path.join(root, "smart-folders-media"),
    playlistsMediaDir: path.join(root, "playlists-media"),
    scanProgressMediaDir: path.join(root, "scan-progress-media"),
    scanIssuesMediaDir: path.join(root, "scan-issues-media"),
    thumbnailEditMediaDir: path.join(root, "thumbnail-edit-media"),
    workInfoMediaDir: path.join(root, "work-info-media"),
    mediaInfoMediaDir: path.join(root, "media-info-media"),
    workInfoPluginsMediaDir: path.join(root, "work-info-plugins-media"),
    // ページから上げる画像は動画のフォルダの外に置き、スキャンが拾わないようにする。
    thumbnailUploadDir: path.join(root, "thumbnail-uploads"),
  };
}

function generateFixtures(dirs) {
  generateMediaFixtures(dirs.mediaDir);
  generateFolderFixtures(dirs.foldersMediaDir);
  generateSearchFixtures(dirs.searchMediaDir);
  generateFolderSearchFixtures(dirs.foldersSearchMediaDir);
  generateTagsFixtures(dirs.tagsMediaDir);
  generateGuestFixtures(dirs.guestMediaDir);
  generateGroupsFixtures(dirs.groupsMediaDir, dirs.groupsSpareDir);
  generateDragOrganizeFixtures(dirs.dragOrganizeMediaDir);
  generateHistoryFixtures(dirs.historyMediaDir);
  generateHomeFixtures(dirs.homeMediaDir);
  generateOnboardingFixtures(dirs.onboardingMediaDir);
  generateActivityFixtures(dirs.activityMediaDir);
  generateActivityVersionFixtures(dirs.activityVersionsMediaDir);
  generateSmartFoldersFixtures(dirs.smartFoldersMediaDir);
  generatePlaylistsFixtures(dirs.playlistsMediaDir);
  generateScanProgressFixtures(dirs.scanProgressMediaDir);
  generateScanIssueFixtures(dirs.scanIssuesMediaDir);
  generateThumbnailEditFixtures(dirs.thumbnailEditMediaDir);
  generateThumbnailUploadFixtures(dirs.thumbnailUploadDir);
  generateWorkInfoFixtures(dirs.workInfoMediaDir);
  generateMediaInfoFixtures(dirs.mediaInfoMediaDir);
  generateChapterFixtures(dirs.mediaInfoMediaDir);
  generateWorkInfoPluginFixtures(dirs.workInfoPluginsMediaDir);
}

// テストの動画は ffmpeg で作ると 1 回 20 秒ほどかかるので、作り方（media-fixtures.mjs、
// 生成先の並びを決めるこのファイル、写す画像）と ffmpeg の版が同じあいだは、一度作った
// ものを写して使う。CI はこの置き場を
// actions/cache で持ち越す（.github/workflows/ci.yml）。
const fixtureCacheRoot =
  process.env.MDM_E2E_FIXTURE_CACHE ?? path.join(repoRoot, ".local", "e2e-fixtures");

function fixtureKey() {
  const hash = createHash("sha256");
  hash.update(readFileSync(path.join(webRoot, "e2e", "media-fixtures.mjs")));
  hash.update(readFileSync(fileURLToPath(import.meta.url)));
  hash.update(
    readFileSync(path.join(webRoot, "e2e", "fixtures", "thumbnail-upload.webp")),
  );
  const version = spawnSync("ffmpeg", ["-version"], { encoding: "utf8" });
  if (version.error !== undefined) throw version.error;
  hash.update(version.stdout.split("\n")[0] ?? "");
  return hash.digest("hex").slice(0, 16);
}

// prepareFixtures は、キャッシュに無ければ作ってから、runRoot へ写す。写したあとで、
// 写せないもの（名前付きパイプ）と実行の時刻に合わせるもの（更新時刻）を整える。
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
  prepareScanIssueFixtures(dirs.scanIssuesMediaDir);
  prepareSearchFixtures(dirs.searchMediaDir);
}

const fixtures = fixtureDirs(runRoot);
const settingsMediaDir = path.join(runRoot, "settings-media");
// work-info-plugins.e2e.ts が設定画面から上げる作品情報プラグイン。動画のフォルダの外に置く。
const workInfoPlugin = path.join(runRoot, "plugins", "e2e-works.wasm");
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
          MDM_E2E_MEDIA_DIR: fixtures.mediaDir,
          MDM_E2E_SETTINGS_MEDIA_DIR: settingsMediaDir,
          MDM_E2E_FOLDERS_MEDIA_DIR: fixtures.foldersMediaDir,
          MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR: fixtures.foldersSearchMediaDir,
          MDM_E2E_SEARCH_MEDIA_DIR: fixtures.searchMediaDir,
          MDM_E2E_TAGS_MEDIA_DIR: fixtures.tagsMediaDir,
          MDM_E2E_GUEST_MEDIA_DIR: fixtures.guestMediaDir,
          MDM_E2E_GROUPS_MEDIA_DIR: fixtures.groupsMediaDir,
          MDM_E2E_GROUPS_SPARE_DIR: fixtures.groupsSpareDir,
          MDM_E2E_DRAG_ORGANIZE_MEDIA_DIR: fixtures.dragOrganizeMediaDir,
          MDM_E2E_HISTORY_MEDIA_DIR: fixtures.historyMediaDir,
          MDM_E2E_HOME_MEDIA_DIR: fixtures.homeMediaDir,
          MDM_E2E_ONBOARDING_MEDIA_DIR: fixtures.onboardingMediaDir,
          MDM_E2E_ACTIVITY_MEDIA_DIR: fixtures.activityMediaDir,
          MDM_E2E_ACTIVITY_VERSIONS_MEDIA_DIR: fixtures.activityVersionsMediaDir,
          MDM_E2E_SMART_FOLDERS_MEDIA_DIR: fixtures.smartFoldersMediaDir,
          MDM_E2E_PLAYLISTS_MEDIA_DIR: fixtures.playlistsMediaDir,
          MDM_E2E_SCAN_PROGRESS_MEDIA_DIR: fixtures.scanProgressMediaDir,
          MDM_E2E_SCAN_ISSUES_MEDIA_DIR: fixtures.scanIssuesMediaDir,
          MDM_E2E_THUMBNAIL_EDIT_MEDIA_DIR: fixtures.thumbnailEditMediaDir,
          MDM_E2E_THUMBNAIL_UPLOAD_DIR: fixtures.thumbnailUploadDir,
          MDM_E2E_WORK_INFO_MEDIA_DIR: fixtures.workInfoMediaDir,
          MDM_E2E_MEDIA_INFO_MEDIA_DIR: fixtures.mediaInfoMediaDir,
          MDM_E2E_WORK_INFO_PLUGINS_MEDIA_DIR: fixtures.workInfoPluginsMediaDir,
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

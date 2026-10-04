import { createHash } from "node:crypto";
import { mkdir, readFile, readdir } from "node:fs/promises";
import path from "node:path";

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
  type Request,
} from "@playwright/test";

interface Video {
  id: number;
  title: string;
  playable: boolean;
  probeState: string;
  durationMs?: number;
  container?: string;
  videoCodec?: string;
  audioCodec?: string;
  seekThumbnailUrl?: string;
}

interface MediaFolder {
  id: number;
  version: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
const videos = new Map<string, Video>();
let folder: MediaFolder | undefined;
let sourceSnapshot = new Map<string, string>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

async function snapshot(directory: string): Promise<Map<string, string>> {
  const result = new Map<string, string>();
  for (const name of (await readdir(directory)).sort()) {
    const content = await readFile(path.join(directory, name));
    result.set(name, createHash("sha256").update(content).digest("hex"));
  }
  return result;
}

async function waitForScan(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        if (!response.ok()) return "missing";
        return ((await response.json()) as { state: string }).state;
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

async function waitForVideos(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/videos?limit=100");
        const page = (await response.json()) as { items: Video[] };
        for (const item of page.items) videos.set(item.title, item);
        return [...videos.values()].filter((item) => item.probeState === "done").length;
      },
      { timeout: 60_000 },
    )
    .toBe(12);
}

async function waitForSeekThumbnails(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        let ready = 0;
        for (const item of videos.values()) {
          if (item.seekThumbnailUrl === undefined) continue;
          // 配置情報は、スプライトが完成するまで 409 を返す。
          const response = await request.get(item.seekThumbnailUrl);
          if (response.ok()) ready++;
        }
        return ready;
      },
      { timeout: 60_000 },
    )
    .toBe(
      [...videos.values()].filter((item) => item.seekThumbnailUrl !== undefined).length,
    );
}

/**
 * play は動画のページを開き、大きな再生ボタンで再生を始める。
 *
 * 再生を始める前に動画の取得が失敗すると、プレイヤーの上に失敗の層（role="alert"）が
 * 出て、大きな再生ボタンは隠れたままになる（vjs-has-started）。ボタンだけを待つと、
 * テストの期限まで「ボタンが見えない」とだけ言って止まる（#256・#259 のマージ後に、
 * 配信が 404 を返したときがそうだった）。最初の読み込みの成否が決まるまで待ち、
 * 失敗の層が出たら、その文言と配信の応答を添えてすぐに失敗させる。
 */
async function play(page: Page, item: Video) {
  const mediaResponses: string[] = [];
  page.on("response", (response) => {
    const url = new URL(response.url());
    if (/^\/api\/videos\/\d+\/(stream|transcode\.mp4)$/.test(url.pathname)) {
      mediaResponses.push(`${String(response.status())} ${url.pathname}${url.search}`);
    }
  });
  const started = Date.now();
  await page.goto(`/videos/${String(item.id)}`);
  // 押す前に、最初の読み込み（preload=metadata）の成否が決まるのを待つ。ボタンが
  // 見えた直後に取得が失敗すると、押そうとしている間にボタンが隠れるためである。
  await page.waitForFunction(
    () => {
      const failed = document.querySelector('[data-player-frame] [role="alert"]');
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      return failed !== null || (element !== null && element.readyState >= 1);
    },
    undefined,
    { timeout: 10_000 },
  );
  const button = page.locator(".vjs-big-play-button");
  const failure = page.locator("[data-player-frame]").getByRole("alert").first();
  if (await failure.isVisible()) {
    throw new Error(
      `${item.title} (id ${String(item.id)}) の再生を始める前に失敗の層が出た: ` +
        `${(await failure.innerText()).replaceAll("\n", " ")} / 配信の応答: ` +
        `${mediaResponses.join(", ") || "なし"}`,
    );
  }
  await button.click();
  await page.waitForFunction(() => {
    const element = document.querySelector("video");
    return (
      element !== null &&
      !element.paused &&
      element.readyState >= 2 &&
      element.currentTime > 0.1
    );
  });
  expect(Date.now() - started).toBeLessThan(3000);
}

/**
 * switchTo480p は直接再生を始め、再生中に操作バーの画質メニューから 480p を選ぶ。
 * 480p の変換へ差し替わり、映像の高さが 480 になり、表示の時刻が戻らないことを確かめる。
 */
async function switchTo480p(page: Page, item: Video) {
  await page.setViewportSize({ width: 1280, height: 800 });
  await play(page, item);
  await page.waitForFunction(() => {
    const element = document.querySelector("video");
    return element !== null && element.currentTime > 2;
  });
  const height = () =>
    page.evaluate(() => document.querySelector("video")?.videoHeight ?? 0);
  expect(await height()).toBe(1080);
  const quality = page.locator(".vjs-control-bar > .vv-quality");
  await page.locator(".video-js").hover();
  await expect(quality.locator(".vv-quality-value")).toHaveText("1080p");
  await expect(quality.getByRole("button", { name: "Quality" })).toBeVisible();

  const transcode = page.waitForRequest((candidate) => {
    const url = new URL(candidate.url());
    return (
      url.pathname === `/api/videos/${String(item.id)}/transcode.mp4` &&
      url.searchParams.get("quality") === "480p"
    );
  });
  const before = await displayedSeconds(page);
  await quality.hover();
  await page.getByRole("menuitemradio", { name: /^480p/ }).click();
  const request = await transcode;
  expect(
    Number(new URL(request.url()).searchParams.get("startMs")),
  ).toBeGreaterThanOrEqual((before - 1) * 1000);
  await expect(quality.locator(".vv-quality-value")).toHaveText("480p");
  await expect(page.getByRole("button", { name: "Converting to 480p" })).toBeVisible();
  await expect.poll(height, { timeout: 15_000 }).toBe(480);
  await page.waitForFunction(() => {
    const element = document.querySelector("video");
    return element !== null && !element.paused && element.readyState >= 2;
  });
  expect(await displayedSeconds(page)).toBeGreaterThanOrEqual(before);
}

function mediaRequests(page: Page): Request[] {
  const requests: Request[] = [];
  page.on("request", (request) => {
    if (/\/api\/videos\/\d+\/(stream|transcode\.mp4)/.test(request.url())) {
      requests.push(request);
    }
  });
  return requests;
}

/**
 * reportedStart は変換の要求に付いた attempt の、実際の開始位置の報告を待って返す
 * （specs/018-live-transcode-seek/contracts/transcode-start-api.md）。
 */
async function reportedStart(page: Page, transcode: Request): Promise<number> {
  const attempt = new URL(transcode.url()).searchParams.get("attempt");
  if (attempt === null) throw new Error(`attempt が無い: ${transcode.url()}`);
  const response = await page.waitForResponse(
    (candidate) =>
      candidate.url().includes("/transcode-start?") &&
      new URL(candidate.url()).searchParams.get("attempt") === attempt,
  );
  expect(response.status()).toBe(200);
  return ((await response.json()) as { startMs: number }).startMs;
}

/**
 * pauseAndRecordProgress は再生中の動画を操作バーで止め、止まったあとの保存の位置を返す。
 * 再生中は 5 秒ごとにも保存を送る（web/src/player/VideoPlayer.tsx の saveIntervalMs）ので、
 * 押す前に待ち始めた最初の PUT が止めたときの保存とは限らない。止まったのを確かめてから、
 * 最後に送った保存の位置を使う。`lastSaved` は、そのあと（再読み込みで離れるときの保存も
 * 含む）に送った最後の位置を返す。
 */
async function pauseAndRecordProgress(page: Page, item: Video) {
  const saves: number[] = [];
  page.on("request", (candidate) => {
    if (
      candidate.url().endsWith(`/api/videos/${String(item.id)}/progress`) &&
      candidate.method() === "PUT"
    ) {
      saves.push((candidate.postDataJSON() as { positionMs: number }).positionMs);
    }
  });
  const before = saves.length;
  await page.locator(".vjs-play-control").click();
  await page.waitForFunction(() => document.querySelector("video")?.paused === true);
  await expect.poll(() => saves.length).toBeGreaterThan(before);
  const lastSaved = () => {
    const last = saves.at(-1);
    if (last === undefined) throw new Error("再生位置の保存が送られていない");
    return last;
  };
  return { saved: lastSaved(), lastSaved };
}

/** reloadAndWaitForTranscode は再読み込みのあとのライブ変換の要求と、その開始位置を返す。 */
async function reloadAndWaitForTranscode(page: Page, item: Video) {
  const resumedPromise = page.waitForRequest((candidate) =>
    candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`),
  );
  await page.reload();
  const resumed = await resumedPromise;
  return {
    resumed,
    startMs: Number(new URL(resumed.url()).searchParams.get("startMs")),
  };
}

/** frameColor は再生中の映像の左上の 1 画素の RGBA を返す。 */
async function frameColor(page: Page): Promise<number[]> {
  return page.evaluate(() => {
    const element = document.querySelector("video");
    if (element === null) throw new Error("video is missing");
    const canvas = document.createElement("canvas");
    canvas.width = 1;
    canvas.height = 1;
    const context = canvas.getContext("2d");
    if (context === null) throw new Error("2d context is missing");
    context.drawImage(element, 0, 0, 1, 1);
    return [...context.getImageData(0, 0, 1, 1).data];
  });
}

/** displayedSeconds は操作バーの現在時刻の表示（m:ss）を秒で返す。 */
async function displayedSeconds(page: Page): Promise<number> {
  const text = await page.locator(".vjs-current-time-display").innerText();
  const match = /(\d+):(\d{2})/.exec(text);
  if (match === null) throw new Error(`現在時刻を読めない: ${text}`);
  return Number(match[1]) * 60 + Number(match[2]);
}

async function saveProgress(request: APIRequestContext, item: Video, positionMs: number) {
  const response = await request.put(`/api/videos/${String(item.id)}/progress`, {
    headers: mutationHeaders,
    data: { positionMs },
  });
  expect(response.ok()).toBe(true);
}

async function throttle(page: Page, bytesPerSecond: number) {
  const session = await page.context().newCDPSession(page);
  await session.send("Network.enable");
  await session.send("Network.emulateNetworkConditions", {
    offline: false,
    latency: 0,
    downloadThroughput: bytesPerSecond,
    uploadThroughput: bytesPerSecond,
    connectionType: "wifi",
  });
}

/**
 * settleResources は、画面が読み込み始めたフォントと画像を読み終えるまで待つ。
 * 帯域を絞ったページでは、読み込み途中のフォントや関連動画のサムネイルが帯域と
 * 接続を使い、次のページへの移動を遅らせる。離脱の速さを測る前に済ませておき、
 * 変換の停止とは関係のない読み込みを測らない。
 */
async function settleResources(page: Page) {
  await page.waitForFunction(async () => {
    await document.fonts.ready;
    return Array.from(document.images).every((image) => image.complete);
  });
}

test.describe.serial("live MP4 playback", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
    if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
    sourceSnapshot = await snapshot(mediaDir);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaDir },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request);
    await waitForVideos(request);
    await waitForSeekThumbnails(request);
  });

  test.afterAll(async ({ request }) => {
    const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
    if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
    expect(await snapshot(mediaDir)).toEqual(sourceSnapshot);
    if (folder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("形式matrixをdirectまたはtranscodeの正しい経路で3秒以内に再生する", async ({
    browser,
  }) => {
    test.setTimeout(90_000);
    const matrix = [
      { title: "direct", direct: true, video: "h264", audio: "aac" },
      { title: "container-only", direct: false, video: "h264", audio: "aac" },
      { title: "container-only-mov", direct: false, video: "h264", audio: "aac" },
      { title: "video-only", direct: false, video: "mpeg4", audio: "aac" },
      { title: "audio-only", direct: false, video: "h264", audio: "flac" },
      { title: "video-audio", direct: false, video: "mpeg4", audio: "pcm_s16le" },
      { title: "silent", direct: false, video: "mpeg4", audio: undefined },
    ];

    for (const expected of matrix) {
      const page = await browser.newPage();
      const requests = mediaRequests(page);
      const item = video(expected.title);
      expect(item.playable).toBe(expected.direct);
      expect(item.videoCodec).toBe(expected.video);
      expect(item.audioCodec).toBe(expected.audio);

      await play(page, item);

      const direct = requests.filter((request) => request.url().endsWith("/stream"));
      const transcode = requests.filter((request) =>
        request.url().includes("/transcode.mp4"),
      );
      if (expected.direct) expect(direct.length).toBeGreaterThan(0);
      else expect(direct).toHaveLength(0);
      expect(transcode.length).toBe(expected.direct ? 0 : 1);
      await page.close();
    }
  });

  test("direct errorは保存位置から1回だけfallbackし、transcode errorはalertで止まる", async ({
    browser,
    request,
  }) => {
    test.setTimeout(30_000);
    const direct = video("direct-fallback");
    await saveProgress(request, direct, 5000);

    const fallbackPage = await browser.newPage();
    const fallbackRequests = mediaRequests(fallbackPage);
    await fallbackPage.route(
      `**/api/videos/${String(direct.id)}/stream`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "video/mp4",
          body: "invalid media",
        });
      },
    );
    const fallback = fallbackPage.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(direct.id)}/transcode.mp4`),
    );
    await fallbackPage.goto(`/videos/${String(direct.id)}`);
    const fallbackRequest = await fallback;
    expect(new URL(fallbackRequest.url()).searchParams.get("startMs")).toBe("5000");
    await fallbackPage.locator(".vjs-big-play-button").click();
    await fallbackPage.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime > 0.1;
    });
    expect(
      fallbackRequests.filter((candidate) => candidate.url().includes("/transcode.mp4")),
    ).toHaveLength(1);
    await fallbackPage.close();

    const failed = video("container-only");
    const failurePage = await browser.newPage();
    await failurePage.setViewportSize({ width: 360, height: 800 });
    const failureRequests = mediaRequests(failurePage);
    await failurePage.route(
      `**/api/videos/${String(failed.id)}/transcode.mp4*`,
      async (route) => route.abort("failed"),
    );
    const started = Date.now();
    await failurePage.goto(`/videos/${String(failed.id)}`);
    await expect(failurePage.getByRole("alert")).toBeVisible({ timeout: 10_000 });
    if (screenshotDir !== undefined) {
      await mkdir(screenshotDir, { recursive: true });
      await failurePage.screenshot({
        path: path.join(screenshotDir, "20260922-live-mp4-e2e-error-360.png"),
        fullPage: true,
      });
    }
    expect(Date.now() - started).toBeLessThan(10_000);
    await failurePage.waitForTimeout(500);
    expect(
      failureRequests.filter((candidate) => candidate.url().includes("/transcode.mp4")),
    ).toHaveLength(1);
    await failurePage.close();
  });

  test("長いGOPのresumeと未buffer seekで論理時刻と映像内容を揃える", async ({
    page,
    request,
  }) => {
    test.setTimeout(30_000);
    await page.goto("/");
    await throttle(page, 128 * 1024);
    const item = video("long-gop");
    await saveProgress(request, item, 5000);
    const requests = mediaRequests(page);

    const resumed = page.waitForRequest((candidate) =>
      candidate
        .url()
        .includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=5000`),
    );
    await page.goto(`/videos/${String(item.id)}`);
    const initialRequest = await resumed;
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime > 0.1;
    });

    // 絞った回線では再生が始まるまでに 2 秒を超えることがあり、そのあいだに video.js は
    // 操作バーを隠して押せなくする（vjs-user-inactive）。人と同じく、プレイヤーの上で
    // マウスを動かして操作バーを出してから再生バーを押す。
    const playerBox = await page.locator(".video-js").boundingBox();
    if (playerBox === null) throw new Error("player is not visible");
    await page.mouse.move(
      playerBox.x + playerBox.width * 0.75,
      playerBox.y + playerBox.height / 2,
      { steps: 3 },
    );
    await expect(page.locator(".video-js")).toHaveClass(/vjs-user-active/);
    const seekBar = page.locator(".vjs-progress-control");
    const box = await seekBar.boundingBox();
    if (box === null) throw new Error("seek bar is not visible");
    const seekStarted = Date.now();
    const seekRequestPromise = page.waitForRequest(
      (candidate) =>
        candidate
          .url()
          .includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`) &&
        !candidate.url().endsWith("startMs=5000"),
    );
    const oldRequestStopped = page.waitForEvent("requestfailed", {
      predicate: (candidate) => candidate === initialRequest,
      timeout: 5000,
    });
    await page.mouse.click(box.x + box.width * 0.75, box.y + box.height / 2);
    const seekRequest = await seekRequestPromise;
    await oldRequestStopped;
    const seekStart = Number(new URL(seekRequest.url()).searchParams.get("startMs"));
    expect(seekStart).toBeGreaterThan(20_000);
    expect(seekStart).toBeLessThan(25_500);
    // 映像はコピーで直前のキーフレーム（20 秒）から始まる。
    const actualStart = await reportedStart(page, seekRequest);
    expect(actualStart).toBeGreaterThan(19_500);
    expect(actualStart).toBeLessThanOrEqual(seekStart);
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return (
        element !== null &&
        !element.paused &&
        element.readyState >= 2 &&
        element.currentTime > 0.1
      );
    });
    expect(Date.now() - seekStarted).toBeLessThan(2000);

    const color = await frameColor(page);
    expect(color[2]).toBeGreaterThan(color[0] ?? 255);
    expect(color[2]).toBeGreaterThan(color[1] ?? 255);

    const progress = page.waitForRequest(
      (candidate) =>
        candidate.url().endsWith(`/api/videos/${String(item.id)}/progress`) &&
        candidate.method() === "PUT",
    );
    await page.locator(".vjs-play-control").click();
    const body = (await progress).postDataJSON() as { positionMs: number };
    expect(body.positionMs).toBeGreaterThanOrEqual(actualStart);
    expect(body.positionMs).toBeLessThan(seekStart + 5000);
    expect(
      requests.filter((candidate) => candidate.url().includes("/stream")),
    ).toHaveLength(0);
  });

  test("覚えた480pで1080pの直接再生できる動画を変換で始め、未buffer seekのあとも480pを保つ", async ({
    page,
  }) => {
    // specs/027-playback-quality 受け入れ条件 5・7。
    test.setTimeout(45_000);
    const item = video("hd-1080p");
    expect(item.playable).toBe(true);
    await page.goto("/");
    await page.evaluate(() => {
      window.localStorage.setItem("vv.playback-quality.v1", JSON.stringify("480p"));
    });
    // 変換が再生より先に進みすぎて、シーク先まで読み込み済みにならないよう回線を絞る。
    await throttle(page, 256 * 1024);
    const requests = mediaRequests(page);
    const initial = page.waitForRequest((candidate) =>
      candidate
        .url()
        .endsWith(`/api/videos/${String(item.id)}/transcode.mp4?quality=480p`),
    );
    await page.goto(`/videos/${String(item.id)}`);
    await initial;
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return (
        element !== null &&
        !element.paused &&
        element.readyState >= 2 &&
        element.currentTime > 0.1
      );
    });
    const height = () =>
      page.evaluate(() => document.querySelector("video")?.videoHeight ?? 0);
    expect(await height()).toBe(480);

    const playerBox = await page.locator(".video-js").boundingBox();
    if (playerBox === null) throw new Error("player is not visible");
    await page.mouse.move(
      playerBox.x + playerBox.width * 0.75,
      playerBox.y + playerBox.height / 2,
      { steps: 3 },
    );
    await expect(page.locator(".video-js")).toHaveClass(/vjs-user-active/);
    // 操作バーは再生を始めるまで出ない（video.js の vjs-has-started）ので、再生を始めて
    // ポインターで操作バーを出してから確かめる。
    await expect(page.getByRole("button", { name: "Converting to 480p" })).toBeVisible();
    const seekBar = page.locator(".vjs-progress-control");
    const box = await seekBar.boundingBox();
    if (box === null) throw new Error("seek bar is not visible");
    const seekRequestPromise = page.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`),
    );
    await page.mouse.click(box.x + box.width * 0.9, box.y + box.height / 2);
    const seekRequest = await seekRequestPromise;
    const query = new URL(seekRequest.url()).searchParams;
    expect(Number(query.get("startMs"))).toBeGreaterThan(15_000);
    expect(query.get("quality")).toBe("480p");
    await expect.poll(async () => displayedSeconds(page)).toBeGreaterThanOrEqual(15);
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && element.readyState >= 2;
    });
    expect(await height()).toBe(480);
    expect(
      requests.filter((candidate) => candidate.url().includes("/stream")),
    ).toHaveLength(0);
  });

  test("再生中に操作バーの画質で480pを選ぶと、同じ位置から480pで続く（所有者・ゲスト）", async ({
    page,
    browser,
    request,
  }) => {
    // specs/027-playback-quality 受け入れ条件 2・4・8。
    test.setTimeout(60_000);
    const item = video("hd-1080p");
    expect(item.playable).toBe(true);
    await switchTo480p(page, item);

    const published = await request.put("/api/video-visibility", {
      headers: mutationHeaders,
      data: { videoIds: [item.id], public: true },
    });
    expect(published.status()).toBe(200);
    const guest = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    try {
      await switchTo480p(await guest.newPage(), item);
    } finally {
      await guest.close();
      const restored = await request.put("/api/video-visibility", {
        headers: mutationHeaders,
        data: { videoIds: [item.id], public: false },
      });
      expect(restored.status()).toBe(200);
    }
  });

  test("途切れの警告は左上に出て再生と操作を止めず、閉じると同じ動画では出ない（360・768・1280px・全画面）", async ({
    page,
  }) => {
    // specs/027-playback-quality 受け入れ条件 9〜11、ui-design.md「Stall warning」。
    // 回線の遅さはテストで再現できないので、再生中の要素にデータ待ちと再開の出来事を送る。
    test.setTimeout(90_000);
    const item = video("hd-1080p");
    const warning = page.locator("[data-stall-warning]");
    const stallThreeTimes = () =>
      page.evaluate(() => {
        const element = document.querySelector("video");
        if (element === null) throw new Error("video がありません");
        for (let i = 0; i < 3; i += 1) {
          element.dispatchEvent(new Event("waiting"));
          element.dispatchEvent(new Event("playing"));
        }
      });
    for (const width of [360, 768, 1280]) {
      await page.setViewportSize({ width, height: 800 });
      await play(page, item);
      await expect(warning).toHaveCount(0);
      await stallThreeTimes();
      await expect(
        warning.getByRole("status").filter({
          hasText: "Slow connection is interrupting playback",
        }),
      ).toBeVisible();
      // 左上に出て、操作バーに重ならない。
      const frame = await page.locator("[data-player-frame]").boundingBox();
      const box = await warning.getByRole("status").boundingBox();
      if (frame === null || box === null) throw new Error("枠か警告が見えません");
      const inset = width >= 640 ? 12 : 8;
      expect(Math.round(box.x - frame.x)).toBe(inset);
      expect(Math.round(box.y - frame.y)).toBe(inset);
      await page.locator(".video-js").hover();
      const bar = await page.locator(".vjs-control-bar").boundingBox();
      if (bar === null) throw new Error("操作バーが見えません");
      expect(box.y + box.height).toBeLessThan(bar.y);
      // 画質は変わらず、画質を変える操作も無い。
      await expect(warning.getByRole("button")).toHaveCount(1);
      await expect(page.locator(".vv-quality-value")).toHaveText("1080p");
      expect(await page.evaluate(() => document.querySelector("video")?.paused)).toBe(
        false,
      );
      if (screenshotDir !== undefined) {
        await mkdir(screenshotDir, { recursive: true });
        await page.screenshot({
          path: path.join(screenshotDir, `20260929-stall-warning-${String(width)}.png`),
        });
      }
      if (width === 1280) {
        await page.locator(".vjs-control-bar .vjs-fullscreen-control").click();
        await expect
          .poll(() => page.evaluate(() => document.fullscreenElement !== null))
          .toBe(true);
        await expect(warning.getByRole("status")).toBeVisible();
        const full = await warning.getByRole("status").boundingBox();
        expect(Math.round(full?.x ?? -1)).toBe(12);
        expect(Math.round(full?.y ?? -1)).toBe(12);
        if (screenshotDir !== undefined) {
          await page.screenshot({
            path: path.join(screenshotDir, "20260929-stall-warning-fullscreen.png"),
          });
        }
        await page.keyboard.press("f");
        await expect
          .poll(() => page.evaluate(() => document.fullscreenElement === null))
          .toBe(true);
      }
      // 操作バーの再生はそのまま押せる。
      await page.locator(".video-js").hover();
      await page.locator(".vjs-control-bar > .vjs-play-control").click();
      await expect
        .poll(() => page.evaluate(() => document.querySelector("video")?.paused))
        .toBe(true);
      // 閉じると消え、同じ動画では再び条件を満たしても出ない。
      await warning.getByRole("button", { name: "Dismiss" }).click();
      await expect(warning).toHaveCount(0);
      await page.locator(".vjs-control-bar > .vjs-play-control").click();
      await page.waitForFunction(() => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.readyState >= 2;
      });
      await stallThreeTimes();
      await page.waitForTimeout(300);
      await expect(warning).toHaveCount(0);
    }
  });

  test("コピーで始めた変換は直前のキーフレームの時刻を表示し、再読み込みで同じ場面から再開する", async ({
    page,
  }) => {
    test.setTimeout(45_000);
    const item = video("sparse-keyframes");
    expect(item.playable).toBe(false);
    await play(page, item);

    // 14 秒付近（緑の区間、直前のキーフレームは 8 秒）へシークする。
    const playerBox = await page.locator(".video-js").boundingBox();
    if (playerBox === null) throw new Error("player is not visible");
    await page.mouse.move(
      playerBox.x + playerBox.width * 0.6,
      playerBox.y + playerBox.height / 2,
      { steps: 3 },
    );
    const seekBar = page.locator(".vjs-progress-control");
    const box = await seekBar.boundingBox();
    if (box === null) throw new Error("seek bar is not visible");
    const seekRequestPromise = page.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`),
    );
    await page.mouse.click(box.x + box.width * (14 / 24), box.y + box.height / 2);
    const seekRequest = await seekRequestPromise;
    const seekStart = Number(new URL(seekRequest.url()).searchParams.get("startMs"));
    expect(seekStart).toBeGreaterThan(11_000);
    expect(seekStart).toBeLessThan(16_000);
    const actualStart = await reportedStart(page, seekRequest);
    expect(actualStart).toBeGreaterThan(7_900);
    expect(actualStart).toBeLessThan(8_100);

    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.readyState >= 2;
    });
    const { saved, lastSaved } = await pauseAndRecordProgress(page, item);
    // 表示も保存も、映っている内容の時刻（キーフレーム + 再生した分）になる。
    const elementSeconds = await page
      .locator("video")
      .evaluate((element) => (element as HTMLVideoElement).currentTime);
    expect(saved).toBeGreaterThanOrEqual(actualStart);
    expect(saved).toBeLessThan(seekStart);
    expect(Math.abs(saved - (actualStart + elementSeconds * 1000))).toBeLessThan(1000);
    expect(Math.abs((await displayedSeconds(page)) - saved / 1000)).toBeLessThanOrEqual(
      1,
    );
    const paused = await frameColor(page);
    expect(paused[1]).toBeGreaterThan(paused[0] ?? 255);
    expect(paused[1]).toBeGreaterThan(paused[2] ?? 255);

    // 再読み込みすると保存した位置から始まり、同じキーフレームからの同じ場面が映る。
    const { resumed, startMs } = await reloadAndWaitForTranscode(page, item);
    expect(startMs).toBe(lastSaved());
    expect(Math.abs(startMs - saved)).toBeLessThan(1000);
    expect(await reportedStart(page, resumed)).toBe(actualStart);
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.readyState >= 2;
    });
    const color = await frameColor(page);
    expect(color[1]).toBeGreaterThan(color[0] ?? 255);
    expect(color[1]).toBeGreaterThan(color[2] ?? 255);
    const shown = await displayedSeconds(page);
    expect(shown).toBeGreaterThanOrEqual(Math.floor(actualStart / 1000));
    expect(shown).toBeLessThan(seekStart / 1000);
  });

  test("ライブ変換の字幕は実際の開始位置の offset で取り直し、元動画の時刻に出る", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);
    const item = video("sparse-keyframes");
    expect(item.playable).toBe(false);
    await saveProgress(request, item, 0);
    // ラベルの無い字幕をオンにしておく。
    await page.addInitScript(() => {
      window.localStorage.setItem(
        "vv.subtitles.v1",
        JSON.stringify({ enabled: true, label: "" }),
      );
    });
    // 表示に出た字幕の文字をすべて記録する（6〜7 秒の cue が一瞬でも出ないことを見る）。
    await page.addInitScript(() => {
      const seen: string[] = [];
      (window as unknown as { vvSeenCues: string[] }).vvSeenCues = seen;
      new MutationObserver(() => {
        const text = document.querySelector(".vjs-text-track-display")?.textContent;
        if (text) seen.push(text);
      }).observe(document, { subtree: true, childList: true, characterData: true });
    });
    const seenCues = () =>
      page.evaluate(() => (window as unknown as { vvSeenCues: string[] }).vvSeenCues);
    const subtitleRequests: { url: string; afterReport: boolean }[] = [];
    let reported = false;
    page.on("response", (response) => {
      if (response.url().includes("/transcode-start?")) reported = true;
    });
    page.on("request", (candidate) => {
      if (candidate.url().includes(`/api/videos/${String(item.id)}/subtitles/`)) {
        subtitleRequests.push({ url: candidate.url(), afterReport: reported });
      }
    });
    await play(page, item);

    const playerBox = await page.locator(".video-js").boundingBox();
    if (playerBox === null) throw new Error("player is not visible");
    await page.mouse.move(
      playerBox.x + playerBox.width * 0.6,
      playerBox.y + playerBox.height / 2,
      { steps: 3 },
    );
    const seekBar = page.locator(".vjs-progress-control");
    const box = await seekBar.boundingBox();
    if (box === null) throw new Error("seek bar is not visible");
    const seekRequestPromise = page.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`),
    );
    const shiftedPromise = page.waitForRequest((candidate) =>
      /\/subtitles\/sparse-keyframes\.srt\?offsetMs=\d+$/.test(candidate.url()),
    );
    await page.mouse.click(box.x + box.width * (14 / 24), box.y + box.height / 2);
    const seekRequest = await seekRequestPromise;
    const actualStart = await reportedStart(page, seekRequest);
    expect(actualStart).toBeGreaterThan(7_900);
    expect(actualStart).toBeLessThan(8_100);
    const shifted = await shiftedPromise;
    expect(new URL(shifted.url()).searchParams.get("offsetMs")).toBe(String(actualStart));
    expect(
      subtitleRequests.find((entry) => entry.url === shifted.url())?.afterReport,
    ).toBe(true);

    // 9〜10 秒の cue が、表示の 9〜10 秒台に出る。
    const display = page.locator(".vjs-text-track-display");
    await expect(display).toContainText("After keyframe cue", { timeout: 10_000 });
    const shownAt = await displayedSeconds(page);
    expect(shownAt).toBeGreaterThanOrEqual(9);
    expect(shownAt).toBeLessThanOrEqual(10);
    expect((await seenCues()).join(" ")).not.toContain("Before keyframe cue");

    // 再読み込みで再開位置から始めても、同じ offset で取り直す。この動画は 24 秒で、
    // 残り 15 秒以内（9 秒以降）の位置は視聴済みとして保存され、開き直すと先頭から
    // 始まる（internal/domain/progress.go の CompletionTailMs）。上で 9〜10 秒の cue を
    // 見たので、止めたときと離れるときの保存はここで止め、再開位置は 9 秒より前に置く
    // （止めたときの保存からの再開は、前の試験が確かめる）。
    const progressUrl = `**/api/videos/${String(item.id)}/progress`;
    await page.route(progressUrl, (route) => route.abort());
    await page.locator(".vjs-play-control").click();
    await page.waitForFunction(() => document.querySelector("video")?.paused === true);
    const resumeAt = actualStart + 500;
    await saveProgress(request, item, resumeAt);
    const resumedSubtitle = page.waitForRequest((candidate) =>
      candidate
        .url()
        .endsWith(`/subtitles/sparse-keyframes.srt?offsetMs=${String(actualStart)}`),
    );
    reported = false;
    const { resumed, startMs } = await reloadAndWaitForTranscode(page, item);
    await page.unroute(progressUrl);
    expect(startMs).toBe(resumeAt);
    expect(await reportedStart(page, resumed)).toBe(actualStart);
    await resumedSubtitle;
    await page.locator(".vjs-big-play-button").click();
    await expect(display).toContainText("After keyframe cue", { timeout: 10_000 });
    const resumedAt = await displayedSeconds(page);
    expect(resumedAt).toBeGreaterThanOrEqual(9);
    expect(resumedAt).toBeLessThanOrEqual(10);
    expect((await seenCues()).join(" ")).not.toContain("Before keyframe cue");
  });

  test("離脱とreloadは自分の変換だけを止め、別tabの再生を継続する", async ({
    browser,
  }) => {
    test.setTimeout(60_000);
    const context = await browser.newContext();
    const first = await context.newPage();
    const second = await context.newPage();
    await Promise.all([first.goto("/"), second.goto("/")]);
    await throttle(first, 32 * 1024);
    await throttle(second, 32 * 1024);
    const item = video("long-gop");

    const firstRequestPromise = first.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4`),
    );
    const secondRequestPromise = second.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4`),
    );
    await Promise.all([
      first.goto(`/videos/${String(item.id)}`),
      second.goto(`/videos/${String(item.id)}`),
    ]);
    await firstRequestPromise;
    const secondRequest = await secondRequestPromise;
    await Promise.all([
      first.locator(".vjs-big-play-button").click(),
      second.locator(".vjs-big-play-button").click(),
    ]);
    await Promise.all([
      first.waitForFunction(() => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      }),
      second.waitForFunction(() => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      }),
    ]);

    await settleResources(first);
    const leaveStarted = Date.now();
    await first.goto("/");
    expect(Date.now() - leaveStarted).toBeLessThan(5000);
    await expect(first.locator("video")).toHaveCount(0);
    const before = await second
      .locator("video")
      .evaluate((element) => (element as HTMLVideoElement).currentTime);
    await second.waitForFunction((time) => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime > time + 0.2;
    }, before);

    const restarted = second.waitForRequest(
      (candidate) =>
        candidate !== secondRequest &&
        candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4`),
    );
    const reloadStarted = Date.now();
    await second.reload();
    await restarted;
    // reload は認証のゲートが GET /api/auth/session を待ってから描くので、その1往復と
    // ゲートのモジュールの分だけ長い。メンテナーの判断で上限を 8000ms にした（PR 335）。
    expect(Date.now() - reloadStarted).toBeLessThan(8000);
    await context.close();
  });

  test("360px、768px、1280pxでシークpreviewとkeyboard操作が重ならない", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct");
    for (const width of [360, 768, 1280]) {
      await page.setViewportSize({ width, height: 800 });
      await page.goto(`/videos/${String(item.id)}`);
      await expect(page.locator(".video-js")).toBeVisible();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(width);
      const bounds = await page.locator(".video-js").boundingBox();
      expect(bounds?.x ?? -1).toBeGreaterThanOrEqual(0);
      expect((bounds?.x ?? width) + (bounds?.width ?? width + 1)).toBeLessThanOrEqual(
        width,
      );

      await page.locator(".vjs-big-play-button").click();
      await page.waitForFunction(() => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      });
      const seekBar = page.locator(".vjs-progress-holder");
      const seekControl = page.locator(".vjs-progress-control");
      const seekBounds = await seekBar.boundingBox();
      const controlBounds = await seekControl.boundingBox();
      const playerBounds = await page.locator(".video-js").boundingBox();
      if (seekBounds === null || controlBounds === null || playerBounds === null) {
        throw new Error("player controls are not visible");
      }
      // シートは取り付けの間 1 回だけ取得し、ポインターを動かしても取り直さない
      // （specs/021-seek-thumbnail-sprite/research.md R-5）。
      const sheetRequests: string[] = [];
      const onRequest = (request: Request) => {
        if (/\/seek-thumbnail\/\d+/.test(new URL(request.url()).pathname)) {
          sheetRequests.push(request.url());
        }
      };
      page.on("request", onRequest);
      await page.mouse.move(seekBounds.x + seekBounds.width * 0.25, controlBounds.y + 2);
      await expect(page.locator('.vv-seek-preview[data-state="ready"]')).toBeVisible({
        timeout: 5000,
      });
      for (const ratio of [0.01, 0.2, 0.5, 0.8, 0.99]) {
        await page.mouse.move(
          seekBounds.x + seekBounds.width * ratio,
          seekBounds.y + seekBounds.height / 2,
          { steps: 5 },
        );
        const preview = page.locator('.vv-seek-preview[data-state="ready"]');
        await expect(preview).toBeVisible({ timeout: 5000 });
        const previewBounds = await preview.boundingBox();
        expect(previewBounds?.x ?? -1).toBeGreaterThanOrEqual(playerBounds.x);
        expect(
          (previewBounds?.x ?? width) + (previewBounds?.width ?? width + 1),
        ).toBeLessThanOrEqual(playerBounds.x + playerBounds.width);
      }
      page.off("request", onRequest);
      // 6 秒の fixture は 1 シートに収まる。
      expect(sheetRequests).toHaveLength(1);
      if (screenshotDir !== undefined) {
        await mkdir(screenshotDir, { recursive: true });
        await page.mouse.move(
          seekBounds.x + seekBounds.width * 0.5,
          seekBounds.y + seekBounds.height / 2,
        );
        await expect(page.locator('.vv-seek-preview[data-state="ready"]')).toBeVisible();
        await page.screenshot({
          path: path.join(screenshotDir, `20260922-seek-thumbnail-${String(width)}.png`),
          fullPage: true,
        });
      }

      await page.locator(".video-js").focus();
      await page.locator(".video-js").press("Space");
      await page.locator(".video-js").press("Space");
      const beforeSeek = await page
        .locator("video")
        .evaluate((element) => (element as HTMLVideoElement).currentTime);
      const progressHolder = page.locator(".vjs-progress-holder");
      await progressHolder.focus();
      await progressHolder.press("ArrowRight");
      await expect
        .poll(() =>
          page
            .locator("video")
            .evaluate((element) => (element as HTMLVideoElement).currentTime),
        )
        .toBeGreaterThan(beforeSeek);
      // 見出しの帯の右端の閉じる × で戻る。× は帯の 1 か所だけにある。
      const back = page.getByRole("button", { name: "Close" });
      await back.focus();
      await Promise.all([page.waitForURL("/"), back.press("Enter")]);
    }
  });

  test("シーク画像を取得できなくても時刻と操作を維持する", async ({ page }) => {
    const item = video("direct");
    await page.setViewportSize({ width: 360, height: 800 });
    await play(page, item);
    await page.route("**/seek-thumbnail?*", (route) => route.abort("failed"));
    const seekBar = page.locator(".vjs-progress-holder");
    const seekBounds = await seekBar.boundingBox();
    if (seekBounds === null) throw new Error("seek bar is not visible");
    await page.mouse.move(
      seekBounds.x + seekBounds.width * 0.75,
      seekBounds.y + seekBounds.height / 2,
    );
    await expect(
      page.locator('.vv-seek-preview[data-state="unavailable"]'),
    ).toBeVisible();
    if (screenshotDir !== undefined) {
      await page.screenshot({
        path: path.join(screenshotDir, "20260922-seek-thumbnail-unavailable-360.png"),
        fullPage: true,
      });
    }
  });

  test("直接配信とライブ変換で、同じ位置に同じコマを出す", async ({ page }) => {
    test.setTimeout(30_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    const shown: Array<{ position: string; pixels: Buffer }> = [];
    // 同じ内容を MP4（直接配信）と MKV（ライブ変換）にした fixture で、コマを決める
    // 位置は元動画の論理時刻である（contracts/seek-sprite-api.md §5）。
    for (const item of [video("direct"), video("container-only")]) {
      await play(page, item);
      await page.addStyleTag({
        content: ".vv-seek-preview-time { visibility: hidden; }",
      });
      const seekBar = page.locator(".vjs-progress-holder");
      const seekBounds = await seekBar.boundingBox();
      if (seekBounds === null) throw new Error("seek bar is not visible");
      await page.mouse.move(
        seekBounds.x + seekBounds.width * 0.9,
        seekBounds.y + seekBounds.height / 2,
      );
      const preview = page.locator('.vv-seek-preview[data-state="ready"]');
      await expect(preview).toBeVisible({ timeout: 5000 });
      await expect(preview).toContainText("0:05");
      const image = preview.locator(".vv-seek-preview-image");
      shown.push({
        position: await image.evaluate(
          (element) => getComputedStyle(element).backgroundPosition,
        ),
        pixels: await image.screenshot({ animations: "disabled" }),
      });
    }
    expect(shown[0]?.position).toBe(shown[1]?.position);
    expect(shown[0]?.position).not.toBe("0% 0%");
    expect(shown[0]?.pixels.equals(shown[1]?.pixels ?? Buffer.alloc(0))).toBe(true);
  });

  test("縦動画のシークpreviewも実JPEGを表示する", async ({ page }) => {
    const item = video("portrait");
    await page.setViewportSize({ width: 768, height: 800 });
    await play(page, item);

    const seekBar = page.locator(".vjs-progress-holder");
    const seekBounds = await seekBar.boundingBox();
    if (seekBounds === null) throw new Error("player controls are not visible");
    await page.mouse.move(
      seekBounds.x + seekBounds.width * 0.5,
      seekBounds.y + seekBounds.height / 2,
    );

    const preview = page.locator('.vv-seek-preview[data-state="ready"]');
    await expect(preview).toBeVisible({ timeout: 5000 });
    const previewBounds = await preview.boundingBox();
    if (previewBounds === null) throw new Error("seek preview is not visible");
    // 画像は動画の縦横比を保つ（specs/009-seek-thumbnail-preview/ui-design.md）。
    // fixture の portrait.mp4 は 180x320 なので 9:16 になる。
    expect(previewBounds.width / previewBounds.height).toBeCloseTo(9 / 16, 1);
    // シートは object URL で持ち、背景として 1 コマの箱に敷く。
    const image = preview.locator(".vv-seek-preview-image");
    await expect(image).toHaveCSS("background-image", /url\("blob:/);
    await expect(image).toHaveCSS("background-size", /.+/);
    if (screenshotDir !== undefined) {
      await mkdir(screenshotDir, { recursive: true });
      await page.screenshot({
        path: path.join(screenshotDir, "20260922-seek-thumbnail-portrait-768.png"),
        fullPage: true,
      });
    }
  });

  test("画面全体のキー操作が効き、操作バーの再生速度が再生に反映される", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct-fallback");
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/videos/${String(item.id)}`);
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime > 1.5;
    });

    // 関連動画のリンクにフォーカスがあっても、画面全体のキー操作が効く。
    await page.locator("aside").getByRole("link").first().focus();
    await page.keyboard.press("m");
    await expect
      .poll(() =>
        page.locator("video").evaluate((element) => (element as HTMLVideoElement).muted),
      )
      .toBe(true);
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.keyboard.press("Space");
    await expect
      .poll(() =>
        page.locator("video").evaluate((element) => (element as HTMLVideoElement).paused),
      )
      .toBe(true);
    await page.keyboard.press("0");
    await expect
      .poll(() =>
        page
          .locator("video")
          .evaluate((element) => (element as HTMLVideoElement).currentTime),
      )
      .toBeLessThan(1);

    await page.keyboard.press("Space");
    await page.locator(".video-js").hover();
    await page.locator(".vjs-control-bar > .vjs-playback-rate").hover();
    await page.getByRole("menuitemradio", { name: /^1\.5x/ }).click();
    await expect
      .poll(() =>
        page
          .locator("video")
          .evaluate((element) => (element as HTMLVideoElement).playbackRate),
      )
      .toBe(1.5);
    const rateStart = await page
      .locator("video")
      .evaluate((element) => (element as HTMLVideoElement).currentTime);
    await page.waitForTimeout(1000);
    const rateEnd = await page
      .locator("video")
      .evaluate((element) => (element as HTMLVideoElement).currentTime);
    expect(rateEnd - rateStart).toBeGreaterThan(1.2);
    expect(
      await page
        .locator("video")
        .evaluate((element) => (element as HTMLVideoElement).paused),
    ).toBe(false);
    await expect(page.locator(".vjs-remaining-time")).toHaveCount(0);
    await expect(page.locator(".vjs-current-time")).toBeVisible();
    await expect(page.locator(".vjs-duration")).toBeVisible();
    if (screenshotDir !== undefined) {
      await mkdir(screenshotDir, { recursive: true });
      await page.locator(".video-js").hover();
      await page.screenshot({
        path: path.join(screenshotDir, "20260923-video-detail-1280.png"),
        fullPage: true,
      });
    }
  });

  test("隣の字幕をメニューで選ぶと cue の時刻に出て、「オフ」で消え、選択を再読み込みの後も覚える", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct");
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/videos/${String(item.id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");

    // 字幕ボタンは再生速度の前に出る。一度も選んでいないのでオフで始まる。
    const button = page.locator(".vjs-control-bar > .vjs-subs-caps-button");
    await expect(button).toBeVisible();
    await expect(button.locator("button")).toHaveAttribute("title", "Subtitles (C)");
    await expect(button.locator("button")).toHaveAttribute("aria-keyshortcuts", "C");
    await expect(
      page.locator(".vjs-control-bar > .vjs-subs-caps-button + .vjs-playback-rate"),
    ).toHaveCount(1);
    const items = button.locator(".vjs-menu-item .vjs-menu-item-text");
    await expect(items).toHaveText(["Off", "Default", "ja"]);
    await expect(button.locator(".vjs-texttrack-settings")).toHaveCount(0);
    const display = page.locator(".vjs-text-track-display");
    await expect(display).not.toContainText("Default subtitle cue");

    // cue は 0.5 秒から。再生して 0.5 秒を過ぎると、選んだ字幕の文字が出る。
    await page.evaluate(() => {
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      if (element === null) throw new Error("video is missing");
      element.muted = true;
      void element.play();
    });
    await page.locator(".video-js").hover();
    await button.hover();
    await page.getByRole("menuitemradio", { name: /^Default/ }).click();
    await expect(display).toContainText("Default subtitle cue");

    await button.hover();
    await page.getByRole("menuitemradio", { name: /^Off/ }).click();
    await expect(display).not.toContainText("Default subtitle cue");

    await button.hover();
    await page.getByRole("menuitemradio", { name: /^ja/ }).click();
    await expect(display).toContainText("日本語の字幕");

    // 再読み込みしても ja がオンのまま始まる。
    await page.reload();
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");
    await expect(
      button.locator(".vjs-menu-item", { hasText: /^ja/ }).first(),
    ).toHaveAttribute("aria-checked", "true");
    await page.evaluate(() => {
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      if (element === null) throw new Error("video is missing");
      element.muted = true;
      void element.play();
    });
    await expect(display).toContainText("日本語の字幕");

    // c キーでオフにし、もう一度で最後に選んだ ja に戻る。
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.keyboard.press("c");
    await expect(display).not.toContainText("日本語の字幕");
    await page.keyboard.press("c");
    await expect(display).toContainText("日本語の字幕");
  });

  test("関連動画のサムネイルの帯で、シークバーの吹き出しと同じコマを出し、押すとその動画へ移る（768・1280px）", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    // 帯の対象は関連動画の direct、開いておく動画は同じフォルダの container-only
    // （specs/032-card-scrub-preview、親 Issue #616 の要件 8・受け入れ条件 3）。
    const target = video("direct");
    const durationMs = target.durationMs ?? 0;
    if (target.seekThumbnailUrl === undefined) throw new Error("no sprite");
    const sprite = (await (await request.get(target.seekThumbnailUrl)).json()) as {
      intervalMs: number;
      frameCount: number;
    };
    expect(sprite.frameCount).toBeGreaterThan(1);
    const sample = Math.floor(sprite.frameCount / 2);
    // コマが受け持つ範囲の真ん中。末尾のコマの範囲は動画の長さで切れる（6 秒の動画を
    // 5 秒おきに 2 コマにすると 2 コマ目は 5〜6 秒）ので、終わりを長さで抑える。
    const sampleStart = sample * sprite.intervalMs;
    const sampleEnd = Math.min((sample + 1) * sprite.intervalMs, durationMs);
    const ratio = (sampleStart + sampleEnd) / 2 / durationMs;

    // プレイヤーのシークバーの吹き出しが、同じ割合の位置で出す背景の位置。
    await page.setViewportSize({ width: 1280, height: 800 });
    await play(page, target);
    const seekBar = await page.locator(".vjs-progress-holder").boundingBox();
    if (seekBar === null) throw new Error("seek bar is not visible");
    await page.mouse.move(
      seekBar.x + seekBar.width * ratio,
      seekBar.y + seekBar.height / 2,
    );
    const bubble = page.locator('.vv-seek-preview[data-state="ready"]');
    await expect(bubble).toBeVisible({ timeout: 5000 });
    const playerPosition = await bubble
      .locator(".vv-seek-preview-image")
      .evaluate((element) => getComputedStyle(element).backgroundPosition);

    const targetPath = `/api/videos/${String(target.id)}/seek-thumbnail`;
    for (const width of [768, 1280]) {
      await test.step(String(width), async () => {
        await page.setViewportSize({ width, height: 800 });
        // 開いた動画のプレイヤーも自分のスプライトを取るので、帯の対象の要求だけを数える。
        const requests: string[] = [];
        const onRequest = (sent: Request) => {
          if (new URL(sent.url()).pathname.startsWith(targetPath))
            requests.push(sent.url());
        };
        page.on("request", onRequest);
        await page.goto(`/videos/${String(video("container-only").id)}`);
        const link = page.locator("aside").getByRole("link", { name: /^direct \d/ });
        await expect(link).toBeVisible({ timeout: 10_000 });
        await link.scrollIntoViewIfNeeded();
        const band = link.locator("[data-scrub-band]");
        const face = link.locator("[data-scrub-band] >> xpath=..");
        const faceBox = await face.boundingBox();
        if (faceBox === null) throw new Error("thumbnail is not visible");
        const layout = () =>
          link.evaluate((element) => {
            const rect = element.getBoundingClientRect();
            const column = element.closest("aside")?.getBoundingClientRect();
            return [rect.x, rect.y, rect.width, rect.height, column?.width ?? 0];
          });

        // 帯に入らずにサムネイルを横と縦に通り過ぎるだけでは取得しない。
        await page.mouse.move(faceBox.x - 20, faceBox.y + faceBox.height * 0.3);
        await page.mouse.move(
          faceBox.x + faceBox.width + 20,
          faceBox.y + faceBox.height * 0.3,
          { steps: 10 },
        );
        await page.mouse.move(faceBox.x + faceBox.width / 2, faceBox.y - 10);
        await page.mouse.move(
          faceBox.x + faceBox.width / 2,
          faceBox.y + faceBox.height * 0.6,
          { steps: 10 },
        );
        await page.waitForTimeout(300);
        expect(requests).toHaveLength(0);
        const before = await layout();

        // 帯の同じ割合の位置で、吹き出しと同じシート・同じ背景の位置になる（受け入れ条件 3）。
        const bandBox = await band.boundingBox();
        if (bandBox === null) throw new Error("band is not visible");
        const point = {
          x: Math.min(bandBox.x + bandBox.width * ratio, bandBox.x + bandBox.width - 0.5),
          y: bandBox.y + bandBox.height / 2,
        };
        await page.mouse.move(point.x, point.y);
        const image = link.locator("[data-scrub-frame-image]");
        await expect(image).toBeVisible({ timeout: 5000 });
        expect(
          await image.evaluate((element) => getComputedStyle(element).backgroundPosition),
        ).toBe(playerPosition);
        expect(
          await image.evaluate((element) => getComputedStyle(element).backgroundImage),
        ).toMatch(/^url\("blob:/);
        await expect(link.locator("[data-scrub-bar]")).toBeVisible();
        await expect(link.getByText(/^\d+:\d\d \/ \d+:\d\d$/)).toBeVisible();
        // 帯の出入りで行の高さと列の幅が変わらない。
        expect(await layout()).toEqual(before);
        expect(requests.filter((url) => /\/seek-thumbnail\/\d+/.test(url))).toHaveLength(
          1,
        );
        if (screenshotDir !== undefined) {
          await mkdir(screenshotDir, { recursive: true });
          await page.screenshot({
            path: path.join(screenshotDir, `20261001-related-scrub-${String(width)}.png`),
            fullPage: false,
          });
        }
        page.off("request", onRequest);

        // 帯の上のクリックでその動画へ移る。
        await page.mouse.click(point.x, point.y);
        await expect(page).toHaveURL(new RegExp(`/videos/${String(target.id)}$`));
        await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");
      });
    }
  });

  test("関連動画から移ったあとの × と Esc は最初の一覧へ戻る", async ({ page }) => {
    test.setTimeout(30_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?q=direct");
    const card = page.getByRole("link", { name: "direct", exact: true });
    await card.click();
    await expect(page).toHaveURL(new RegExp(`/videos/${String(video("direct").id)}$`));
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");
    await expect(
      page.getByRole("heading", { level: 2, name: "Related videos" }),
    ).toBeVisible();

    const related = page.locator("aside").getByRole("link").first();
    const relatedTitle = (await related.textContent()) ?? "";
    await related.click();
    await expect(page).not.toHaveURL(
      new RegExp(`/videos/${String(video("direct").id)}$`),
    );
    await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("direct");
    expect(relatedTitle).toContain(
      (await page.getByRole("heading", { level: 1 }).textContent()) ?? "missing",
    );
    await page.getByRole("button", { name: "Close" }).click();
    await expect(page).toHaveURL(/\/\?q=direct$/);

    await page.goBack();
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(/\/\?q=direct$/);
  });

  test("操作バーの読み上げ名とツールチップが英語でキーを添え、失敗の説明も英語で出す", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct");
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/videos/${String(item.id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");

    // video.js の独自言語はカタログの英語から作り、キーボード操作を持つボタンにキーを添える。
    await expect(page.locator(".video-js")).toHaveAttribute("lang", "en-x-vv");
    const controls: [string, string, string][] = [
      [".vjs-control-bar > .vjs-play-control", "Play (Space)", "Space"],
      [".vjs-control-bar .vjs-mute-control", "Mute (M)", "M"],
      [".vjs-control-bar > .vjs-fullscreen-control", "Fullscreen (F)", "F"],
      [".vjs-control-bar .vv-bar-button", "Restart (0)", "0"],
    ];
    for (const [selector, title, keys] of controls) {
      const control = page.locator(selector);
      await expect(control).toHaveAttribute("title", title);
      await expect(control).toHaveAttribute("aria-keyshortcuts", keys);
    }
    await expect(page.locator(".vjs-control-bar .vv-bar-button")).toHaveAttribute(
      "aria-label",
      "Restart",
    );
    await expect(
      page.locator(".vjs-control-bar > .vjs-play-control .vjs-control-text"),
    ).toHaveText("Play (Space)");

    // キーボードショートカットは変わらない（M でミュートし、ボタンの名前が切り替わる）。
    await page.keyboard.press("m");
    await expect(page.locator(".vjs-control-bar .vjs-mute-control")).toHaveAttribute(
      "title",
      "Unmute (M)",
    );
    await page.keyboard.press("m");
    await expect(page.locator(".vjs-control-bar .vjs-mute-control")).toHaveAttribute(
      "title",
      "Mute (M)",
    );

    // 開く要求がファイルの無い理由で失敗したら、その理由を英語で出す。
    await page.route(`**/api/videos/${String(item.id)}/open`, (route) =>
      route.fulfill({
        status: 409,
        json: { code: "file_missing", message: "The video file is missing." },
      }),
    );
    await page.route(`**/api/videos/${String(item.id)}`, async (route) => {
      const response = await route.fetch();
      const body = (await response.json()) as Record<string, unknown>;
      await route.fulfill({
        response,
        json: {
          ...body,
          location: { ...(body.location as object), openable: true },
        },
      });
    });
    await page.reload();
    await page.getByRole("button", { name: "Open file" }).click();
    await expect(page.getByRole("alert")).toHaveText(
      "Couldn't open the file: The video file is missing.",
    );

    // 英語化の前の読み取り失敗（コードが無く、理由が日本語）は、理由を出さずに英語の概要を出す。
    await page.unroute(`**/api/videos/${String(item.id)}`);
    await page.route(`**/api/videos/${String(item.id)}`, async (route) => {
      const response = await route.fetch();
      const body = (await response.json()) as Record<string, unknown>;
      await route.fulfill({
        response,
        json: {
          ...body,
          probeState: "failed",
          playable: false,
          probeError: "ffprobe の実行に失敗しました: 壊れています",
          probeErrorCode: undefined,
        },
      });
    });
    await page.reload();
    const failure = page.locator("[data-player-frame]").getByRole("alert");
    await expect(failure).toContainText("Couldn't read this video");
    await expect(failure).toContainText("Couldn't read this video's information.");
    await expect(failure).not.toContainText("壊れています");
  });
});

import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
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
  container?: string;
  videoCodec?: string;
  audioCodec?: string;
}

interface MediaFolder {
  id: number;
  version: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
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

/** Wait for initial media loading and fail with the alert and stream response when it fails; otherwise the hidden play button only produces a timeout. */
async function play(page: Page, item: Video, search = "") {
  const mediaResponses: string[] = [];
  page.on("response", (response) => {
    const url = new URL(response.url());
    if (/^\/api\/videos\/\d+\/(stream|transcode\.mp4)$/.test(url.pathname)) {
      mediaResponses.push(`${String(response.status())} ${url.pathname}${url.search}`);
    }
  });
  await page.goto(`/videos/${String(item.id)}${search}`);
  // Wait for preload metadata before clicking; a late load failure can hide the play button.
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

/** Match the reported actual start to the transcode attempt. */
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

/** Use the latest save after playback pauses, since the first PUT may be a periodic save. lastSaved also includes later navigation saves. */
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

// Keep tests independent so one failure does not skip the rest; beforeAll restores fixtures after worker replacement.
test.describe("live MP4 playback", () => {
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

  test("広い画面では左の列の中身が長くても、ページ全体はスクロールしない（1280・1920px）", async ({
    page,
  }) => {
    // Absolutely positioned descendants must not escape the scrolling column and extend the page.
    for (const viewport of [
      { width: 1280, height: 720 },
      { width: 1920, height: 950 },
    ]) {
      await page.setViewportSize(viewport);
      await page.goto(`/videos/${String(video("direct").id)}`);
      await expect(page.locator('[data-slot="detail-page-main"]')).toBeVisible();
      await expect
        .poll(() =>
          page.evaluate(() => document.documentElement.scrollHeight - window.innerHeight),
        )
        .toBe(0);
    }
  });

  test("タッチ端末は映像をタップして再生・一時停止する", async ({ browser }) => {
    const context = await browser.newContext({
      hasTouch: true,
      viewport: { width: 390, height: 800 },
    });
    const page = await context.newPage();
    try {
      await page.goto(`/videos/${String(video("direct").id)}`);
      expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(
        true,
      );
      await page.waitForFunction(
        () =>
          (document.querySelector<HTMLVideoElement>("video.vjs-tech")?.readyState ?? 0) >=
          1,
      );
      await expect(page.locator(".vjs-big-play-button")).toBeHidden();
      await page.locator("div.vjs-poster").tap();
      await expect
        .poll(() =>
          page
            .locator("video.vjs-tech")
            .evaluate((el) => !(el as HTMLVideoElement).paused),
        )
        .toBe(true);

      await page.locator("video.vjs-tech").tap();
      await expect
        .poll(() =>
          page
            .locator("video.vjs-tech")
            .evaluate((el) => (el as HTMLVideoElement).paused),
        )
        .toBe(true);
      await page.locator("video.vjs-tech").tap();
      await expect
        .poll(() =>
          page
            .locator("video.vjs-tech")
            .evaluate((el) => !(el as HTMLVideoElement).paused),
        )
        .toBe(true);
    } finally {
      await context.close();
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

    // Reveal controls after slow startup: video.js can hide them before playback begins.
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
    // Stream copy starts at the preceding keyframe (20 seconds).
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

    const color = await frameColor(page);
    expect(color[2]).toBeGreaterThan(color[0] ?? 255);
    expect(color[2]).toBeGreaterThan(color[1] ?? 255);

    expect(
      requests.filter((candidate) => candidate.url().includes("/stream")),
    ).toHaveLength(0);
  });

  test("コピーで始めた変換は直前のキーフレームの時刻を表示し、再読み込みで同じ場面から再開する", async ({
    page,
  }) => {
    test.setTimeout(45_000);
    const item = video("sparse-keyframes");
    expect(item.playable).toBe(false);
    await play(page, item);

    // Seek into the green interval near 14 seconds, whose preceding keyframe is at 8 seconds.
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
    // Displayed and saved time must follow the actual content: keyframe time plus elapsed playback.
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

  test("360px、768px、1280pxでプレイヤーが画面の幅に収まり、ページが横にはみ出さない", async ({
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
    }
  });

  test("ファイル、バーチャル、ファイルの順のプレイリストは、連続再生でバーチャルな項目を飛ばして 2 本目に届く", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);
    const first = video("direct");
    const second = video("container-only");
    const created = await request.post("/api/virtual-videos", {
      headers: mutationHeaders,
      data: { title: "e2e virtual between files" },
    });
    expect(created.status()).toBe(201);
    const virtual = (await created.json()) as { id: number; contentVersion: string };
    const made = await request.post("/api/playlists", {
      headers: mutationHeaders,
      data: { name: "e2e virtual skip" },
    });
    expect(made.status()).toBe(201);
    const playlistId = ((await made.json()) as { id: number }).id;
    try {
      const added = await request.post(`/api/playlists/${String(playlistId)}/items`, {
        headers: mutationHeaders,
        data: { videoIds: [first.id, virtual.id, second.id] },
      });
      expect(added.ok()).toBe(true);
      const read = await request.get(`/api/playlists/${String(playlistId)}`);
      const items = ((await read.json()) as { items: { id: number }[] }).items;
      expect(items).toHaveLength(3);
      const [firstItem, , secondItem] = items;

      await page.setViewportSize({ width: 1280, height: 800 });
      await play(
        page,
        first,
        `?playlist=${String(playlistId)}&item=${String(firstItem!.id)}`,
      );
      // The 6-second clip ends by itself; the notice names the second file video, not the virtual one.
      const frame = page.locator("[data-player-frame]");
      await expect(
        frame.getByRole("link", { name: new RegExp(`^${second.title} \\d`) }),
      ).toBeVisible({ timeout: 20_000 });
      // With autoplay on, the notice advances by itself; otherwise the ended layer waits for Play next.
      const playNext = frame.getByRole("button", { name: "Play next" });
      if (await playNext.isVisible()) await playNext.click();
      await expect(page).toHaveURL(
        `/videos/${String(second.id)}?playlist=${String(playlistId)}&item=${String(secondItem!.id)}`,
        { timeout: 15_000 },
      );
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(second.title);
    } finally {
      await request.delete(`/api/playlists/${String(playlistId)}`, {
        headers: mutationHeaders,
      });
      await request.delete(
        `/api/videos/${String(virtual.id)}?contentVersion=${encodeURIComponent(virtual.contentVersion)}`,
        { headers: mutationHeaders },
      );
    }
  });

  test("隣の字幕をメニューで選ぶと、サーバーが変換した字幕を cue の時刻に出す", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct");
    await page.setViewportSize({ width: 1280, height: 800 });
    // Controls appear only after playback starts; subtitle cues begin at 0.5 seconds.
    await play(page, item);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");
    await page.locator(".video-js").hover();

    const button = page.locator(".vjs-control-bar > .vjs-subs-caps-button");
    const display = page.locator(".vjs-text-track-display");

    await button.hover();
    await page.getByRole("menuitemradio", { name: /^Default/ }).click();
    await expect(display).toContainText("Default subtitle cue");

    await button.hover();
    await page.getByRole("menuitemradio", { name: /^ja/ }).click();
    await expect(display).toContainText("日本語の字幕");
  });
});

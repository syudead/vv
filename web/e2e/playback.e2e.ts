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
      { timeout: 30_000 },
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
      { timeout: 30_000 },
    )
    .toBe(9);
}

async function waitForSeekThumbnails(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        let ready = 0;
        for (const item of videos.values()) {
          if (item.seekThumbnailUrl === undefined) continue;
          const separator = item.seekThumbnailUrl.includes("?") ? "&" : "?";
          const response = await request.get(
            `${item.seekThumbnailUrl}${separator}positionMs=0`,
          );
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

async function play(page: Page, item: Video) {
  const started = Date.now();
  await page.goto(`/videos/${String(item.id)}`);
  const button = page.locator(".vjs-big-play-button");
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

function mediaRequests(page: Page): Request[] {
  const requests: Request[] = [];
  page.on("request", (request) => {
    if (/\/api\/videos\/\d+\/(stream|transcode\.mp4)/.test(request.url())) {
      requests.push(request);
    }
  });
  return requests;
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

test.describe.serial("live MP4 playback", () => {
  test.beforeAll(async ({ request }) => {
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

    const color = await page.evaluate(() => {
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
    expect(color[2]).toBeGreaterThan(color[0] ?? 255);
    expect(color[2]).toBeGreaterThan(color[1] ?? 255);

    const progress = page.waitForRequest(
      (candidate) =>
        candidate.url().endsWith(`/api/videos/${String(item.id)}/progress`) &&
        candidate.method() === "PUT",
    );
    await page.locator(".vjs-play-control").click();
    const body = (await progress).postDataJSON() as { positionMs: number };
    expect(body.positionMs).toBeGreaterThanOrEqual(seekStart);
    expect(body.positionMs).toBeLessThan(seekStart + 5000);
    expect(
      requests.filter((candidate) => candidate.url().includes("/stream")),
    ).toHaveLength(0);
  });

  test("離脱とreloadは自分の変換だけを止め、別tabの再生を継続する", async ({
    browser,
  }) => {
    test.setTimeout(30_000);
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

    const leaveStarted = Date.now();
    await first.goto("/");
    expect(Date.now() - leaveStarted).toBeLessThan(5000);
    await expect(first.locator("video")).toHaveCount(0);
    const before = await second
      .locator("video")
      .evaluate((element) => element.currentTime);
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
    expect(Date.now() - reloadStarted).toBeLessThan(5000);
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
      await page.mouse.move(seekBounds.x + seekBounds.width * 0.25, controlBounds.y + 2);
      await expect(page.locator('.vv-seek-preview[data-state="ready"]')).toBeVisible({
        timeout: 5000,
      });
      for (const ratio of [0.01, 0.5, 0.99]) {
        await page.mouse.move(
          seekBounds.x + seekBounds.width * ratio,
          seekBounds.y + seekBounds.height / 2,
        );
        const preview = page.locator('.vv-seek-preview[data-state="ready"]');
        await expect(preview).toBeVisible({ timeout: 5000 });
        const previewBounds = await preview.boundingBox();
        expect(previewBounds?.x ?? -1).toBeGreaterThanOrEqual(playerBounds.x);
        expect(
          (previewBounds?.x ?? width) + (previewBounds?.width ?? width + 1),
        ).toBeLessThanOrEqual(playerBounds.x + playerBounds.width);
      }
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
        .evaluate((element) => element.currentTime);
      const progressHolder = page.locator(".vjs-progress-holder");
      await progressHolder.focus();
      await progressHolder.press("ArrowRight");
      await expect
        .poll(() => page.locator("video").evaluate((element) => element.currentTime))
        .toBeGreaterThan(beforeSeek);
      const back = page.getByRole("link", { name: "ライブラリ" });
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
    expect(previewBounds.width / previewBounds.height).toBeCloseTo(16 / 9, 1);
    const image = preview.locator("img");
    await expect(image).toHaveAttribute("src", /\/seek-thumbnail\?.*positionMs=/);
    expect(await image.getAttribute("src")).not.toContain("blob:");
    if (screenshotDir !== undefined) {
      await mkdir(screenshotDir, { recursive: true });
      await page.screenshot({
        path: path.join(screenshotDir, "20260922-seek-thumbnail-portrait-768.png"),
        fullPage: true,
      });
    }
  });
});

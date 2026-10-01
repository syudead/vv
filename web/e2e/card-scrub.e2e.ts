import { createHash } from "node:crypto";
import { mkdir, readFile, readdir } from "node:fs/promises";
import path from "node:path";

import {
  expect,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
  type Request,
} from "@playwright/test";

// 一覧の動画カードのスクラブの帯（specs/032-card-scrub-preview、親 Issue #616 の
// 受け入れ条件 1〜7・9）を、playback.e2e.ts と同じ実物のスプライトで確かめる。

interface Video {
  id: number;
  title: string;
  probeState: string;
  previewState: string;
  previewUrl?: string;
  durationMs?: number;
  seekThumbnailUrl?: string;
  progress?: { positionMs: number; completed: boolean };
}

interface SeekSprite {
  intervalMs: number;
  frameCount: number;
  columns: number;
  rows: number;
  sheets: string[];
}

interface MediaFolder {
  id: number;
  version: number;
}

interface Surface {
  name: string;
  url: () => string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
const videos = new Map<string, Video>();
let folder: MediaFolder | undefined;
let sourceSnapshot = new Map<string, string>();

// ライブラリ、フォルダ画面、フォルダ内の検索結果（親 Issue 要件 8）。
const surfaces: Surface[] = [
  { name: "library", url: () => "/" },
  { name: "folder", url: () => `/folders/${String(folder?.id)}` },
  { name: "folder-search", url: () => `/folders/${String(folder?.id)}?q=direct` },
];

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function card(page: Page, item: Video): Locator {
  return page.locator(`article[data-video-id="${String(item.id)}"]`);
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

// 帯の確かめに使う動画のスプライトとループ再生の素材ができるまで待つ。
async function waitForMedia(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/videos?limit=100");
        const page = (await response.json()) as { items: Video[] };
        for (const item of page.items) videos.set(item.title, item);
        const direct = videos.get("direct");
        const portrait = videos.get("portrait");
        if (direct?.previewState !== "done" || direct.previewUrl === undefined) {
          return "preview";
        }
        for (const item of [direct, portrait]) {
          if (item?.seekThumbnailUrl === undefined) return "sprite";
          // 配置情報は、スプライトが完成するまで 409 を返す。
          if (!(await request.get(item.seekThumbnailUrl)).ok()) return "sprite";
        }
        return "ready";
      },
      { timeout: 180_000 },
    )
    .toBe("ready");
}

async function spriteOf(request: APIRequestContext, item: Video): Promise<SeekSprite> {
  if (item.seekThumbnailUrl === undefined) throw new Error("no sprite");
  const response = await request.get(item.seekThumbnailUrl);
  expect(response.ok()).toBe(true);
  return (await response.json()) as SeekSprite;
}

/** seekRequests はカードとプレイヤーが出すシーク用スプライトへの要求を数える。 */
function seekRequests(page: Page) {
  const layout: string[] = [];
  const sheets: string[] = [];
  page.on("request", (request: Request) => {
    const pathname = new URL(request.url()).pathname;
    if (/\/seek-thumbnail$/.test(pathname)) layout.push(request.url());
    if (/\/seek-thumbnail\/\d+$/.test(pathname)) sheets.push(request.url());
  });
  return { layout, sheets };
}

async function box(locator: Locator) {
  const bounds = await locator.boundingBox();
  if (bounds === null) throw new Error("element is not visible");
  return bounds;
}

/** bandPoint は帯の中の、左端から ratio の位置の座標である。右端は帯の内側に収める。 */
async function bandPoint(target: Locator, ratio: number) {
  const band = await box(target.locator("[data-scrub-band]"));
  return {
    x: Math.min(band.x + band.width * ratio, band.x + band.width - 0.5),
    y: band.y + band.height / 2,
  };
}

/** mediaTop はサムネイルの面の上の方（帯の外）の座標である。 */
async function mediaTop(target: Locator) {
  const media = await box(target.locator("[data-preview-media]"));
  return { x: media.x + media.width / 2, y: media.y + media.height * 0.2 };
}

/** frameIndex は表示中のコマの背景の位置から、シートの中のコマの番号を読む。 */
async function frameIndex(target: Locator, sprite: SeekSprite) {
  const position = await target
    .locator("[data-scrub-frame-image]")
    .evaluate((element) => getComputedStyle(element).backgroundPosition);
  const [x, y] = position.split(" ").map((value) => Number.parseFloat(value));
  const column =
    sprite.columns > 1 ? Math.round(((x ?? 0) / 100) * (sprite.columns - 1)) : 0;
  const row = sprite.rows > 1 ? Math.round(((y ?? 0) / 100) * (sprite.rows - 1)) : 0;
  return { position, index: row * sprite.columns + column };
}

/** midFrameRatio は frame 番目のコマの真ん中を指す、帯の割合である。 */
function midFrameRatio(sprite: SeekSprite, durationMs: number, frame: number) {
  return ((frame + 0.5) * sprite.intervalMs) / durationMs;
}

async function layoutOf(page: Page, target: Locator) {
  return {
    rect: await target.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return [rect.x, rect.y, rect.width, rect.height];
    }),
    scrollY: await page.evaluate(() => window.scrollY),
  };
}

async function screenshot(page: Page, width: number, state: string) {
  if (screenshotDir === undefined) return;
  await mkdir(screenshotDir, { recursive: true });
  await page.screenshot({
    path: path.join(screenshotDir, `20261001-card-scrub-${String(width)}-${state}.png`),
    // 画面全体の撮影は窓の大きさを変えて帯を reset するので、表示中の範囲だけを撮る。
    fullPage: false,
  });
}

test.describe.serial("library card scrub band", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(240_000);
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
    await waitForMedia(request);
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

  test("帯の位置のコマを出し、通過では取得せず、帯に入ると 1 回ずつ取得する", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    const item = video("direct");
    const durationMs = item.durationMs ?? 0;
    const sprite = await spriteOf(request, item);
    expect(sprite.frameCount).toBeGreaterThan(1);
    await page.setViewportSize({ width: 1280, height: 800 });

    // プレイヤーのシークバーの吹き出しが、コマの真ん中の割合で出す背景の位置。
    const sample = Math.floor(sprite.frameCount / 2);
    const playerRatio = midFrameRatio(sprite, durationMs, sample);
    await page.goto(`/videos/${String(item.id)}`);
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime > 0.1;
    });
    const seekBar = await box(page.locator(".vjs-progress-holder"));
    await page.mouse.move(
      seekBar.x + seekBar.width * playerRatio,
      seekBar.y + seekBar.height / 2,
    );
    const bubble = page.locator('.vv-seek-preview[data-state="ready"]');
    await expect(bubble).toBeVisible({ timeout: 5000 });
    const playerPosition = await bubble
      .locator(".vv-seek-preview-image")
      .evaluate((element) => getComputedStyle(element).backgroundPosition);

    for (const surface of surfaces) {
      await test.step(surface.name, async () => {
        const requests = seekRequests(page);
        await page.goto(surface.url());
        const target = card(page, item);
        await expect(target).toBeVisible();
        await target.scrollIntoViewIfNeeded();

        // 帯に入らずにカードを縦と横に通り過ぎるだけでは取得しない（受け入れ条件 5）。
        const media = await box(target.locator("[data-preview-media]"));
        await page.mouse.move(media.x - 20, media.y + media.height * 0.3);
        await page.mouse.move(media.x + media.width + 20, media.y + media.height * 0.3, {
          steps: 10,
        });
        await page.mouse.move(media.x + media.width / 2, media.y - 20);
        await page.mouse.move(media.x + media.width / 2, media.y + media.height * 0.6, {
          steps: 10,
        });
        await page.waitForTimeout(300);
        expect(requests.layout).toHaveLength(0);
        expect(requests.sheets).toHaveLength(0);

        // 帯の出入りでカードの矩形とスクロール位置が変わらない（受け入れ条件 9）。
        await page.waitForTimeout(300);
        const before = await layoutOf(page, target);

        // 左端から右端へ、コマが先頭から末尾へ単調に進む（受け入れ条件 1・2）。
        const indices: number[] = [];
        for (const ratio of [0, 0.1, 0.25, 0.4, 0.5, 0.6, 0.75, 0.9, 1]) {
          const point = await bandPoint(target, ratio);
          await page.mouse.move(point.x, point.y);
          await expect(target.locator("[data-scrub-frame-image]")).toBeVisible({
            timeout: 5000,
          });
          indices.push((await frameIndex(target, sprite)).index);
        }
        expect(indices[0]).toBe(0);
        expect(indices.at(-1)).toBe(sprite.frameCount - 1);
        for (let i = 1; i < indices.length; i++) {
          expect(indices[i]).toBeGreaterThanOrEqual(indices[i - 1] ?? 0);
        }
        // 帯の右端でバーが面の右端まで届く。
        const bar = await box(target.locator("[data-scrub-bar] > span"));
        const face = await box(target.locator("[data-scrub-band]"));
        expect(Math.abs(bar.x + bar.width - (face.x + face.width))).toBeLessThan(1);
        expect(await layoutOf(page, target)).toEqual(before);

        // 同じ割合の位置で、再生画面の吹き出しと同じコマになる（受け入れ条件 3）。
        const point = await bandPoint(target, playerRatio);
        await page.mouse.move(point.x, point.y);
        expect((await frameIndex(target, sprite)).position).toBe(playerPosition);

        // 帯を出入りしても取り直さない（受け入れ条件 5）。
        const top = await mediaTop(target);
        await page.mouse.move(top.x, top.y);
        await expect(target.locator("[data-scrub-frame-image]")).toHaveCount(0);
        expect(await layoutOf(page, target)).toEqual(before);
        await page.mouse.move(point.x, point.y);
        await expect(target.locator("[data-scrub-frame-image]")).toBeVisible();
        expect(requests.layout).toHaveLength(1);
        expect(requests.sheets).toHaveLength(1);
        await page.mouse.move(0, 0);
      });
    }
  });

  test("ループ再生中に帯へ入ると止め、上へ出ると止めた場面から再開する", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    const item = video("direct");
    await page.setViewportSize({ width: 1280, height: 800 });
    for (const surface of surfaces) {
      await test.step(surface.name, async () => {
        await page.goto(surface.url());
        const target = card(page, item);
        await expect(target).toBeVisible();
        await target.scrollIntoViewIfNeeded();
        const top = await mediaTop(target);
        await page.mouse.move(top.x, top.y);
        const loop = target.locator("video");
        await expect(loop).toHaveCount(1, { timeout: 5000 });
        await expect
          .poll(
            () => loop.evaluate((element) => (element as HTMLVideoElement).currentTime),
            {
              timeout: 10_000,
            },
          )
          .toBeGreaterThan(0.2);

        const point = await bandPoint(target, 0.5);
        await page.mouse.move(point.x, point.y);
        await expect
          .poll(() => loop.evaluate((element) => (element as HTMLVideoElement).paused))
          .toBe(true);
        const stoppedAt = await loop.evaluate(
          (element) => (element as HTMLVideoElement).currentTime,
        );
        await page.waitForTimeout(300);
        expect(
          await loop.evaluate((element) => (element as HTMLVideoElement).currentTime),
        ).toBe(stoppedAt);

        await page.mouse.move(top.x, top.y);
        await expect
          .poll(() => loop.evaluate((element) => (element as HTMLVideoElement).paused))
          .toBe(false);
        await expect
          .poll(() =>
            loop.evaluate((element) => (element as HTMLVideoElement).currentTime),
          )
          .toBeGreaterThan(stoppedAt);
        await page.mouse.move(0, 0);
      });
    }
  });

  test("帯の上のクリックで再生画面を開き、開始位置は変わらない", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);
    const item = video("portrait");
    await page.setViewportSize({ width: 1280, height: 800 });
    for (const surface of surfaces) {
      await test.step(surface.name, async () => {
        const url =
          surface.name === "folder-search"
            ? `${surface.url().split("?")[0] ?? ""}?q=portrait`
            : surface.url();
        const response = await request.get(`/api/videos/${String(item.id)}`);
        const saved = ((await response.json()) as Video).progress?.positionMs ?? 0;
        await page.goto(url);
        const target = card(page, item);
        await expect(target).toBeVisible();
        await target.scrollIntoViewIfNeeded();
        const point = await bandPoint(target, 0.8);
        await page.mouse.move(point.x, point.y);
        await expect(target.locator("[data-scrub-bar]")).toBeVisible();
        await page.mouse.click(point.x, point.y);
        await expect(page).toHaveURL(new RegExp(`/videos/${String(item.id)}$`));
        await page.waitForFunction(() => {
          const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
          return element !== null && element.readyState >= 1;
        });
        const start = await page
          .locator("video.vjs-tech")
          .evaluate((element) => (element as HTMLVideoElement).currentTime);
        expect(Math.abs(start * 1000 - saved)).toBeLessThan(1000);
      });
    }
  });

  test("取得待ちと取得失敗でも時刻とバーだけを差し替え、カードの寸法を変えない", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const landscape = video("direct");
    const portrait = video("portrait");
    for (const width of [360, 768, 1280]) {
      await test.step(String(width), async () => {
        await page.setViewportSize({ width, height: 800 });

        // 取得待ち: シートの応答を止めておく。
        let releaseSheet: (() => void) | undefined;
        const held = new Promise<void>((resolve) => {
          releaseSheet = resolve;
        });
        await page.route("**/seek-thumbnail/*", async (route) => {
          await held;
          await route.continue();
        });
        await page.goto("/");
        const target = card(page, landscape);
        await target.scrollIntoViewIfNeeded();
        const top = await mediaTop(target);
        await page.mouse.move(top.x, top.y);
        await page.waitForTimeout(300);
        const before = await layoutOf(page, target);
        const point = await bandPoint(target, 0.5);
        await page.mouse.move(point.x, point.y);
        await expect(target.locator("[data-scrub-bar]")).toBeVisible();
        await expect(target.getByText(/^\d+:\d\d \/ \d+:\d\d$/)).toBeVisible();
        await expect(target.locator("[data-scrub-frame]")).toHaveCount(0);
        expect(await layoutOf(page, target)).toEqual(before);
        await screenshot(page, width, "waiting");

        // 取得が終わると、その時点の位置のコマが出る。
        releaseSheet?.();
        await expect(target.locator("[data-scrub-frame-image]")).toBeVisible({
          timeout: 5000,
        });
        expect(await layoutOf(page, target)).toEqual(before);
        await screenshot(page, width, "frame");
        await page.unroute("**/seek-thumbnail/*");

        // 縦長の動画のコマは、サムネイルと同じ高さで中央に出る。
        const narrow = card(page, portrait);
        await narrow.scrollIntoViewIfNeeded();
        const narrowTop = await mediaTop(narrow);
        await page.mouse.move(narrowTop.x, narrowTop.y);
        const narrowPoint = await bandPoint(narrow, 0.5);
        await page.mouse.move(narrowPoint.x, narrowPoint.y);
        const image = narrow.locator("[data-scrub-frame-image]");
        await expect(image).toBeVisible({ timeout: 5000 });
        // hover の拡大（300ms）が終わってから、面とコマを測る。
        await page.waitForTimeout(400);
        const face = await box(narrow.locator("[data-preview-media]"));
        const frame = await box(image);
        // 面の高さいっぱい・中央で、左右にはぼかしたサムネイルが残る。
        expect(frame.width).toBeLessThan(face.width / 2);
        expect(Math.abs(frame.height - face.height)).toBeLessThan(2);
        expect(
          Math.abs(frame.x + frame.width / 2 - (face.x + face.width / 2)),
        ).toBeLessThan(2);
        await expect(narrow.locator("[data-thumbnail-backdrop]")).toBeVisible();
        await screenshot(page, width, "portrait");
        await page.mouse.move(0, 0);

        // 取得失敗: 配置情報の取得が失敗してもサムネイルのまま、時刻とバーは差し替わる。
        await page.route("**/seek-thumbnail?*", (route) => route.abort("failed"));
        await page.goto("/");
        const failing = card(page, landscape);
        await failing.scrollIntoViewIfNeeded();
        const failingTop = await mediaTop(failing);
        await page.mouse.move(failingTop.x, failingTop.y);
        await page.waitForTimeout(300);
        const failingBefore = await layoutOf(page, failing);
        const failingPoint = await bandPoint(failing, 0.5);
        await page.mouse.move(failingPoint.x, failingPoint.y);
        await expect(failing.locator("[data-scrub-bar]")).toBeVisible();
        await page.waitForTimeout(500);
        await expect(failing.locator("[data-scrub-frame]")).toHaveCount(0);
        await expect(failing.getByText(/^\d+:\d\d \/ \d+:\d\d$/)).toBeVisible();
        expect(await layoutOf(page, failing)).toEqual(failingBefore);
        await screenshot(page, width, "unavailable");
        await page.unroute("**/seek-thumbnail?*");
        await page.mouse.move(0, 0);
      });
    }
  });
});

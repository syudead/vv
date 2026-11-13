import { readFileSync } from "node:fs";
import path from "node:path";

import {
  expect,
  test,
  type APIRequestContext,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

// The image search on the real stack: generated footage, the real index job, hashing, ffmpeg and a browser drop
// on the library search field, which opens the search's own screen at /image-search. Every query is a moment between keyframes that the current matching finds; a
// failure to find one is a finding to report, never a reason to move the query. Component tests cover the
// states a real search cannot be made to produce; see docs/design-docs/web-testing.md.

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
}

interface SearchAnswer {
  unindexedCount: number;
  items: {
    video: Video;
    match: { videoId: number; kind: string; band: string; rate: number };
  }[];
}

interface Query {
  name: string;
  type: string;
  data: Buffer;
}

// The titles and holds of generateImageSearchFixtures in media-fixtures.mjs.
const PAN = "画像検索のパン";
const RECURRING = "画像検索の繰り返すショット";
const SHOT_B = "画像検索のショットB";
/** The text filter every title of generateImageSearchFixtures holds, its 24 fillers included. */
const LIBRARY_FILTER = "画像検索";
const FILLERS = 24;
const HOLDS = 9;
const HOLD_SECONDS = 3;
/** 1 s after the pan's keyframe at 5 s: the matching finds it by refinement, not on a keyframe. */
const PAN_FRAME_MS = 6_000;
/** 1.5 s into shot A's fifth hold, between the keyframes of a shot that does not move. */
const SHOT_A_MS = 25_500;
/** 1.5 s into shot B's hold at 54 s. */
const SHOT_B_MS = 55_500;

const widths = [375, 768, 1280, 1920] as const;
const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();
const queries: Record<
  "pan" | "shotA" | "shotB" | "pan160" | "panPart",
  Query | undefined
> = {
  pan: undefined,
  shotA: undefined,
  shotB: undefined,
  pan160: undefined,
  panPart: undefined,
};

function query(key: keyof typeof queries): Query {
  const found = queries[key];
  if (found === undefined) throw new Error(`beforeAll did not prepare ${key}`);
  return found;
}

async function listVideo(request: APIRequestContext, title: string) {
  const response = await request.get(
    `/api/videos?limit=50&query=${encodeURIComponent(title)}`,
  );
  const page = (await response.json()) as { items: Video[] };
  return page.items.find((item) => item.title === title);
}

async function cutFrame(request: APIRequestContext, title: string, positionMs: number) {
  const video = videos.get(title);
  if (video === undefined) throw new Error(`${title} is missing`);
  const cut = await request.get(
    `/api/videos/${String(video.id)}/frame?positionMs=${String(positionMs)}`,
  );
  expect(cut.status()).toBe(200);
  return cut.body();
}

async function searchApi(request: APIRequestContext, file: Query): Promise<SearchAnswer> {
  const response = await request.post("/api/image-search", {
    headers: { Origin: origin },
    multipart: { file: { name: file.name, mimeType: file.type, buffer: file.data } },
  });
  if (!response.ok()) return { unindexedCount: 0, items: [] };
  return (await response.json()) as SearchAnswer;
}

async function titlesFound(request: APIRequestContext, file: Query): Promise<string[]> {
  return (await searchApi(request, file)).items.map((item) => item.video.title);
}

/** The library listing the image search media, its URL as the search field writes it. */
const filteredLibrary = `/?q=${encodeURIComponent(LIBRARY_FILTER)}`;

/** Drops the file on the search field of the page already open, as the browser does with a file from outside. */
async function dropOnField(page: Page, file: Query) {
  const box = page.getByLabel("Search videos", { exact: true });
  await expect(box).toBeVisible();
  const field = box.locator("..");
  const dataTransfer = await page.evaluateHandle(
    ({ base64, name, type }) => {
      const bytes = Uint8Array.from(atob(base64), (char) => char.charCodeAt(0));
      const transfer = new DataTransfer();
      transfer.items.add(new File([bytes], name, { type }));
      return transfer;
    },
    { base64: file.data.toString("base64"), name: file.name, type: file.type },
  );
  await field.dispatchEvent("dragenter", { dataTransfer });
  await field.dispatchEvent("dragover", { dataTransfer });
  await expect(field).toHaveAttribute("data-file-drag", "field");
  await field.dispatchEvent("drop", { dataTransfer });
}

/** Drops the file on the library's search field, which opens the image search screen. */
async function dropOnSearchField(page: Page, file: Query, from = "/") {
  await page.goto(from);
  await dropOnField(page, file);
  await expect(page).toHaveURL(/\/image-search$/);
}

/** The results column of the image search screen. */
function resultsColumn(page: Page): Locator {
  return page.locator("[data-slot=query-results-page-body]");
}

/** Counts the searches the page sends from now on. */
function countSearches(page: Page): () => number {
  let count = 0;
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/api/image-search")) {
      count += 1;
    }
  });
  return () => count;
}

/** The library at the filter, scrolled to `top`, waiting for the restored position to hold. */
async function scrolledLibrary(page: Page, top: number) {
  await page.goto(filteredLibrary);
  await expect(page.getByRole("link", { name: new RegExp(PAN) }).first()).toBeVisible();
  await page.evaluate((y) => window.scrollTo(0, y), top);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(top);
}

/** The library is back with its filter and the scroll position it had. */
async function expectLibraryAt(page: Page, top: number) {
  await expect(page).toHaveURL(new RegExp(`\\?q=${encodeURIComponent(LIBRARY_FILTER)}`));
  await expect(page.getByLabel("Search videos", { exact: true })).toHaveValue(
    LIBRARY_FILTER,
  );
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(top);
}

/** Whether the element's box lies inside the viewport, as the owner sees it without scrolling. */
async function inViewport(page: Page, locator: Locator): Promise<boolean> {
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  if (box === null || viewport === null) return false;
  return (
    box.y >= 0 &&
    box.x >= 0 &&
    box.y + box.height <= viewport.height &&
    box.x + box.width <= viewport.width
  );
}

/** Waits until every frame picture on screen, and every one the strips have asked for, is drawn. */
async function framesLoaded(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() =>
        [...document.querySelectorAll<HTMLImageElement>("[data-slot=frame-strip] img")]
          .filter((image) => {
            // A picture inside its strip's visible part and the viewport has to be drawn; the rest load later.
            const box = image.getBoundingClientRect();
            const strip = image
              .closest("[data-slot=frame-strip]")
              ?.getBoundingClientRect();
            return (
              image.src !== "" ||
              (strip !== undefined &&
                box.right > strip.left &&
                box.left < strip.right &&
                box.bottom > 0 &&
                box.top < window.innerHeight)
            );
          })
          .every((image) => image.complete && image.naturalWidth > 0),
      ),
    )
    .toBe(true);
}

/** Searches at a width and returns the answer's rows once the first is drawn with its frames loaded. */
async function searchAt(page: Page, width: number, file: Query): Promise<Locator> {
  await page.setViewportSize({ width, height: 900 });
  await dropOnSearchField(page, file);
  const rows = resultsColumn(page).locator("[data-slot=result-row]");
  await expect(rows.first()).toBeVisible({ timeout: 60_000 });
  await framesLoaded(page);
  return rows;
}

/** One screenshot per width and state, attached for the review of ui-design.md's criteria. */
async function shoot(page: Page, testInfo: TestInfo, name: string) {
  const file = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path: file });
  await testInfo.attach(name, { path: file, contentType: "image/png" });
}

function title(row: Locator): Locator {
  return row.locator("[data-slot=result-row-title]");
}

function frameLinks(row: Locator): Locator {
  return row.getByRole("list", { name: "Matching frames" }).getByRole("link");
}

/** The `t` of a player link, in seconds. */
async function startOf(link: Locator): Promise<number> {
  const href = await link.getAttribute("href");
  return Number(new URL(href ?? "", origin).searchParams.get("t"));
}

async function pageScrollsSideways(page: Page): Promise<boolean> {
  return page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
}

test.describe("画像検索", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(300_000);
    const root = process.env.MDM_E2E_IMAGE_SEARCH_MEDIA_DIR;
    const queryDir = process.env.MDM_E2E_IMAGE_SEARCH_QUERY_DIR;
    if (root === undefined || queryDir === undefined)
      throw new Error("MDM_E2E_IMAGE_SEARCH_MEDIA_DIR or _QUERY_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);

    for (const name of [PAN, RECURRING, SHOT_B]) {
      await expect
        .poll(async () => (await listVideo(request, name))?.probeState, {
          timeout: 120_000,
        })
        .toBe("done");
      const video = await listVideo(request, name);
      if (video === undefined) throw new Error(`${name} is missing`);
      videos.set(name, video);
    }

    const png = (name: string, data: Buffer) => ({ name, type: "image/png", data });
    queries.pan = png("pan-6s.png", await cutFrame(request, PAN, PAN_FRAME_MS));
    queries.shotA = png("shot-a.png", await cutFrame(request, RECURRING, SHOT_A_MS));
    queries.shotB = png("shot-b.png", await cutFrame(request, RECURRING, SHOT_B_MS));
    const mp4 = (name: string) => ({
      name,
      type: "video/mp4",
      data: readFileSync(path.join(queryDir, name)),
    });
    queries.pan160 = mp4("pan-160.mp4");
    queries.panPart = mp4("pan-5-25.mp4");

    // The index is built by a job after probing; each video's entries must be in place before the drops.
    const pan = query("pan");
    await expect
      .poll(async () => (await titlesFound(request, pan))[0], { timeout: 180_000 })
      .toBe(PAN);
    const shotA = query("shotA");
    await expect
      .poll(async () => (await titlesFound(request, shotA))[0], { timeout: 180_000 })
      .toBe(RECURRING);
    const shotB = query("shotB");
    await expect
      .poll(async () => (await titlesFound(request, shotB)).slice(0, 2).sort(), {
        timeout: 180_000,
      })
      .toEqual([RECURRING, SHOT_B].sort());
    // The fillers are listed and indexed too, so no screen names videos that were not searched.
    await expect
      .poll(
        async () => {
          const response = await request.get(
            `/api/videos?limit=50&query=${encodeURIComponent(LIBRARY_FILTER)}`,
          );
          return ((await response.json()) as { items: Video[] }).items.length;
        },
        { timeout: 120_000 },
      )
      .toBe(FILLERS + 3);
    await expect
      .poll(async () => (await searchApi(request, pan)).unindexedCount, {
        timeout: 180_000,
      })
      .toBe(0);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("パンの 6 秒のフレームで、パンの行を最初に Best candidate 付きで出し、そのフレームから再生する", async ({
    page,
  }, testInfo) => {
    test.setTimeout(240_000);
    const pan = videos.get(PAN);
    for (const width of widths) {
      const rows = await searchAt(page, width, query("pan"));
      const first = rows.first();
      await expect(title(first)).toHaveText(PAN);
      await expect(first.getByText("Best candidate")).toBeVisible();
      await expect(
        first.getByRole("link", { name: "Play", exact: true }),
      ).toHaveAttribute("data-variant", "default");
      await expect(frameLinks(first)).toHaveCount(1);
      expect(
        Math.abs((await startOf(frameLinks(first).first())) - 6),
      ).toBeLessThanOrEqual(0.25);
      expect(await pageScrollsSideways(page)).toBe(false);
      await shoot(page, testInfo, `pan-frame-${String(width)}`);
    }

    // At 1280 px, open the frame and play from it.
    const rows = await searchAt(page, 1280, query("pan"));
    const frame = frameLinks(rows.first()).first();
    await frame.click();
    await expect(page).toHaveURL(new RegExp(`/videos/${String(pan?.id)}\\?t=`));
    const start = Number(new URL(page.url()).searchParams.get("t"));
    await page.locator(".vjs-big-play-button").click();
    const playedFrom = await page.waitForFunction(() => {
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      if (element === null || element.paused || element.readyState < 2) return null;
      return element.currentTime;
    });
    const position = (await playedFrom.jsonValue()) as number;
    expect(Math.abs(position - PAN_FRAME_MS / 1000)).toBeLessThanOrEqual(0.5);
    expect(position).toBeGreaterThanOrEqual(start - 0.5);
  });

  test("ショット A のフレームで、9 つの保持区間に 1 つずつのフレームを 1 行に出し、ストリップだけが横に動く", async ({
    page,
  }, testInfo) => {
    test.setTimeout(240_000);
    for (const width of widths) {
      const rows = await searchAt(page, width, query("shotA"));
      const first = rows.first();
      await expect(title(first)).toHaveText(RECURRING);
      await expect(first.getByText("9 matching frames", { exact: false })).toBeVisible();
      const links = frameLinks(first);
      await expect(links).toHaveCount(HOLDS);
      const holds = new Set<number>();
      for (const link of await links.all()) {
        const start = await startOf(link);
        const hold = Math.floor(start / (2 * HOLD_SECONDS));
        expect(start - hold * 2 * HOLD_SECONDS).toBeLessThan(HOLD_SECONDS);
        holds.add(hold);
      }
      expect(holds.size).toBe(HOLDS);

      // The strip cuts the frames that do not fit beside the query panel; up to 1280 px nine never fit.
      const strip = first.getByRole("list", { name: "Matching frames" });
      const { overflows, fits } = await strip.evaluate((element) => {
        const frames = [...element.children] as HTMLElement[];
        const gap = Number.parseFloat(getComputedStyle(element).columnGap);
        const needed = frames.reduce(
          (sum, frame) => sum + frame.getBoundingClientRect().width + gap,
          -gap,
        );
        return {
          overflows: element.scrollWidth > element.clientWidth,
          fits: needed <= element.clientWidth,
        };
      });
      expect(overflows).toBe(!fits);
      if (width <= 1280) expect(overflows).toBe(true);
      await shoot(page, testInfo, `shot-a-${String(width)}`);

      // One tab stop for the strip; End reaches the ninth frame by scrolling the strip, not the page.
      await links.first().focus();
      await page.keyboard.press("End");
      await expect(links.nth(HOLDS - 1)).toBeFocused();
      await expect(links.nth(HOLDS - 1)).toBeInViewport();
      if (overflows) {
        expect(await strip.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
      }
      expect(await pageScrollsSideways(page)).toBe(false);
      await framesLoaded(page);
      await shoot(page, testInfo, `shot-a-end-${String(width)}`);
    }
  });

  test("ショット B のフレームで、同じ表示率の行を 2 つ出し、Best candidate も default の Play も出さない", async ({
    page,
  }, testInfo) => {
    test.setTimeout(240_000);
    for (const width of widths) {
      const rows = await searchAt(page, width, query("shotB"));
      const titles = [
        await title(rows.nth(0)).innerText(),
        await title(rows.nth(1)).innerText(),
      ];
      expect(titles.sort()).toEqual([RECURRING, SHOT_B].sort());
      const rings = rows.locator("[data-slot=rate-ring]");
      await expect(rings.nth(1)).toHaveText((await rings.nth(0).innerText()).trim());
      const results = resultsColumn(page);
      await expect(results.getByText("Best candidate")).toHaveCount(0);
      for (const play of await results
        .getByRole("link", { name: "Play", exact: true })
        .all()) {
        await expect(play).toHaveAttribute("data-variant", "secondary");
      }
      expect(await pageScrollsSideways(page)).toBe(false);
      await shoot(page, testInfo, `shot-b-${String(width)}`);
    }
  });

  test("160 px に再エンコードしたパンで、Whole video の行と、レーンを満たす SpanBar のブロックを出す", async ({
    page,
  }, testInfo) => {
    test.setTimeout(300_000);
    for (const width of widths) {
      const rows = await searchAt(page, width, query("pan160"));
      const first = rows.first();
      await expect(title(first)).toHaveText(PAN);
      await expect(first.getByText("Whole video")).toBeVisible();
      const lane = first.getByRole("group", { name: "Matched section" });
      const block = lane.getByRole("button");
      const laneBox = await lane.boundingBox();
      const blockBox = await block.boundingBox();
      if (laneBox === null || blockBox === null) throw new Error("lane is not drawn");
      // A whole match covers at least 90% of the video.
      expect(blockBox.width).toBeGreaterThanOrEqual(laneBox.width * 0.9);
      expect(await pageScrollsSideways(page)).toBe(false);
      await shoot(page, testInfo, `video-query-${String(width)}`);
    }
  });

  test("字幕を描いたパンのフレームと、パンの 5〜25 秒は、API が答えた帯と種類を画面に出す", async ({
    page,
    request,
  }, testInfo) => {
    test.setTimeout(300_000);
    await page.goto("/");
    const captioned = await page.evaluate(async (base64) => {
      const image = new Image();
      image.src = `data:image/png;base64,${base64}`;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.naturalWidth;
      canvas.height = image.naturalHeight;
      const context = canvas.getContext("2d");
      if (context === null) throw new Error("no canvas");
      context.drawImage(image, 0, 0);
      context.font = `bold ${String(Math.round(canvas.height / 9))}px sans-serif`;
      context.textAlign = "center";
      context.lineWidth = 4;
      context.strokeStyle = "black";
      context.fillStyle = "white";
      const y = canvas.height * 0.88;
      context.strokeText("字幕のテスト Subtitle test", canvas.width / 2, y);
      context.fillText("字幕のテスト Subtitle test", canvas.width / 2, y);
      return canvas.toDataURL("image/png").split(",")[1] ?? "";
    }, query("pan").data.toString("base64"));
    const cases: { name: string; file: Query }[] = [
      {
        name: "captioned",
        file: {
          name: "captioned.png",
          type: "image/png",
          data: Buffer.from(captioned, "base64"),
        },
      },
      { name: "pan-part", file: query("panPart") },
    ];
    for (const { name, file } of cases) {
      const answer = await searchApi(request, file);
      const item = answer.items.find((found) => found.video.title === PAN);
      testInfo.annotations.push({
        type: name,
        description: JSON.stringify(item?.match ?? "not found"),
      });
      if (item === undefined) throw new Error(`${name}: the API does not list the pan`);
      const bandName = item.match.band === "strong" ? "Strong match" : "Similar";
      const percent = `${String(Math.round(item.match.rate * 100))}%`;
      for (const width of widths) {
        await searchAt(page, width, file);
        const band = page
          .locator("[data-slot=result-band]")
          .filter({ has: page.getByText(PAN, { exact: true }) });
        const row = band
          .locator("[data-slot=result-row]")
          .filter({ has: page.getByText(PAN, { exact: true }) });
        await expect(band.getByRole("heading")).toContainText(bandName);
        await expect(row.locator("[data-slot=rate-ring]")).toHaveText(percent);
        await expect(row.locator("[data-slot=rate-ring]")).toHaveAttribute(
          "data-tone",
          item.match.band === "strong" ? "primary" : "muted",
        );
        if (item.match.kind !== "frame") {
          await expect(
            row.getByText(item.match.kind === "whole" ? "Whole video" : "Part of video"),
          ).toBeVisible();
        }
        await shoot(page, testInfo, `${name}-${String(width)}`);
      }
    }
  });

  test("絞り込んでスクロールしたライブラリからパンのフレームを落とすと /image-search を開き、ファイルと最初の行のフレームが画面に入る", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 1280, height: 900 });
    await scrolledLibrary(page, 400);
    await dropOnField(page, query("pan"));
    await expect(page).toHaveURL(/\/image-search$/);

    const rows = resultsColumn(page).locator("[data-slot=result-row]");
    await expect(rows.first()).toBeVisible({ timeout: 60_000 });
    await framesLoaded(page);
    const preview = page.locator("[data-slot=query-preview]");
    expect(await inViewport(page, preview)).toBe(true);
    expect(await inViewport(page, frameLinks(rows.first()).first())).toBe(true);
    // The file's preview is the largest picture on the screen.
    const previewBox = await preview.boundingBox();
    const frameBox = await frameLinks(rows.first()).first().locator("img").boundingBox();
    if (previewBox === null || frameBox === null)
      throw new Error("pictures are not drawn");
    expect(previewBox.width * previewBox.height).toBeGreaterThan(
      frameBox.width * frameBox.height,
    );
  });

  for (const way of ["browser Back", "Library", "chip ×"] as const) {
    test(`${way} で、絞り込みとスクロール位置を保ったライブラリに戻る`, async ({
      page,
    }) => {
      test.setTimeout(120_000);
      await page.setViewportSize({ width: 1280, height: 900 });
      await scrolledLibrary(page, 400);
      await dropOnField(page, query("pan"));
      await expect(
        resultsColumn(page).locator("[data-slot=result-row]").first(),
      ).toBeVisible({ timeout: 60_000 });

      if (way === "browser Back") await page.goBack();
      else if (way === "Library")
        await page.getByRole("button", { name: "Library" }).click();
      else await page.getByRole("button", { name: "Clear image search" }).click();

      await expectLibraryAt(page, 400);
    });
  }

  test("フレームを開いて戻ると、同じ行、畳んだ行、スクロール位置を 2 回目の検索なしで出す", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const searches = countSearches(page);
    // A low viewport, so the two rows of shot B scroll the page.
    await page.setViewportSize({ width: 1280, height: 520 });
    await dropOnSearchField(page, query("shotB"), filteredLibrary);
    const rows = resultsColumn(page).locator("[data-slot=result-row]");
    await expect(rows).toHaveCount(2, { timeout: 60_000 });
    await framesLoaded(page);
    const collapsedTitle = await title(rows.nth(1)).innerText();
    await rows.nth(1).locator("[data-slot=result-row-toggle]").click();
    await expect(frameLinks(rows.nth(1))).toHaveCount(0);
    const top = await page.evaluate(() => {
      window.scrollTo(0, document.documentElement.scrollHeight);
      return window.scrollY;
    });
    expect(top).toBeGreaterThan(0);

    // The dev server's Strict Mode may send the first search twice; only a search after this is counted.
    const sentBefore = searches();
    await frameLinks(rows.nth(0)).first().click();
    await expect(page).toHaveURL(/\/videos\//);
    await page.goBack();

    await expect(page).toHaveURL(/\/image-search$/);
    await expect(rows).toHaveCount(2);
    await expect(title(rows.nth(1))).toHaveText(collapsedTitle);
    await expect(frameLinks(rows.nth(1))).toHaveCount(0);
    await expect(frameLinks(rows.nth(0))).not.toHaveCount(0);
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(top);
    await expect(frameLinks(rows.nth(0)).first()).toBeFocused();
    expect(searches()).toBe(sentBefore);
  });

  test("375 px で、畳んだパネルのバーは行がスクロールする間も上のバーの下に留まる", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 375, height: 500 });
    await dropOnSearchField(page, query("shotB"));
    const rows = resultsColumn(page).locator("[data-slot=result-row]");
    await expect(rows).toHaveCount(2, { timeout: 60_000 });
    const bar = page.locator("[data-slot=query-panel-bar]");
    await expect(bar).toBeVisible();
    // The bar and the first row's header and frames are on screen after the answer.
    expect(await inViewport(page, bar)).toBe(true);
    expect(await inViewport(page, title(rows.first()))).toBe(true);
    expect(await inViewport(page, frameLinks(rows.first()).first())).toBe(true);
    const navbar = await page.locator("header").first().boundingBox();
    if (navbar === null) throw new Error("top bar is not drawn");

    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(0);
    const stuck = await bar.boundingBox();
    if (stuck === null) throw new Error("panel bar is not drawn");
    expect(Math.abs(stuck.y - (navbar.y + navbar.height))).toBeLessThanOrEqual(16);
    expect(await pageScrollsSideways(page)).toBe(false);
  });

  test("直接開いた /image-search は Idle を出し、外に落としたファイルでは何も始まらず、Library はアプリ内で / を開く", async ({
    page,
  }, testInfo) => {
    test.setTimeout(120_000);
    const searches = countSearches(page);
    for (const width of widths) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/image-search");
      await expect(
        page.getByRole("heading", { name: "Search by an image or a video" }),
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "Choose a file…" })).toHaveCount(1);
      await expect(page.locator("[data-slot=query-panel]")).toHaveCount(0);
      expect(await pageScrollsSideways(page)).toBe(false);
      await shoot(page, testInfo, `idle-${String(width)}`);
    }

    const body = page.locator("[data-slot=query-results-page-body]");
    const dataTransfer = await page.evaluateHandle((base64) => {
      const bytes = Uint8Array.from(atob(base64), (char) => char.charCodeAt(0));
      const transfer = new DataTransfer();
      transfer.items.add(new File([bytes], "pan.png", { type: "image/png" }));
      return transfer;
    }, query("pan").data.toString("base64"));
    await body.dispatchEvent("dragover", { dataTransfer });
    await body.dispatchEvent("drop", { dataTransfer });
    await expect(
      page.getByRole("heading", { name: "Search by an image or a video" }),
    ).toBeVisible();
    expect(searches()).toBe(0);

    await page.getByRole("button", { name: "Library" }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByLabel("Search videos", { exact: true })).toBeVisible();
  });

  test("縦長の画像は一致なしをプレビューごと出し、無地の画像は理由を結果の列に出す", async ({
    page,
  }, testInfo) => {
    test.setTimeout(240_000);
    await page.goto("/");
    const drawn = await page.evaluate(() => {
      // A portrait picture of shapes no fixture holds, and a single-colour picture the server refuses.
      const portrait = document.createElement("canvas");
      portrait.width = 360;
      portrait.height = 640;
      const context = portrait.getContext("2d");
      if (context === null) throw new Error("no canvas");
      let seed = 7;
      const next = () => {
        seed = (seed * 1103515245 + 12345) % 2147483648;
        return seed / 2147483648;
      };
      for (let index = 0; index < 60; index += 1) {
        context.fillStyle = `hsl(${String(Math.round(next() * 360))} 70% ${String(Math.round(20 + next() * 60))}%)`;
        context.fillRect(
          next() * 360,
          next() * 640,
          20 + next() * 140,
          20 + next() * 140,
        );
      }
      const blank = document.createElement("canvas");
      blank.width = 320;
      blank.height = 180;
      const fill = blank.getContext("2d");
      if (fill === null) throw new Error("no canvas");
      fill.fillStyle = "#3a6ea5";
      fill.fillRect(0, 0, 320, 180);
      const data = (canvas: HTMLCanvasElement) =>
        canvas.toDataURL("image/png").split(",")[1] ?? "";
      return { portrait: data(portrait), blank: data(blank) };
    });
    const png = (name: string, base64: string): Query => ({
      name,
      type: "image/png",
      data: Buffer.from(base64, "base64"),
    });
    const portrait = png("portrait.png", drawn.portrait);
    const blank = png("blank.png", drawn.blank);

    for (const width of widths) {
      await page.setViewportSize({ width, height: 900 });
      await dropOnSearchField(page, portrait);
      await expect(
        page.getByRole("heading", { name: "No video has this footage" }),
      ).toBeVisible({ timeout: 60_000 });
      await expect(
        page.getByText("Every video in your library was searched."),
      ).toBeVisible();
      await expect(page.locator("[data-slot=query-panel]")).toContainText("No match");
      if (width >= 1024) {
        // The portrait preview is whole: its box keeps the file's ratio inside the panel.
        const preview = page.locator("[data-slot=query-preview]");
        await expect(preview).toBeVisible();
        const shape = await preview.evaluate((element) => {
          const image = element as HTMLImageElement;
          return image.naturalHeight > image.naturalWidth;
        });
        expect(shape).toBe(true);
      }
      expect(await pageScrollsSideways(page)).toBe(false);
      await shoot(page, testInfo, `no-match-${String(width)}`);

      await dropOnSearchField(page, blank);
      const alert = resultsColumn(page).getByRole("alert");
      await expect(alert).toContainText("There's nothing to compare in this file", {
        timeout: 60_000,
      });
      await expect(
        alert.getByRole("button", { name: "Choose another file…" }),
      ).toBeVisible();
      await expect(page.locator("[data-slot=query-panel]")).toContainText("blank.png");
      expect(await pageScrollsSideways(page)).toBe(false);
      await shoot(page, testInfo, `refused-${String(width)}`);
    }
  });
});

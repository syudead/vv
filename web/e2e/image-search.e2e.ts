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
// on the library search field. Every query is a moment between keyframes that the current matching finds; a
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
  if (!response.ok()) return { items: [] };
  return (await response.json()) as SearchAnswer;
}

async function titlesFound(request: APIRequestContext, file: Query): Promise<string[]> {
  return (await searchApi(request, file)).items.map((item) => item.video.title);
}

/** Drops the file on the library search field, as the browser does with a file from outside the page. */
async function dropOnSearchField(page: Page, file: Query) {
  await page.goto("/");
  const box = page.getByRole("combobox", { name: "Search videos" });
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
  const results = page.getByRole("region", { name: "Image search" });
  const rows = results.locator("[data-slot=result-row]");
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

      // Up to 1280 px the strip cuts the frames that do not fit; at 1920 px all nine fit.
      const strip = first.getByRole("list", { name: "Matching frames" });
      const overflows = await strip.evaluate(
        (element) => element.scrollWidth > element.clientWidth,
      );
      expect(overflows).toBe(width <= 1280);
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
      const results = page.getByRole("region", { name: "Image search" });
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
});

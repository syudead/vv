import { expect, test, type APIRequestContext } from "@playwright/test";

// Drop a frame cut from a fixture video on the library search field and play the first result from
// where it matched, with the real index, matcher and browser drop. Component tests cover the field's
// states and refusals; see docs/design-docs/web-testing.md.

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
  items: { video: Video; match: { videoId: number; positionMs: number } }[];
}

// Keep the title aligned with IMAGE_SEARCH_VIDEO in media-fixtures.mjs.
const IMAGE_SEARCH_VIDEO = "画像検索の確認";
const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
/** Between the keyframes at 6 s and 9 s, so the match comes from refining between them. */
const framePositionMs = 7_000;
let folder: MediaFolder | undefined;
let video: Video | undefined;
let frame: Buffer | undefined;

async function findVideo(request: APIRequestContext): Promise<Video | undefined> {
  const response = await request.get(
    `/api/videos?limit=50&query=${encodeURIComponent(IMAGE_SEARCH_VIDEO)}`,
  );
  const page = (await response.json()) as { items: Video[] };
  return page.items.find((item) => item.title === IMAGE_SEARCH_VIDEO);
}

/** The first result's title once the video is indexed; until then nothing matches. */
async function firstMatch(request: APIRequestContext, png: Buffer): Promise<string> {
  const response = await request.post("/api/image-search", {
    headers: { Origin: origin },
    multipart: { file: { name: "frame.png", mimeType: "image/png", buffer: png } },
  });
  if (!response.ok()) return `${String(response.status())} ${await response.text()}`;
  return ((await response.json()) as SearchAnswer).items[0]?.video.title ?? "none";
}

test.describe("画像検索", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(300_000);
    const root = process.env.MDM_E2E_IMAGE_SEARCH_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_IMAGE_SEARCH_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);

    await expect
      .poll(async () => (await findVideo(request))?.probeState, { timeout: 120_000 })
      .toBe("done");
    video = await findVideo(request);
    if (video === undefined) throw new Error("fixture video is missing");

    const cut = await request.get(
      `/api/videos/${String(video.id)}/frame?positionMs=${String(framePositionMs)}`,
    );
    expect(cut.status()).toBe(200);
    frame = await cut.body();
    // The search index is built by a job after probing; the drop below needs it in place.
    const png = frame;
    await expect
      .poll(async () => firstMatch(request, png), { timeout: 180_000 })
      .toBe(IMAGE_SEARCH_VIDEO);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("受け入れ 1: 切り出したフレームを検索欄に落とし、最初の結果をその位置から再生する", async ({
    page,
  }) => {
    if (video === undefined || frame === undefined)
      throw new Error("beforeAll did not run");
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/");
    const box = page.getByRole("combobox", { name: "Search videos" });
    await expect(box).toBeVisible();
    const field = box.locator("..");

    const dataTransfer = await page.evaluateHandle((base64) => {
      const bytes = Uint8Array.from(atob(base64), (char) => char.charCodeAt(0));
      const transfer = new DataTransfer();
      transfer.items.add(new File([bytes], "frame.png", { type: "image/png" }));
      return transfer;
    }, frame.toString("base64"));
    await field.dispatchEvent("dragenter", { dataTransfer });
    await field.dispatchEvent("dragover", { dataTransfer });
    await expect(field).toHaveAttribute("data-file-drag", "field");
    await field.dispatchEvent("drop", { dataTransfer });

    const results = page.getByRole("region", { name: "Image search" });
    const first = results.getByRole("link").first();
    await expect(first).toContainText(IMAGE_SEARCH_VIDEO, { timeout: 30_000 });
    await expect(page.getByRole("button", { name: "Clear image search" })).toBeVisible();

    await first.click();
    await expect(page).toHaveURL(new RegExp(`/videos/${String(video.id)}\\?t=`));
    const start = Number(new URL(page.url()).searchParams.get("t"));
    expect(Math.abs(start - framePositionMs / 1000)).toBeLessThanOrEqual(0.5);

    await page.locator(".vjs-big-play-button").click();
    const playedFrom = await page.waitForFunction(() => {
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      if (element === null || element.paused || element.readyState < 2) return null;
      return element.currentTime;
    });
    const position = (await playedFrom.jsonValue()) as number;
    expect(position).toBeGreaterThanOrEqual(start - 0.5);
    expect(position).toBeLessThan(start + 3);
  });
});

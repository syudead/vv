import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// Verify scroll restoration from the second shuffled page and visible keyboard focus with real browser layout. Lower-level suites cover search behavior; see docs/design-docs/web-testing.md.

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
}

interface VideoPage {
  items: Video[];
  total: number;
  nextCursor?: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
/** Seven named clips, 62 filler clips and one unknown-duration video. */
const expectedVideos = 70;
let folder: MediaFolder | undefined;

async function listAll(request: APIRequestContext, query = ""): Promise<VideoPage> {
  const response = await request.get(`/api/videos?limit=200${query}`);
  expect(response.ok()).toBe(true);
  return (await response.json()) as VideoPage;
}

async function cardIds(page: Page): Promise<number[]> {
  return (
    await page
      .locator("[data-video-id]")
      .evaluateAll((cards) => cards.map((card) => (card as HTMLElement).dataset.videoId))
  ).map(Number);
}

/** Load pages until the expected number of cards is rendered. */
async function loadEverything(page: Page, total: number) {
  await expect
    .poll(
      async () => {
        await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
        return page.locator("[data-video-id]").count();
      },
      { timeout: 30_000 },
    )
    .toBe(total);
}

test.describe.serial("library search", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(300_000);
    const root = process.env.MDM_E2E_SEARCH_MEDIA_DIR;
    if (root === undefined) throw new Error("MDM_E2E_SEARCH_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    const scanId = ((await scan.json()) as { id: number }).id;
    await expect
      .poll(
        async () => {
          const current = (await (await request.get("/api/scans/current")).json()) as {
            id: number;
            state: string;
          };
          return current.id === scanId && current.state !== "running";
        },
        { timeout: 60_000 },
      )
      .toBe(true);
    // Wait for probing because duration determines the sort order.
    await expect
      .poll(
        async () => {
          const all = await listAll(request);
          return (
            all.total === expectedVideos &&
            all.items.filter((item) => item.probeState === "done").length ===
              expectedVideos - 1
          );
        },
        { timeout: 120_000 },
      )
      .toBe(true);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("15: ランダムは全件を 1 度ずつ出し、2 ページ目の動画から戻ると同じ並びの同じ位置に戻る", async ({
    page,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?sort=random");
    await expect(page).toHaveURL(/\?sort=random&seed=\d+$/);
    const url = page.url();

    await loadEverything(page, expectedVideos);
    const order = await cardIds(page);
    expect(new Set(order).size).toBe(expectedVideos);

    const target = page.locator(`[data-video-id="${String(order[64])}"]`);
    await target.scrollIntoViewIfNeeded();
    const top = await target.evaluate((card) => card.getBoundingClientRect().top);
    await target.getByRole("link").first().click();
    await expect(page).toHaveURL(new RegExp(`/videos/${String(order[64])}$`));
    await page.goBack();
    await expect(page).toHaveURL(url);
    await expect(target).toBeInViewport();
    expect(
      Math.abs((await target.evaluate((card) => card.getBoundingClientRect().top)) - top),
    ).toBeLessThan(8);
    expect(await cardIds(page)).toEqual(order);
  });

  test("23: キーボードだけで結果のカードへ着くと、フォーカスの輪が見える", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?sort=titleAsc");
    await expect(page.locator("[data-video-id]").first()).toBeVisible();

    const focused = () => page.locator(":focus");

    const first = page.locator("[data-video-id]").first();
    for (let i = 0; i < 40; i += 1) {
      await page.keyboard.press("Tab");
      const inResults = await focused().evaluate(
        (element) => element.closest("[data-video-id]") !== null,
      );
      if (inResults) break;
    }
    const target = first.locator(":focus");
    await expect(target).toHaveCount(1);
    const visible = await target.evaluate((element) => {
      const outlined = (candidate: Element | null) =>
        candidate !== null && getComputedStyle(candidate).outlineStyle !== "none";
      return (
        element.matches(":focus-visible") &&
        (outlined(element) || outlined(element.closest("[data-video-id]")))
      );
    });
    expect(visible).toBe(true);
  });
});

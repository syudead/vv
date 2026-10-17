import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// ライブラリの検索・絞り込み・並べ替え（親 Issue #195）のうち、本物のブラウザでしか
// 確かめられないものだけを流す（docs/design-docs/web-testing.md）。ランダムの並びで 2 ページ目の
// 動画を開いて戻ったときのスクロール位置と、キーボードで結果へ着いたときのフォーカスの見え方で
// ある。条件・並べ替え・ページ送り・再読み込みとシャッフルの振る舞いは Vitest
// （src/library/LibraryPage、src/api/useVideos、src/api/listSnapshot）が、検索・絞り込み・
// 並び順の結果は Go のストアのテスト（internal/store/listing*_test.go）が確かめる。
// 動画は run-e2e.mjs が generateSearchFixtures で作る。

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
/** 取り込む動画の数（名前のある 7 本・clip 62 本・長さ不明 1 本）。 */
const expectedVideos = 70;
let folder: MediaFolder | undefined;

async function listAll(request: APIRequestContext, query = ""): Promise<VideoPage> {
  const response = await request.get(`/api/videos?limit=200${query}`);
  expect(response.ok()).toBe(true);
  return (await response.json()) as VideoPage;
}

/** cardIds はカードの動画 id を画面の順に返す。 */
async function cardIds(page: Page): Promise<number[]> {
  return (
    await page
      .locator("[data-video-id]")
      .evaluateAll((cards) => cards.map((card) => (card as HTMLElement).dataset.videoId))
  ).map(Number);
}

/** loadEverything は下までスクロールして、件数分のカードが出るまで続きを読む。 */
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
    // 並べ替えは長さを使うので、読める動画の解析が終わるまで待つ。
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
    await page.goto("/library?sort=random");
    await expect(page).toHaveURL(/\?sort=random&seed=\d+$/);
    const url = page.url();

    await loadEverything(page, expectedVideos);
    const order = await cardIds(page);
    expect(new Set(order).size).toBe(expectedVideos);

    // 2 ページ目の動画を開いて戻ると、同じ並びの同じ位置に戻る。
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
    await page.goto("/library?sort=titleAsc");
    await expect(page.locator("[data-video-id]").first()).toBeVisible();

    const focused = () => page.locator(":focus");
    // ツールバーの操作とレールの項目を越えて、最初の結果のカードへ着く。
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

import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// ライブラリの検索・絞り込み・並べ替え（親 Issue #195）のうち、本物のサーバーと
// ブラウザでしか確かめられないものだけを流す（docs/design-docs/web-testing.md）。
// 未視聴で絞った一覧をスクロールで読み切る画面からサーバーまでの流れ、ランダムの
// 並びでの戻る・再読み込みのスクロール位置、キーボードで結果へ着いたときの
// フォーカスの見え方、画面写真である。条件・並べ替え・手引き・一致なしの振る舞いは
// Vitest（src/library/LibraryPage、src/videoList/SearchBox）が、検索・絞り込み・
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
const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
let folder: MediaFolder | undefined;

async function listAll(request: APIRequestContext, query = ""): Promise<VideoPage> {
  const response = await request.get(`/api/videos?limit=200${query}`);
  expect(response.ok()).toBe(true);
  return (await response.json()) as VideoPage;
}

function byTitle(page: VideoPage, title: string): Video {
  const found = page.items.find((item) => item.title === title);
  if (found === undefined) throw new Error(`${title} is not in the library`);
  return found;
}

async function saveProgress(request: APIRequestContext, id: number, positionMs: number) {
  const response = await request.put(`/api/videos/${String(id)}/progress`, {
    headers: mutationHeaders,
    data: { positionMs },
  });
  expect(response.ok()).toBe(true);
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

/** resultCount はライブラリの件数の表示（「1 item」「70 items」）である。項目（カード）の数を数える。 */
function resultCount(total: number): string {
  return `${total.toLocaleString("en-US")} ${total === 1 ? "item" : "items"}`;
}

/**
 * summary は一覧の上の件数の行である。取り込みの進捗表示も role=status の読み上げを
 * 持つので、件数の段落（p）に絞る。
 */
function summary(page: Page) {
  return page.getByRole("status", { name: "Search results" });
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

    // 未視聴の絞り込みが全件から減るように、1 本に再生位置を残す。15 秒より短い
    // 動画なので、位置を残すと視聴済みになる（internal/domain/progress.go）。
    const all = await listAll(request);
    await saveProgress(request, byTitle(all, "京都旅行 2023").id, 1_000);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("11: 未視聴は全件の数を示し、スクロールに合わせてページ単位で読む", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);
    const unwatched = await listAll(request, "&watch=unwatched");
    expect(unwatched.total).toBeGreaterThan(60);

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/");
    await expect(summary(page)).toHaveText(resultCount(expectedVideos));

    await page
      .getByRole("radiogroup", { name: "View" })
      .getByRole("radio", { name: "List" })
      .click();
    // 行の区切りは行（shadcn/ui の TableRow）の下の線で引く。
    const videoRow = page.locator("tr[data-video-id]").first();
    await expect(videoRow).toHaveCSS("border-bottom-width", "1px");
    await page
      .getByRole("radiogroup", { name: "View" })
      .getByRole("radio", { name: "Grid" })
      .click();

    const requests: URL[] = [];
    page.on("request", (candidate) => {
      const url = new URL(candidate.url());
      if (url.pathname === "/api/library") requests.push(url);
    });

    await page.getByRole("button", { name: "Filter", exact: true }).click();
    await page.getByRole("radio", { name: "Unwatched" }).click();
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(/\?watch=unwatched&sort=addedDesc$/);
    await expect(summary(page)).toHaveText(resultCount(unwatched.total));
    await page.waitForLoadState("networkidle");

    // 選んだ瞬間には 1 ページ目だけを読む。
    const filtered = () =>
      requests.filter((url) => url.searchParams.get("watch") === "unwatched");
    expect(filtered().filter((url) => url.searchParams.has("cursor"))).toHaveLength(0);
    expect(await page.locator("[data-video-id]").count()).toBe(60);

    await loadEverything(page, unwatched.total);
    expect(filtered().some((url) => url.searchParams.has("cursor"))).toBe(true);
    expect(new Set(await cardIds(page))).toEqual(
      new Set(unwatched.items.map((item) => item.id)),
    );
    await expect(summary(page)).toHaveText(resultCount(unwatched.total));
  });

  test("15: ランダムは全件を 1 度ずつ出し、戻る・再読み込みで同じ並び、並べ直すで別の並び", async ({
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

    await page.reload();
    await expect(page).toHaveURL(url);
    await expect(page.locator("[data-video-id]")).toHaveCount(60);
    expect(await cardIds(page)).toEqual(order.slice(0, 60));

    await page.evaluate(() => window.scrollTo(0, 0));
    await page.getByRole("button", { name: "Shuffle" }).click();
    await expect(page).not.toHaveURL(url);
    await expect(page).toHaveURL(/\?sort=random&seed=\d+$/);
    await expect
      .poll(async () => (await cardIds(page)).slice(0, 10))
      .not.toEqual(order.slice(0, 10));
    await page.goBack();
    await expect(page).toHaveURL(url);
  });

  test("23: キーボードだけで検索欄 → 手引き → 絞り込み → 並べ替えと向き → 結果へ進める", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?sort=titleAsc");
    await expect(page.locator("[data-video-id]").first()).toBeVisible();

    const focused = () => page.locator(":focus");
    await page.keyboard.press("/");
    await expect(page.getByRole("searchbox", { name: "Search videos" })).toBeFocused();

    await page.keyboard.press("Tab");
    const help = page.getByRole("button", { name: "How to search" });
    await expect(help).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("dialog", { name: "How to search" }).getByRole("term"),
    ).toHaveCount(4);
    await expect(help).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(page.getByRole("button", { name: "Filter", exact: true })).toBeFocused();
    await expect(page.getByRole("dialog")).toBeHidden();
    await page.keyboard.press("Tab");
    await expect(page.getByRole("button", { name: "Sort by: Title" })).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(
      page.getByRole("button", { name: "Ascending. Press for descending" }),
    ).toBeFocused();

    // 残りのツールバーの操作とレールの項目（人物・団体の 2 つを含む 10 個）を越えて、
    // 最初の結果のカードへ着く。
    const first = page.locator("[data-video-id]").first();
    for (let i = 0; i < 20; i += 1) {
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

  test("画面の画像を撮る", async ({ page, request }) => {
    test.skip(screenshotDir === undefined, "MDM_E2E_SCREENSHOT_DIR is not set");
    test.setTimeout(300_000);
    const dir = screenshotDir ?? "";
    await mkdir(dir, { recursive: true });
    await expect
      .poll(
        async () =>
          (await listAll(request)).items.filter(
            (item) => item.thumbnailState === "pending",
          ).length,
        { timeout: 240_000 },
      )
      .toBe(0);

    const shoot = async (name: string) => {
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(400);
      await page.screenshot({
        path: path.join(dir, `20260924-library-search-${name}.png`),
      });
    };
    for (const width of [360, 768, 1280]) {
      await page.setViewportSize({ width, height: 800 });
      await page.goto(
        `/?q=${encodeURIComponent("京都")}&watch=unwatched&sort=durationDesc`,
      );
      await shoot(`filtered-${String(width)}`);
      await page.goto("/?sort=random&seed=12345");
      await shoot(`random-${String(width)}`);
      await page.goto(
        `/?q=${encodeURIComponent("存在しない題名")}&watch=watched&playable=1&sort=titleAsc`,
      );
      await shoot(`no-match-${String(width)}`);
      await page.goto(`/?q=${encodeURIComponent("京都")}&sort=addedDesc`);
      await page.getByRole("button", { name: "How to search" }).click();
      await expect(page.getByRole("dialog")).toBeVisible();
      await shoot(`help-${String(width)}`);
    }
    await page.setViewportSize({ width: 360, height: 800 });
    await page.goto(
      `/?q=${encodeURIComponent("京都")}&watch=unwatched&sort=durationDesc`,
    );
    await page.getByRole("button", { name: "View and sort" }).click();
    await shoot("compact-360");
    await page.keyboard.press("Escape");
    await page.goto("/?sort=random&seed=12345");
    await page.getByRole("button", { name: "View and sort" }).click();
    await shoot("compact-random-360");
  });
});

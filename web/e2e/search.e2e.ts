import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// ライブラリの検索・絞り込み・並べ替え（親 Issue #195 の受け入れ条件 11〜15・22 と、
// 「未視聴で絞った一覧から再生して戻ると、その動画が同じ位置に残る」）を実ブラウザに
// 通す。動画は run-e2e.mjs が generateSearchFixtures で作る。

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
  durationMs?: number;
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

/** cardTitles は格子のカードの題名を画面の順に返す。 */
function cardTitles(page: Page): Promise<string[]> {
  return page.locator("[data-video-id] h3").allTextContents();
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

function summary(page: Page) {
  return page.getByRole("status").first();
}

/** listed は一覧の要求のうち、並び順が sort の応答を待つ。 */
function listed(page: Page, sort: string) {
  return page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === "/api/videos" && url.searchParams.get("sort") === sort;
  });
}

async function chooseSort(page: Page, current: string, next: string, sort: string) {
  const response = listed(page, sort);
  await page.getByRole("button", { name: `並び順: ${current}` }).click();
  await page.getByRole("menuitemradio", { name: next, exact: true }).click();
  await expect(page.getByRole("button", { name: `並び順: ${next}` })).toBeVisible();
  await response;
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

    // 最近再生した順を確かめるため、3 本に時刻をずらして再生位置を残す。
    // どれも 15 秒より短いので、位置を残すと視聴済みになる（internal/domain/progress.go）。
    const all = await listAll(request);
    await saveProgress(request, byTitle(all, "京都旅行 2023").id, 1_000);
    await new Promise((resolve) => setTimeout(resolve, 1_100));
    await saveProgress(request, byTitle(all, "京都旅行 2024").id, 1_000);
    await new Promise((resolve) => setTimeout(resolve, 1_100));
    await saveProgress(request, byTitle(all, "2024 奈良").id, 1_000);
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
    await expect(summary(page)).toHaveText(`${expectedVideos.toLocaleString("ja-JP")}件`);

    const requests: URL[] = [];
    page.on("request", (candidate) => {
      const url = new URL(candidate.url());
      if (url.pathname === "/api/videos") requests.push(url);
    });

    await page.getByRole("button", { name: "絞り込み" }).click();
    await page.locator("label", { hasText: "未視聴" }).click();
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(/\?watch=unwatched&sort=addedDesc$/);
    await expect(summary(page)).toHaveText(
      `${unwatched.total.toLocaleString("ja-JP")}件`,
    );
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
    await expect(summary(page)).toHaveText(
      `${unwatched.total.toLocaleString("ja-JP")}件`,
    );
  });

  test("12: 条件を載せた URL は別のタブでも同じ一覧を出し、戻る/進むで前の条件に戻る", async ({
    page,
    context,
    request,
  }) => {
    const url = `/?q=${encodeURIComponent("旅行 OR 奈良")}&watch=watched&playable=1&sort=durationDesc`;
    const expected = await listAll(
      request,
      `&query=${encodeURIComponent("旅行 OR 奈良")}&watch=watched&playable=true&sort=durationDesc`,
    );
    expect(expected.total).toBe(3);

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(url);
    await expect
      .poll(() => cardTitles(page))
      .toEqual(expected.items.map((item) => item.title));
    await expect(summary(page)).toHaveText("3件");
    await expect(page.getByRole("searchbox", { name: "動画を検索" })).toHaveValue(
      "旅行 OR 奈良",
    );
    await expect(
      page.getByRole("button", { name: "絞り込み（2 件適用中）" }),
    ).toBeVisible();

    const other = await context.newPage();
    await other.setViewportSize({ width: 1280, height: 800 });
    await other.goto(url);
    await expect
      .poll(() => cardTitles(other))
      .toEqual(expected.items.map((item) => item.title));
    await expect(summary(other)).toHaveText("3件");
    await other.close();

    // 並べ替えを変えると履歴が 1 つ増え、戻ると前の並びに戻る。
    await page.getByRole("button", { name: "降順（長い順）。押すと昇順" }).click();
    await expect(page).toHaveURL(/sort=durationAsc/);
    await expect
      .poll(() => cardTitles(page))
      .toEqual(expected.items.map((item) => item.title).reverse());
    await page.goBack();
    await expect(page).toHaveURL(new RegExp(`sort=durationDesc$`));
    await expect
      .poll(() => cardTitles(page))
      .toEqual(expected.items.map((item) => item.title));
    await page.goForward();
    await expect(page).toHaveURL(/sort=durationAsc$/);

    // 検索語の入力は一続きで 1 つだけ増える。
    await page.goto("/?sort=titleAsc");
    const box = page.getByRole("searchbox", { name: "動画を検索" });
    await box.click();
    await box.pressSequentially("話", { delay: 50 });
    await expect(page).toHaveURL(/\?q=%E8%A9%B1&sort=titleAsc$/);
    await box.pressSequentially(" -10", { delay: 50 });
    await expect(page).toHaveURL(/\?q=%E8%A9%B1\+-10&sort=titleAsc$/);
    await expect.poll(() => cardTitles(page)).toEqual(["2話"]);
    await page.keyboard.press("Tab");
    await page.goBack();
    await expect(page).toHaveURL(/\/\?sort=titleAsc$/);
    await expect(box).toHaveValue("");
  });

  test("13: 7 種の並べ替えを選べ、ランダム以外は向きで並びが逆になる。題名は自然順", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/?q=${encodeURIComponent("旅行 OR 奈良")}&sort=addedDesc`);
    await expect(summary(page)).toHaveText("3件");

    const kinds: [string, string, string][] = [
      ["追加日", "addedDesc", "addedAsc"],
      ["更新日時", "modifiedDesc", "modifiedAsc"],
      ["題名", "titleAsc", "titleDesc"],
      ["長さ", "durationDesc", "durationAsc"],
      ["ファイルサイズ", "sizeDesc", "sizeAsc"],
      ["最近再生した順", "playedDesc", "playedAsc"],
    ];
    let current = "追加日";
    for (const [kind, initial, reversed] of kinds) {
      if (kind !== current) await chooseSort(page, current, kind, initial);
      current = kind;
      await expect(page).toHaveURL(new RegExp(`sort=${initial}$`));
      await expect(page.locator("[data-video-id]")).toHaveCount(3);
      const before = await cardTitles(page);
      const response = listed(page, reversed);
      await page
        .getByRole("button", { name: /^(昇順|降順)(（.+）)?。押すと(昇順|降順)$/ })
        .click();
      await response;
      await expect(page).toHaveURL(new RegExp(`sort=${reversed}$`));
      await expect.poll(() => cardTitles(page)).toEqual([...before].reverse());
    }
    // 更新日時の新しい順は、ファイルの変更日時の新しい順である。
    await chooseSort(page, current, "更新日時", "modifiedDesc");
    await expect
      .poll(() => cardTitles(page))
      .toEqual(["2024 奈良", "京都旅行 2023", "京都旅行 2024"]);

    await chooseSort(page, "更新日時", "ランダム", "random");
    await expect(page).toHaveURL(/sort=random&seed=\d+$/);
    await expect(page.getByRole("button", { name: "並べ直す" })).toBeVisible();

    await page.goto(`/?q=${encodeURIComponent("話")}&sort=titleAsc`);
    await expect.poll(() => cardTitles(page)).toEqual(["2話", "10話"]);
  });

  test("14: 長さの無い動画と再生したことの無い動画は、向きに関係なく末尾に出る", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const durationQuery = encodeURIComponent("長さ不明 OR 旅行");
    await page.goto(`/?q=${durationQuery}&sort=durationDesc`);
    await expect
      .poll(() => cardTitles(page))
      .toEqual(["京都旅行 2023", "京都旅行 2024", "長さ不明"]);
    await page.goto(`/?q=${durationQuery}&sort=durationAsc`);
    await expect
      .poll(() => cardTitles(page))
      .toEqual(["京都旅行 2024", "京都旅行 2023", "長さ不明"]);

    const playedQuery = encodeURIComponent('旅行 OR "clip 01"');
    await page.goto(`/?q=${playedQuery}&sort=playedDesc`);
    await expect
      .poll(() => cardTitles(page))
      .toEqual(["京都旅行 2024", "京都旅行 2023", "clip 01"]);
    await page.goto(`/?q=${playedQuery}&sort=playedAsc`);
    await expect
      .poll(() => cardTitles(page))
      .toEqual(["京都旅行 2023", "京都旅行 2024", "clip 01"]);
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
    await page.getByRole("button", { name: "並べ直す" }).click();
    await expect(page).not.toHaveURL(url);
    await expect(page).toHaveURL(/\?sort=random&seed=\d+$/);
    await expect
      .poll(async () => (await cardIds(page)).slice(0, 10))
      .not.toEqual(order.slice(0, 10));
    await page.goBack();
    await expect(page).toHaveURL(url);
  });

  test("22: 一致なしでは条件や解除操作を重ねない", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?q=zzz-no-such-video&watch=unwatched&playable=1&sort=titleDesc");
    await expect(
      page.getByRole("heading", { name: "条件に一致する動画はありません" }),
    ).toBeVisible();
    await expect(page.getByRole("list", { name: "効いている条件" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "条件を解除" })).toHaveCount(0);
    await expect(page).toHaveURL(
      /\?q=zzz-no-such-video&watch=unwatched&playable=1&sort=titleDesc$/,
    );
    const box = page.getByRole("searchbox", { name: "動画を検索" });
    await expect(box).toHaveValue("zzz-no-such-video");
    await expect(page.getByRole("button", { name: "並び順: 題名" })).toBeVisible();
  });

  test("16: 検索の書き方を開くと 4 つの書き方が出て、閉じても検索語が残る", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/?q=${encodeURIComponent("京都")}&sort=addedDesc`);
    const box = page.getByRole("searchbox", { name: "動画を検索" });
    await expect(box).toHaveValue("京都");
    const url = page.url();

    const help = page.getByRole("button", { name: "検索の書き方" });
    await help.click();
    const dialog = page.getByRole("dialog", { name: "検索の書き方" });
    await expect(dialog.getByRole("heading", { name: "検索の書き方" })).toBeVisible();
    await expect(dialog.getByRole("term")).toHaveText([
      "京都 2024",
      '"京都旅行 2024"',
      "京都 -2023",
      /京都 OR 奈良\s*京都 \| 奈良/,
    ]);
    await expect(dialog.getByRole("definition")).toHaveCount(4);
    await expect(help).toBeFocused();

    // 例を押しても検索欄には入らない。
    await dialog.getByText("京都 -2023").click();
    await expect(box).toHaveValue("京都");

    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(help).toBeFocused();
    await expect(box).toHaveValue("京都");
    expect(page.url()).toBe(url);

    await help.click();
    await expect(dialog).toBeVisible();
    await help.click();
    await expect(dialog).toBeHidden();
    await expect(box).toHaveValue("京都");
    expect(page.url()).toBe(url);
  });

  test("23: キーボードだけで検索欄 → 手引き → 絞り込み → 並べ替えと向き → 結果へ進める", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?sort=titleAsc");
    await expect(page.locator("[data-video-id]").first()).toBeVisible();

    const focused = () => page.locator(":focus");
    await page.keyboard.press("/");
    await expect(page.getByRole("searchbox", { name: "動画を検索" })).toBeFocused();

    await page.keyboard.press("Tab");
    const help = page.getByRole("button", { name: "検索の書き方" });
    await expect(help).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(
      page.getByRole("dialog", { name: "検索の書き方" }).getByRole("term"),
    ).toHaveCount(4);
    await expect(help).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(page.getByRole("button", { name: "絞り込み" })).toBeFocused();
    await expect(page.getByRole("dialog")).toBeHidden();
    await page.keyboard.press("Tab");
    await expect(page.getByRole("button", { name: "並び順: 題名" })).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.getByRole("button", { name: "昇順。押すと降順" })).toBeFocused();

    // 残りのツールバーの操作を越えて、最初の結果のカードへ着く。
    const first = page.locator("[data-video-id]").first();
    for (let i = 0; i < 12; i += 1) {
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

  test("未視聴で絞った一覧から再生して戻ると、その動画が同じ位置に残る", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/?watch=unwatched&sort=titleDesc");
    await expect(page.locator("[data-video-id]")).toHaveCount(60);
    const before = await cardTitles(page);
    const status = (await summary(page).textContent()) ?? "";
    // 2 枚目以降の clip を開く（先頭でない位置に残ることを確かめる）。
    const index = before.findIndex((name) => name.startsWith("clip "));
    expect(index).toBeGreaterThan(0);
    const title = before[index] ?? "";

    const id = Number(
      await page.locator("[data-video-id]").nth(index).getAttribute("data-video-id"),
    );
    await page.getByRole("link", { name: title, exact: true }).click();
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
    // 再生して位置が残ったことを、サーバーへの保存で表す。実際の再生は
    // playback.e2e.ts が確かめており、ここでは戻り先の一覧の扱いだけを見る。
    // 短い動画なので視聴済みになり、「未視聴」の条件からは外れる。
    await saveProgress(page.request, id, 1_500);

    await page.goBack();
    await expect(page).toHaveURL(/\?watch=unwatched&sort=titleDesc$/);
    await expect(page.locator("[data-video-id]")).toHaveCount(60);
    // その場では一覧から外さず、同じ位置に残す。
    expect(await cardTitles(page)).toEqual(before);
    await expect(summary(page)).toHaveText(status);

    // 次に読み直したときに反映する。
    await page.reload();
    await expect(page.locator("[data-video-id]")).toHaveCount(60);
    await expect.poll(() => cardTitles(page)).not.toContain(title);
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
      await page.getByRole("button", { name: "検索の書き方" }).click();
      await expect(page.getByRole("dialog")).toBeVisible();
      await shoot(`help-${String(width)}`);
    }
    await page.setViewportSize({ width: 360, height: 800 });
    await page.goto(
      `/?q=${encodeURIComponent("京都")}&watch=unwatched&sort=durationDesc`,
    );
    await page.getByRole("button", { name: "表示と並び順" }).click();
    await shoot("compact-360");
    await page.keyboard.press("Escape");
    await page.goto("/?sort=random&seed=12345");
    await page.getByRole("button", { name: "表示と並び順" }).click();
    await shoot("compact-random-360");
  });
});

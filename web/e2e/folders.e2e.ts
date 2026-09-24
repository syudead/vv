import { mkdir, rm } from "node:fs/promises";
import path from "node:path";

import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// フォルダ画面の受け入れ条件（親 Issue #145）を、quickstart.md のフォルダ構成で
// 実ブラウザに通す。構成は run-e2e.mjs が generateFolderFixtures で作る。

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
/** 取り込むファイル 16 本のうち、同じ内容の複製 2 本を除いた動画の数。 */
const expectedVideos = 14;
const registered: MediaFolder[] = [];

function rootA(): number {
  const found = registered.find((folder) => folder.path.endsWith("/a/movies"));
  if (found === undefined) throw new Error("a/movies is not registered");
  return found.id;
}

function folderUrl(rootId: number, path = ""): string {
  const segments = path === "" ? [] : path.split("/").map(encodeURIComponent);
  return ["/folders", String(rootId), ...segments].join("/");
}

async function waitForLibrary(
  request: APIRequestContext,
  scanId: number,
  expectedVideoCount = expectedVideos,
) {
  const currentScan = async () => {
    const response = await request.get("/api/scans/current");
    return (await response.json()) as { id: number; state: string; failed: number };
  };
  await expect
    .poll(
      async () => {
        const scan = await currentScan();
        return scan.id === scanId && scan.state !== "running" ? "finished" : scan.state;
      },
      { timeout: 60_000 },
    )
    .toBe("finished");
  // 取り込めなかったファイルがあると動画の数が揃わず、下のサムネイル待ちが
  // 上限まで空回りする。ここで結果を見て、すぐに落とす。
  const scan = await currentScan();
  expect({ state: scan.state, failed: scan.failed }).toEqual({
    state: "done",
    failed: 0,
  });
  const states = async () => {
    const response = await request.get("/api/videos?limit=200");
    const page = (await response.json()) as { items: { thumbnailState: string }[] };
    return page.items.map((item) => item.thumbnailState);
  };
  // サムネイルは1本ずつ順に作られるので、待つのは「作り終わるまで」にする。
  // 失敗したものがあれば、下の件数の検査が落とす。
  await expect
    .poll(
      async () => {
        const all = await states();
        return all.length === expectedVideoCount && !all.includes("pending");
      },
      { timeout: 300_000 },
    )
    .toBe(true);
  expect((await states()).filter((state) => state === "done")).toHaveLength(
    expectedVideoCount,
  );
}

/** cardNames はフォルダカードの名前を画面の順に返す。 */
async function cardNames(page: Page): Promise<string[]> {
  return page.locator("[data-folder-path] h3").allTextContents();
}

/** tabUntil は条件を満たす要素にフォーカスが来るまで Tab を押す。 */
async function tabUntil(
  page: Page,
  matches: (element: {
    tag: string;
    label: string;
    href: string;
    text: string;
  }) => boolean,
  key: "Tab" | "Shift+Tab" = "Tab",
) {
  for (let step = 0; step < 60; step += 1) {
    await page.keyboard.press(key);
    const active = await page.evaluate(() => {
      const element = document.activeElement;
      return {
        tag: element?.tagName ?? "",
        label: element?.getAttribute("aria-label") ?? "",
        href: element?.getAttribute("href") ?? "",
        text: element?.textContent?.trim() ?? "",
        // 輪郭は要素自身か、カードなら外側の箱（overflow-hidden で切れるため）に出る。
        visible:
          (element?.matches(":focus-visible") ?? false) &&
          [element, element?.closest("article")].some((box) => {
            if (!box) return false;
            const style = getComputedStyle(box);
            return style.outlineStyle !== "none" && parseFloat(style.outlineWidth) > 0;
          }),
      };
    });
    if (matches(active)) {
      expect(active.visible, "キーボードフォーカスが見えている").toBe(true);
      return;
    }
  }
  throw new Error("focus target was not reached");
}

test.describe.serial("folder browser", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(420_000);
    const root = process.env.MDM_E2E_FOLDERS_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_FOLDERS_MEDIA_DIR is not configured");

    for (const relative of ["a/movies", "b/movies"]) {
      const created = await request.post("/api/media-folders", {
        headers: mutationHeaders,
        data: { path: `${root}/${relative}` },
      });
      expect(created.status()).toBe(201);
      registered.push((await created.json()) as MediaFolder);
    }
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForLibrary(request, ((await scan.json()) as { id: number }).id);
  });

  test.afterAll(async ({ request }) => {
    const response = await request.get("/api/media-folders");
    const current = (await response.json()) as MediaFolder[];
    for (const folder of current.filter((item) =>
      registered.some((mine) => mine.id === item.id),
    )) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("サイドバーからフォルダ画面を開き、同名の登録フォルダをパスで見分ける", async ({
    page,
  }) => {
    await page.goto("/");
    const sidebar = page.getByRole("complementary", { name: "メインナビゲーション" });
    await sidebar.getByRole("link", { name: "フォルダ" }).click();
    await expect(page).toHaveURL(/\/folders$/);

    await expect(
      page.getByRole("link", { name: /^movies、.*\/a\/movies$/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /^movies、.*\/b\/movies$/ }),
    ).toBeVisible();
    await expect(page.locator("[data-folder-path] p[title$='/a/movies']")).toBeVisible();
    await expect(page.locator("[data-folder-path] p[title$='/b/movies']")).toBeVisible();
  });

  test("子フォルダを自然順に並べ、直下だけを出し、プレビューは直下の4件まで", async ({
    page,
    request,
  }) => {
    await page.goto(folderUrl(rootA()));
    await expect(page.locator("[data-folder-path]").first()).toBeVisible();
    expect(await cardNames(page)).toEqual([
      "1",
      "2",
      "10",
      "100% #1 日本語",
      "A",
      "dup",
      "five",
      "only-deeper",
    ]);

    const five = page.locator('[data-folder-path="five"]');
    await expect(five.locator("img")).toHaveCount(4);
    const deeper = page.locator('[data-folder-path="only-deeper"]');
    await expect(deeper.locator("img")).toHaveCount(0);
    await expect(deeper).toContainText("動画 0 本");

    // A を開くと、直下の x と子フォルダ B だけが出る。
    await page.getByRole("link", { name: "A、動画 1 本、フォルダ 1 件" }).click();
    await expect(page).toHaveURL(folderUrl(rootA(), "A"));
    const videoLinks = page.locator("[data-video-id] a");
    await expect(videoLinks).toHaveCount(1);
    await expect(videoLinks.first()).toHaveAccessibleName("x");
    expect(await cardNames(page)).toEqual(["B"]);

    const b = page.getByRole("link", { name: "B、動画 1 本、フォルダ 1 件" });
    await expect(b).toBeVisible();
    // 子フォルダの一群は動画の一群より前に出る。
    const folderBox = await b.boundingBox();
    const videoBox = await videoLinks.first().boundingBox();
    expect(folderBox!.y).toBeLessThan(videoBox!.y);

    // B のプレビューは直下の y だけで、孫の z は入らない。
    const y = (await (
      await request.get(`/api/folders/${String(rootA())}/videos?path=A%2FB`)
    ).json()) as { items: { id: number; title: string }[] };
    expect(y.items.map((item) => item.title)).toEqual(["y"]);
    const previews = page.locator('[data-folder-path="A/B"] img');
    await expect(previews).toHaveCount(1);
    await expect(previews.first()).toHaveAttribute(
      "src",
      new RegExp(`^/api/videos/${String(y.items[0]!.id)}/thumbnail`),
    );
  });

  test("並び順を変えると動画だけを読み直す", async ({ page }) => {
    const listings: string[] = [];
    page.on("request", (request) => {
      if (/\/api\/folders\/\d+\?path=five$/.test(request.url()))
        listings.push(request.url());
    });
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(folderUrl(rootA(), "five"));
    const first = page.locator("[data-video-id] h3").first();
    await expect(first).toHaveText("part-5");
    // 開発サーバーは StrictMode で効果を2回走らせるので、読み込みが落ち着いた後の数と比べる。
    await page.waitForLoadState("networkidle");
    const before = listings.length;

    await page.getByRole("button", { name: "並び順: 追加日" }).click();
    await page.getByRole("menuitemradio", { name: "題名" }).click();
    await expect(first).toHaveText("part-1");
    await page.waitForLoadState("networkidle");
    expect(listings).toHaveLength(before);
  });

  test("パンくず・再読み込み・戻るで同じフォルダを開く", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(folderUrl(rootA(), "A/B"));
    await page.getByRole("link", { name: "C、動画 1 本、フォルダ 0 件" }).click();
    const crumbs = page.getByRole("navigation", { name: "パンくず" });
    await expect(crumbs.locator('[aria-current="page"]')).toHaveText("C");

    await page.reload();
    await expect(crumbs.locator('[aria-current="page"]')).toHaveText("C");
    await expect(page.locator("[data-video-id] h3")).toHaveText(["z"]);

    await page.goBack();
    await expect(page).toHaveURL(folderUrl(rootA(), "A/B"));
    await expect(crumbs.locator('[aria-current="page"]')).toHaveText("B");

    await crumbs.getByRole("link", { name: "movies" }).click();
    await expect(page).toHaveURL(folderUrl(rootA()));

    const special = folderUrl(rootA(), "100% #1 日本語");
    await page.goto(special);
    await page.reload();
    await expect(crumbs.locator('[aria-current="page"]')).toHaveText("100% #1 日本語");
    await expect(page.locator("[data-video-id] h3")).toHaveText(["s"]);
  });

  test("キーボードだけでフォルダをたどって動画へ行き、パンくずで戻れる", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/");
    await tabUntil(page, (active) => active.href === "/folders");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/folders$/);

    await tabUntil(page, (active) => /^movies、.*\/a\/movies$/.test(active.label));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(folderUrl(rootA()));

    await tabUntil(page, (active) => active.label.startsWith("A、"));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(folderUrl(rootA(), "A"));

    await tabUntil(page, (active) => active.href.startsWith("/videos/"));
    await tabUntil(page, (active) => active.text === "movies", "Shift+Tab");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(folderUrl(rootA()));
  });
});

// フォルダ画面と最上位の検索・絞り込み（親 Issue #195 の受け入れ条件 17〜21、
// #228 の完了の条件）。フォルダ構成は generateFolderSearchFixtures が作る:
// movies/A/x 京都.mp4・movies/A/B/y 京都.mp4・movies/A/大阪.mp4・movies/C/z 京都.mp4
// （A/x・A/B/y・C/z が「京都」を含み、C は A の配下ではない）。
test.describe.serial("folder search", () => {
  const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
  let root: MediaFolder | undefined;
  const expectedVideos = 4;

  function cardTitles(page: Page): Promise<string[]> {
    return page.locator("[data-video-id] h3").allTextContents();
  }

  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    const dir = process.env.MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR;
    if (dir === undefined)
      throw new Error("MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: `${dir}/movies` },
    });
    expect(created.status()).toBe(201);
    root = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForLibrary(
      request,
      ((await scan.json()) as { id: number }).id,
      expectedVideos,
    );
  });

  test.afterAll(async ({ request }) => {
    if (root === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(root.id)}?version=${String(root.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("17: フォルダの中の検索は配下すべてを対象にし、置き場所を添える。消すと元の表示に戻る", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(folderUrl(root!.id, "A"));
    await expect(
      page.getByRole("link", { name: "B、動画 1 本、フォルダ 0 件" }),
    ).toBeVisible();

    const box = page.getByRole("searchbox", { name: "Aの中を検索" });
    await box.fill("京都");
    await expect(page).toHaveURL(/\?q=%E4%BA%AC%E9%83%BD&sort=addedDesc$/);
    await expect(
      page.getByRole("heading", { name: "検索結果", level: 2 }),
    ).toBeAttached();
    await expect.poll(() => cardTitles(page)).toEqual(["x 京都", "y 京都"]);
    await expect(page.getByRole("link", { name: "x 京都、このフォルダ" })).toBeVisible();
    await expect(page.getByRole("link", { name: "y 京都、B" })).toBeVisible();
    // 子フォルダのカードと「フォルダ」「動画」の見出しは出さない。
    await expect(page.getByRole("link", { name: /^B、/ })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: /^フォルダ \d/ })).toHaveCount(0);
    const crumbs = page.getByRole("navigation", { name: "パンくず" });
    await expect(crumbs).toContainText("内を検索中");

    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(new RegExp(`${folderUrl(root!.id, "A")}(\\?|$)`));
    await expect(page.locator("body")).not.toContainText("検索結果");
    await expect(
      page.getByRole("link", { name: "B、動画 1 本、フォルダ 0 件" }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "x 京都" })).toBeVisible();
  });

  test("18: 最上位の検索はライブラリ全体を対象にし、置き場所は登録フォルダ名から始まる", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/folders");
    await expect(page.getByRole("button", { name: "絞り込み" })).toBeDisabled();

    const box = page.getByRole("searchbox", { name: "すべてのフォルダの動画を検索" });
    await box.fill("京都");
    await expect
      .poll(async () => new Set(await cardTitles(page)))
      .toEqual(new Set(["x 京都", "y 京都", "z 京都"]));
    await expect(page.getByRole("button", { name: "絞り込み" })).toBeEnabled();
    await expect(page.getByRole("link", { name: "x 京都、movies/A" })).toBeVisible();
    await expect(page.getByRole("link", { name: "y 京都、movies/A/B" })).toBeVisible();
    await expect(page.getByRole("link", { name: "z 京都、movies/C" })).toBeVisible();
  });

  test("19・20: 絞り込みだけでは子フォルダのカードが残り、一致なしでは中を探す案内を出す", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(folderUrl(root!.id, "A"));
    await page.getByRole("button", { name: "絞り込み" }).click();
    await page.locator("label", { hasText: "未視聴" }).click();
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(/\?watch=unwatched&sort=addedDesc$/);
    await expect(
      page.getByRole("link", { name: "B、動画 1 本、フォルダ 0 件" }),
    ).toBeVisible();
    await expect(page.getByRole("heading", { name: "動画 2", level: 2 })).toBeVisible();

    await page.getByRole("button", { name: "絞り込み（1 件適用中）" }).click();
    await page.locator("label", { hasText: "視聴済み" }).click();
    await page.keyboard.press("Escape");
    await expect(
      page.getByRole("heading", { name: "条件に一致する動画はありません" }),
    ).toBeVisible();
    await expect(
      page.getByText("中のフォルダも探すには、検索語を入れてください。"),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "B、動画 1 本、フォルダ 0 件" }),
    ).toBeVisible();
    const chips = page
      .getByRole("list", { name: "効いている条件" })
      .getByRole("listitem");
    await expect(chips).toHaveText(["視聴済み", "Aの直下"]);

    // 「条件を解除」を押しても同じフォルダに留まる（受け入れ条件 21）。
    await page.getByRole("button", { name: "条件を解除" }).click();
    await expect(page).toHaveURL(new RegExp(`${folderUrl(root!.id, "A")}(\\?|$)`));
    await expect(page.getByRole("link", { name: "x 京都" })).toBeVisible();
  });

  test("最上位で検索語が空のときは登録フォルダだけを出し、絞り込み・並べ替え・向きを無効にする", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/folders");
    await expect(page.getByRole("link", { name: /^movies、/ })).toBeVisible();
    await expect(page.getByRole("button", { name: "絞り込み" })).toBeDisabled();
    await expect(page.getByRole("button", { name: /^並び順:/ })).toBeDisabled();
  });

  test("無いフォルダの URL に q を付けても、一致なしではなく見つからない案内を出す", async ({
    page,
  }) => {
    await page.goto(`${folderUrl(root!.id, "missing")}?q=x`);
    await expect(page.getByText("このフォルダは見つかりません")).toBeVisible();
  });

  test("画面の画像を撮る", async ({ page, request }) => {
    test.skip(screenshotDir === undefined, "MDM_E2E_SCREENSHOT_DIR is not set");
    test.setTimeout(180_000);
    const dir = screenshotDir ?? "";
    await mkdir(dir, { recursive: true });
    await expect
      .poll(
        async () => {
          const response = await request.get("/api/videos?limit=200");
          const page2 = (await response.json()) as {
            items: { thumbnailState: string }[];
          };
          return page2.items.filter((item) => item.thumbnailState === "pending").length;
        },
        { timeout: 120_000 },
      )
      .toBe(0);

    const shoot = async (name: string) => {
      await page.waitForLoadState("networkidle");
      await page.waitForTimeout(400);
      await page.screenshot({
        path: path.join(dir, `20260924-folder-search-${name}.png`),
      });
    };
    for (const width of [360, 768, 1280]) {
      await page.setViewportSize({ width, height: 800 });
      await page.goto(`${folderUrl(root!.id, "A")}?q=${encodeURIComponent("京都")}`);
      await expect.poll(() => cardTitles(page)).toEqual(["x 京都", "y 京都"]);
      await shoot(`results-${String(width)}`);

      await page.goto(`${folderUrl(root!.id, "A")}?watch=unwatched`);
      await expect(
        page.getByRole("link", { name: "B、動画 1 本、フォルダ 0 件" }),
      ).toBeVisible();
      await shoot(`filter-only-${String(width)}`);

      await page.goto(`/folders?q=${encodeURIComponent("京都")}`);
      await expect
        .poll(async () => new Set(await cardTitles(page)))
        .toEqual(new Set(["x 京都", "y 京都", "z 京都"]));
      await shoot(`top-level-${String(width)}`);

      await page.goto(`${folderUrl(root!.id, "A")}?q=zzz-no-such-video`);
      await expect(
        page.getByRole("heading", { name: "条件に一致する動画はありません" }),
      ).toBeVisible();
      await shoot(`no-match-${String(width)}`);

      // 最上位で検索語が空のとき（絞り込み・並べ替え・向きが無効）。
      await page.goto("/folders");
      await expect(page.getByRole("button", { name: "絞り込み" })).toBeDisabled();
      await shoot(`top-level-empty-${String(width)}`);
    }

    // 360px の「表示と並び順」を開いた状態（フォルダ画面）。
    await page.setViewportSize({ width: 360, height: 800 });
    await page.goto(`${folderUrl(root!.id, "A")}?q=${encodeURIComponent("京都")}`);
    await expect.poll(() => cardTitles(page)).toEqual(["x 京都", "y 京都"]);
    await page.getByRole("button", { name: "表示と並び順" }).click();
    await shoot("compact-360");
  });

  test("表示している検索結果のフォルダが取り込みで無くなると、案内が見つからないに切り替わる", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const dir = process.env.MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR;
    if (dir === undefined)
      throw new Error("MDM_E2E_FOLDERS_SEARCH_MEDIA_DIR is not configured");

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`${folderUrl(root!.id, "A/B")}?q=${encodeURIComponent("京都")}`);
    await expect(page.getByRole("link", { name: "y 京都、このフォルダ" })).toBeVisible();

    // 表示している間に B が丸ごと無くなる想定で、配下を消してから取り込み直す。
    // これでこの describe の他のフィクスチャ（x・y・z・大阪）のうち y が無くなるので、
    // 以後のテストはこれに依存しない（この describe の最後のテストである）。
    // アプリ自身の「更新」ボタンから取り込みを始める（scan.start が
    // requestedScanId を覚えるので、取り込みがどれだけ速く終わっても
    // ScanProvider の finished 判定を取りこぼさない。API を直接叩いて外側から
    // 取り込むと、ページの ScanProvider が「実行中」を観測する前に終わってしまい
    // 検知できないことがある）。
    await rm(`${dir}/movies/A/B`, { recursive: true, force: true });
    await page.getByRole("button", { name: "ライブラリを更新" }).click();

    await expect(page.getByText("このフォルダは見つかりません")).toBeVisible({
      timeout: 60_000,
    });
  });
});

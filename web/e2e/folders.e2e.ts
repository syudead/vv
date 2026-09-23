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
/** 取り込むファイル 77 本のうち、同じ内容の複製 2 本を除いた動画の数。 */
const expectedVideos = 75;
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

async function waitForLibrary(request: APIRequestContext, scanId: number) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const scan = (await response.json()) as { id: number; state: string };
        return scan.id === scanId ? scan.state : "other";
      },
      { timeout: 60_000 },
    )
    .toBe("done");
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
        return all.length === expectedVideos && !all.includes("pending");
      },
      { timeout: 300_000 },
    )
    .toBe(true);
  expect((await states()).filter((state) => state === "done")).toHaveLength(
    expectedVideos,
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
      "many",
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
      if (/\/api\/folders\/\d+\?path=many$/.test(request.url()))
        listings.push(request.url());
    });
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(folderUrl(rootA(), "many"));
    const first = page.locator("[data-video-id] h3").first();
    await expect(first).toHaveText("clip-61");
    // 開発サーバーは StrictMode で効果を2回走らせるので、読み込みが落ち着いた後の数と比べる。
    await page.waitForLoadState("networkidle");
    const before = listings.length;

    await page.getByRole("button", { name: "並び順: 追加日" }).click();
    await page.getByRole("menuitemradio", { name: "題名" }).click();
    await expect(first).toHaveText("clip-01");
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

  test("61本を超えるフォルダで続きを読み、再生から戻ると同じ位置に戻る", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(folderUrl(rootA(), "many"));
    const cards = page.locator("[data-video-id]");
    await expect(cards.first()).toBeVisible();
    await expect(async () => {
      await page.mouse.wheel(0, 4000);
      expect(await cards.count()).toBe(61);
    }).toPass({ timeout: 20_000 });

    const target = cards.nth(40);
    await target.scrollIntoViewIfNeeded();
    // click() はリンクを見える位置へもう一度スクロールすることがあるので、押した瞬間の位置を控える。
    await page.evaluate(() => {
      document.addEventListener(
        "click",
        () => {
          (window as unknown as { clickedAt: number }).clickedAt = window.scrollY;
        },
        { capture: true, once: true },
      );
    });
    await target.locator("a").click();
    await expect(page).toHaveURL(/\/videos\/\d+$/);
    const before = await page.evaluate(
      () => (window as unknown as { clickedAt: number }).clickedAt,
    );
    expect(before).toBeGreaterThan(0);

    await page.getByRole("link", { name: "フォルダ" }).click();
    await expect(page).toHaveURL(folderUrl(rootA(), "many"));
    await expect(cards).toHaveCount(61);
    await expect
      .poll(() => page.evaluate(() => window.scrollY))
      .toBeGreaterThan(before - 5);
    expect(Math.abs((await page.evaluate(() => window.scrollY)) - before)).toBeLessThan(
      5,
    );
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

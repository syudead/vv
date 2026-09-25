import { mkdir } from "node:fs/promises";
import path from "node:path";

import {
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  expect,
  type Page,
  test,
} from "@playwright/test";

import { ownerAccount } from "./owner-account";

// ゲスト（未ログイン）の画面（specs/016-single-account-auth、子 #304、ui-design.md
// 「Guest degradation」「Top bar」「Gate」）を実ブラウザに通す。動画は run-e2e.mjs が
// generateGuestFixtures で作る6本で、所有者が「公開あり」の A・B・E・F を公開にし、
// 同じフォルダの C と「非公開だけ」の D は非公開のままにする。
//
// ほかの e2e が登録したフォルダの動画はどれも非公開なので、ゲストに見えるのは
// ここで公開にした4本だけである。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
const publicTitles = ["ゲスト公開A", "ゲスト公開B", "ゲスト公開E", "ゲスト公開F"];
const privateTitles = ["ゲスト非公開C", "ゲスト非公開D"];
const videos = new Map<string, Video>();
let folder: MediaFolder | undefined;

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

async function waitForScan(request: APIRequestContext, scanId: number) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const scan = (await response.json()) as { id: number; state: string };
        return scan.id === scanId && scan.state !== "running" ? scan.state : "running";
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

/** guestContext は Cookie を持たないブラウザを開く。 */
function guestContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({ storageState: { cookies: [], origins: [] } });
}

/**
 * ownerContext は、既存の e2e が使う所有者のセッションとは別に、自分でログインした
 * ブラウザを開く。ここでログアウトしても、ほかの e2e のセッションは続く。
 */
async function ownerContext(browser: Browser): Promise<BrowserContext> {
  const context = await guestContext(browser);
  const login = await context.request.post("/api/auth/login", {
    headers: mutationHeaders,
    data: ownerAccount,
  });
  expect(login.status()).toBe(200);
  return context;
}

/** apiRequests は、そのページが送った /api/* の要求（メソッドとパス）を集める。 */
function apiRequests(page: Page): string[] {
  const seen: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/"))
      seen.push(`${request.method()} ${url.pathname}`);
  });
  return seen;
}

/** cardTitles は一覧のカードの題名を、並びに依らず比べられるよう並べ替えて返す。 */
async function cardTitles(page: Page): Promise<string[]> {
  const titles = await page.locator("article[data-video-id] h3").allTextContents();
  return titles.sort();
}

async function sidebarNames(page: Page): Promise<string[]> {
  const sidebar = page.getByRole("complementary", { name: "メインナビゲーション" });
  const names = await sidebar.locator("a, button").allTextContents();
  return names.map((name) => name.trim());
}

async function startPlayback(page: Page) {
  await page.locator(".vjs-big-play-button").click();
  await page.waitForFunction(() => {
    const element = document.querySelector("video");
    return element !== null && !element.paused && element.currentTime > 0.1;
  });
}

test.describe.serial("guest", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    const root = process.env.MDM_E2E_GUEST_MEDIA_DIR;
    if (root === undefined) throw new Error("MDM_E2E_GUEST_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);

    const titles = [...publicTitles, ...privateTitles];
    await expect
      .poll(
        async () => {
          const response = await request.get("/api/videos?limit=200&query=ゲスト");
          const page = (await response.json()) as { items: Video[] };
          for (const item of page.items) videos.set(item.title, item);
          return titles.every((title) => {
            const found = videos.get(title);
            return found?.probeState === "done" && found.thumbnailState !== "pending";
          });
        },
        { timeout: 120_000 },
      )
      .toBe(true);

    const published = await request.put("/api/video-visibility", {
      headers: mutationHeaders,
      data: { videoIds: publicTitles.map((title) => video(title).id), public: true },
    });
    expect(published.status()).toBe(200);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("所有者が公開にした動画だけが一覧・検索・フォルダ・関連動画に出て、件数も合う", async ({
    browser,
  }) => {
    const context = await guestContext(browser);
    const page = await context.newPage();

    await page.goto("/");
    await expect.poll(() => cardTitles(page)).toEqual(publicTitles);
    await expect(page.getByText("4件", { exact: true })).toBeVisible();

    // 検索: 公開の動画だけに当たり、非公開の題名では何も出ない。
    await page.goto(`/?q=${encodeURIComponent("ゲスト")}`);
    await expect.poll(() => cardTitles(page)).toEqual(publicTitles);
    await expect(page.getByText("4件", { exact: true })).toBeVisible();
    await page.goto(`/?q=${encodeURIComponent("ゲスト非公開")}`);
    await expect(page.getByText("条件に一致する動画はありません")).toBeVisible();

    // フォルダ: 公開の動画を含む登録フォルダだけが出て、パスを添えない。
    await page.goto("/folders");
    const rootCards = page.locator("[data-folder-path]");
    await expect(rootCards).toHaveCount(1);
    await expect(rootCards.locator("h3")).toHaveText("guest-media");
    await expect(rootCards).not.toContainText("/");
    await rootCards.getByRole("link").click();
    // 非公開の動画だけのフォルダは出ない。
    await expect(page.locator("[data-folder-path] h3")).toHaveText(["公開あり"]);
    await page.locator("[data-folder-path]").getByRole("link").click();
    await expect.poll(() => cardTitles(page)).toEqual(publicTitles);
    await expect(page.getByRole("heading", { level: 2, name: /^動画/ })).toHaveText(
      "動画 4",
    );
    // パンくずの登録フォルダの段は、骨組みのまま残らず名前で出る。
    const crumbs = page.getByRole("navigation", { name: "パンくず" });
    await expect(crumbs.getByRole("link", { name: "guest-media" })).toBeVisible();

    // 関連動画: 同じフォルダの公開の動画だけが並ぶ。
    await page.goto(`/videos/${String(video("ゲスト公開A").id)}`);
    const related = page.getByRole("complementary").filter({
      has: page.getByRole("heading", { name: "関連動画" }),
    });
    await expect(related.getByRole("link", { name: /^ゲスト公開B/ })).toBeVisible();
    await expect(related.getByRole("link", { name: /ゲスト非公開/ })).toHaveCount(0);

    await context.close();
  });

  test("ゲストで公開の動画を再生でき、非公開の動画の再生 URL は「開けません」になる", async ({
    browser,
  }) => {
    const context = await guestContext(browser);
    const page = await context.newPage();

    await page.goto(`/videos/${String(video("ゲスト公開A").id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("ゲスト公開A");
    await startPlayback(page);

    for (const title of privateTitles) {
      await page.goto(`/videos/${String(video(title).id)}`);
      await expect(page.getByText("この動画は開けません")).toBeVisible();
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(0);
    }
    await context.close();
  });

  test("ゲストの画面に設定・スキャン・タグ・選択・公開の切り替え・再生位置が出ない", async ({
    browser,
  }) => {
    const context = await guestContext(browser);
    const page = await context.newPage();
    const requests = apiRequests(page);

    await page.goto("/");
    await expect.poll(() => cardTitles(page)).toEqual(publicTitles);
    expect(await sidebarNames(page)).toEqual(["ライブラリ", "フォルダ", "ログイン"]);
    await expect(
      page.getByRole("button", { name: /ライブラリを更新|取り込み/ }),
    ).toHaveCount(0);
    await expect(page.getByRole("checkbox")).toHaveCount(0);
    await page.locator("article[data-video-id]").first().hover();
    await expect(page.getByRole("checkbox")).toHaveCount(0);

    await page.getByRole("button", { name: "絞り込み" }).click();
    const filter = page.getByRole("dialog");
    await expect(filter.getByText("再生できるものだけ")).toBeVisible();
    await expect(filter.getByText("視聴状態")).toHaveCount(0);
    await page.keyboard.press("Escape");

    await page.getByRole("button", { name: /^並び順:/ }).click();
    const menu = page.getByRole("menu");
    await expect(menu.getByRole("menuitemradio")).toHaveCount(6);
    await expect(menu.getByText("最近再生した順")).toHaveCount(0);
    await page.keyboard.press("Escape");

    // 支援技術のツリーに、所有者だけの操作の名前が無い。
    const tree = await page.locator("body").ariaSnapshot();
    for (const name of [
      "視聴状態",
      "最近再生した順",
      "を選択",
      "タグを追加",
      "ライブラリを更新",
    ]) {
      expect(tree).not.toContain(name);
    }
    expect(tree).not.toMatch(/link "設定"|link "タグ"/);

    await page.goto("/folders");
    await expect(page.locator("[data-folder-path]")).toHaveCount(1);

    // 再生画面: タグ・ファイルの場所・LAST PLAYED・公開の切り替えを出さず、
    // 再生しても止めても再生位置を送らない。
    await page.goto(`/videos/${String(video("ゲスト公開B").id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("ゲスト公開B");
    await expect(page.getByText("ADDED")).toBeVisible();
    await expect(page.getByText("LAST PLAYED")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "タグ" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /ファイルを開く/ })).toHaveCount(0);
    await expect(page.getByRole("switch")).toHaveCount(0);
    await startPlayback(page);
    await page.keyboard.press("Space");
    await page.waitForFunction(() => document.querySelector("video")?.paused === true);
    await page.goto("/");
    await expect.poll(() => cardTitles(page)).toEqual(publicTitles);

    const ownerOnly = requests.filter((request) =>
      /\/api\/(events|scans|processing|media-folders|tags|video-tags|video-visibility)|\/progress$/.test(
        request,
      ),
    );
    expect(ownerOnly).toEqual([]);
    // ゲストの一覧は再生位置を持たないので、進捗の帯も視聴済みの印も出ない。
    await expect(page.getByRole("progressbar")).toHaveCount(0);
    await context.close();
  });

  test("2つのタブの一方でログアウトすると、もう一方は次の一覧の取得でゲストの画面になる", async ({
    browser,
  }) => {
    const context = await ownerContext(browser);
    const first = await context.newPage();
    const second = await context.newPage();
    const search = `/?q=${encodeURIComponent("ゲスト")}`;
    await Promise.all([first.goto(search), second.goto(search)]);
    await expect
      .poll(() => cardTitles(second))
      .toEqual([...publicTitles, ...privateTitles].sort());

    // 所有者の一覧で非公開の動画を選んでおく。
    await second.getByRole("checkbox", { name: "「ゲスト非公開C」を選択" }).check();
    await expect(second.getByRole("region", { name: "選択中の操作" })).toBeVisible();

    await first.getByRole("button", { name: "ログアウト" }).click();
    await expect(
      first
        .getByRole("complementary", { name: "メインナビゲーション" })
        .getByRole("link", {
          name: "ログイン",
        }),
    ).toBeVisible();
    await expect.poll(() => cardTitles(first)).toEqual(publicTitles);

    // もう一方のタブは、次の一覧の取得（並び順の向きの切り替え）で読み直される。
    await second.getByRole("button", { name: /押すと/ }).click();
    await expect(
      second
        .getByRole("complementary", { name: "メインナビゲーション" })
        .getByRole("link", {
          name: "ログイン",
        }),
    ).toBeVisible();
    await expect.poll(() => cardTitles(second)).toEqual(publicTitles);
    await expect(second.getByRole("checkbox")).toHaveCount(0);
    await expect(second.getByRole("region", { name: "選択中の操作" })).toHaveCount(0);
    await expect(second.getByText("ゲスト非公開C")).toHaveCount(0);
    await context.close();
  });

  test("再生中にセッションがサーバー側で失効すると、次の操作でゲストの画面になる", async ({
    browser,
    playwright,
  }) => {
    const context = await ownerContext(browser);
    const page = await context.newPage();
    await page.goto(`/videos/${String(video("ゲスト非公開C").id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("ゲスト非公開C");
    await expect(page.getByText("LAST PLAYED")).toBeVisible();
    await startPlayback(page);

    // 同じ Cookie の別の接続からログアウトして、サーバー側でだけセッションを消す。
    // ブラウザには Cookie が残る。
    const other = await playwright.request.newContext({
      baseURL: origin,
      storageState: await context.storageState(),
    });
    const loggedOut = await other.post("/api/auth/logout", { headers: mutationHeaders });
    expect(loggedOut.status()).toBe(204);
    await other.dispose();

    // 次の操作（一時停止で再生位置を送る）で見る人が変わったと分かり、ページごと
    // 読み直す。非公開の動画なので、ゲストの画面では「開けません」になる。
    await page.keyboard.press("Space");
    await expect(page.getByText("この動画は開けません")).toBeVisible();
    await expect(page.getByText("LAST PLAYED")).toHaveCount(0);
    await context.close();
  });

  test("画面写真（MDM_E2E_SCREENSHOT_DIR があるときだけ）", async ({ browser }) => {
    test.skip(screenshotDir === undefined, "画面写真の置き場が無い");
    if (screenshotDir === undefined) return;
    test.setTimeout(120_000);
    await mkdir(screenshotDir, { recursive: true });
    const shot = (page: Page, name: string, width: number) =>
      page.screenshot({
        path: path.join(screenshotDir, `20260925-guest-${name}-${String(width)}.png`),
      });

    for (const width of [360, 768, 1280]) {
      const context = await browser.newContext({
        storageState: { cookies: [], origins: [] },
        viewport: { width, height: 800 },
      });
      const page = await context.newPage();

      await page.goto("/");
      await expect.poll(() => cardTitles(page)).toEqual(publicTitles);
      await shot(page, "library", width);

      if (width >= 768) {
        await page.getByRole("button", { name: "絞り込み" }).click();
        await expect(page.getByRole("dialog")).toBeVisible();
        await shot(page, "filter", width);
        await page.keyboard.press("Escape");
        await page.getByRole("button", { name: /^並び順:/ }).click();
        await expect(page.getByRole("menu")).toBeVisible();
        await shot(page, "sort", width);
        await page.keyboard.press("Escape");
      } else {
        await page.getByRole("button", { name: "表示と並び順" }).click();
        await expect(page.getByRole("dialog")).toBeVisible();
        await shot(page, "sort", width);
        await page.keyboard.press("Escape");
        await page.getByRole("button", { name: "絞り込み" }).click();
        await expect(page.getByRole("dialog")).toBeVisible();
        await shot(page, "filter", width);
        await page.keyboard.press("Escape");
      }

      // サイドバー（1280 は展開、768 はレール、360 はドロワーを開いた状態）。
      if (width === 360) {
        await page.getByRole("button", { name: "メニュー" }).click();
        await expect(
          page.getByRole("complementary", { name: "メインナビゲーション" }),
        ).toBeVisible();
      }
      await shot(page, "sidebar", width);
      if (width === 360) await page.keyboard.press("Escape");

      await page.goto("/folders");
      await expect(page.locator("[data-folder-path]")).toHaveCount(1);
      await shot(page, "folders", width);

      await page.goto(`/videos/${String(video("ゲスト公開A").id)}`);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("ゲスト公開A");
      await expect(page.getByText("ADDED")).toBeVisible();
      await page.screenshot({
        path: path.join(screenshotDir, `20260925-guest-video-${String(width)}.png`),
        fullPage: true,
      });

      // 公開が0本の空の状態（一覧とフォルダ画面の最上位）は、応答を差し替えて撮る。
      await page.route("**/api/videos?*", (route) =>
        route.fulfill({ json: { items: [], total: 0 } }),
      );
      await page.route("**/api/folders", (route) =>
        route.fulfill({ json: { folders: [] } }),
      );
      await page.goto("/");
      await expect(page.getByText("公開されている動画はありません")).toBeVisible();
      await shot(page, "empty", width);
      await page.goto("/folders");
      await expect(page.getByText("公開されている動画はありません")).toBeVisible();
      await shot(page, "folders-empty", width);
      await context.close();
    }
  });
});

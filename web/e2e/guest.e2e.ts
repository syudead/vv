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
// 同じフォルダの C と「非公開だけ」の D は非公開のままにする。「非公開だけ」には
// 画面写真の本数を満たすための非公開の6本（確認用G〜L）も置く。
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
// 画面写真の本数（ui-design.md「Visual review criteria」の 12 本以上）を満たすための、
// 非公開のままの6本。題名で数える確かめに混ざらないよう「ゲスト」を含めない。
const fillerTitles = ["確認用G", "確認用H", "確認用I", "確認用J", "確認用K", "確認用L"];
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
function guestContext(
  browser: Browser,
  viewport?: { width: number; height: number },
): Promise<BrowserContext> {
  return browser.newContext({ storageState: { cookies: [], origins: [] }, viewport });
}

/**
 * ownerContext は、既存の e2e が使う所有者のセッションとは別に、自分でログインした
 * ブラウザを開く。ここでログアウトしても、ほかの e2e のセッションは続く。
 */
async function ownerContext(
  browser: Browser,
  viewport?: { width: number; height: number },
): Promise<BrowserContext> {
  const context = await guestContext(browser, viewport);
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

    const titles = [...publicTitles, ...privateTitles, ...fillerTitles];
    await expect
      .poll(
        async () => {
          for (const query of ["ゲスト", "確認用"]) {
            const response = await request.get(
              `/api/videos?limit=200&query=${encodeURIComponent(query)}`,
            );
            const page = (await response.json()) as { items: Video[] };
            for (const item of page.items) videos.set(item.title, item);
          }
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

    // 関連動画の列: 「公開あり」はグループなので、公開のメンバーだけが「続けて再生」の
    // 並びに出る（specs/017-folder-groups/data-model.md §7）。非公開の動画は並びにも
    // 関連動画にも出ない。
    await page.goto(`/videos/${String(video("ゲスト公開A").id)}`);
    const column = page.getByRole("complementary");
    const members = column.getByRole("region", { name: "続けて再生" });
    await expect(members.getByRole("link", { name: /^ゲスト公開B/ })).toBeVisible();
    await expect(members.locator('li[aria-current="true"]')).toContainText("ゲスト公開A");
    await expect(column.getByRole("link", { name: /ゲスト非公開/ })).toHaveCount(0);

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

    // 再生画面: タグ・ファイルの操作・公開の切り替えを出さず、
    // 再生しても止めても再生位置を送らない。
    await page.goto(`/videos/${String(video("ゲスト公開B").id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("ゲスト公開B");
    await expect(page.getByRole("list", { name: "ファイルの情報" })).toBeVisible();
    await expect(page.getByRole("button", { name: "パスをコピー" })).toHaveCount(0);
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
    await expect(page.getByRole("button", { name: "パスをコピー" })).toBeVisible();
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
    await expect(page.getByRole("button", { name: "パスをコピー" })).toHaveCount(0);
    await context.close();
  });

  // 公開の切り替え（子 #305、ui-design.md「Visibility toggle」）。所有者が再生画面から
  // 1本を、選択バーから複数本を公開にすると、別のブラウザのゲストの一覧に現れ、
  // 非公開に戻すと消える。再スキャンの後も公開のままである。終わったら公開の
  // 組を最初の4本に戻す。
  test("所有者が再生画面と選択バーで切り替えた公開が、別のブラウザのゲストの一覧に反映される", async ({
    browser,
    request,
  }) => {
    test.setTimeout(180_000);
    const owner = await ownerContext(browser);
    const ownerPage = await owner.newPage();
    const guest = await guestContext(browser);
    const guestPage = await guest.newPage();
    const guestSees = async () => {
      await guestPage.goto("/");
      return cardTitles(guestPage);
    };
    const withC = [...publicTitles, "ゲスト非公開C"].sort();
    const withCD = [...publicTitles, ...privateTitles].sort();

    // 再生画面: キーボードだけで「非公開」→ Space →「公開中」（キーボード確認の手順 4）。
    await ownerPage.goto(`/videos/${String(video("ゲスト非公開C").id)}`);
    const toggle = ownerPage.getByRole("switch", {
      name: "ログインしていない人に公開する",
    });
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(toggle).toHaveText("非公開");
    await toggle.focus();
    const sent = ownerPage.waitForRequest(
      (req) => req.method() === "PUT" && req.url().endsWith("/api/video-visibility"),
    );
    await ownerPage.keyboard.press("Space");
    expect((await sent).postDataJSON()).toEqual({
      videoIds: [video("ゲスト非公開C").id],
      public: true,
    });
    await expect(toggle).toHaveAttribute("aria-checked", "true");
    await expect(toggle).toHaveText("公開中");
    // 送信の間もフォーカスは切り替えに残る。
    await expect(toggle).toBeFocused();
    await expect.poll(guestSees).toEqual(withC);

    // もう一度 Space で戻すと、ゲストの一覧から消える。
    await ownerPage.keyboard.press("Space");
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect.poll(guestSees).toEqual(publicTitles);

    // 選択バー: C と D を選んで「公開」→「公開にする」（キーボード確認の手順 5）。
    await ownerPage.goto(`/?q=${encodeURIComponent("ゲスト")}`);
    await expect.poll(() => cardTitles(ownerPage)).toEqual(withCD);
    for (const title of privateTitles) {
      await ownerPage.getByRole("checkbox", { name: `「${title}」を選択` }).check();
    }
    const bar = ownerPage.getByRole("region", { name: "選択中の操作" });
    await bar.getByRole("button", { name: "公開" }).focus();
    await ownerPage.keyboard.press("Enter");
    const menu = ownerPage.getByRole("menu");
    await expect(menu.getByRole("menuitem")).toHaveText(["公開にする", "非公開にする"]);
    await expect(
      menu.getByRole("menuitem", { name: "公開にする", exact: true }),
    ).toBeFocused();
    await ownerPage.keyboard.press("Enter");
    await expect(ownerPage.getByText("2 件を公開にしました")).toBeVisible();
    // 選択は残り、カードの右下に公開の印が出る。
    await expect(bar.getByText("2 件を選択中")).toBeVisible();
    for (const title of privateTitles) {
      const card = ownerPage.locator("article").filter({ hasText: title });
      await expect(card.locator(".lucide-globe")).toHaveCount(1);
      await expect(card.getByText("公開", { exact: true })).toHaveCount(1);
    }
    await expect.poll(guestSees).toEqual(withCD);

    // 再スキャンの後も公開のままである。
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);
    await expect.poll(guestSees).toEqual(withCD);

    // 非公開に戻すと消える。
    await bar.getByRole("button", { name: "公開" }).click();
    await ownerPage.getByRole("menuitem", { name: "非公開にする", exact: true }).click();
    await expect(ownerPage.getByText("2 件を非公開にしました")).toBeVisible();
    for (const title of privateTitles) {
      const card = ownerPage.locator("article").filter({ hasText: title });
      await expect(card.locator(".lucide-globe")).toHaveCount(0);
    }
    await expect.poll(guestSees).toEqual(publicTitles);

    await owner.close();
    await guest.close();
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
      await expect(page.getByRole("list", { name: "ファイルの情報" })).toBeVisible();
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

    // 所有者の公開の切り替え（子 #305、ui-design.md「Visual review criteria」）。
    for (const width of [360, 768, 1280]) {
      const owner = await ownerContext(browser, { width, height: 800 });
      const page = await owner.newPage();
      const ownerShot = (name: string, fullPage = false) =>
        page.screenshot({
          path: path.join(
            screenshotDir,
            `20260925-visibility-${name}-${String(width)}.png`,
          ),
          fullPage,
        });

      // 公開の印のあるカード。絞り込まない一覧で、12 本以上のうち 4 本が公開である
      // （ほかの e2e のフォルダの動画はどれも非公開）。
      await page.goto("/");
      await expect
        .poll(async () => {
          const titles = await cardTitles(page);
          return [...publicTitles, ...privateTitles, ...fillerTitles].every((title) =>
            titles.includes(title),
          );
        })
        .toBe(true);
      expect(await page.locator("article[data-video-id]").count()).toBeGreaterThanOrEqual(
        12,
      );
      await expect(page.locator("article[data-video-id] .lucide-globe")).toHaveCount(4);
      await ownerShot("library");

      // 選択バーの「公開」を開いた状態（360 は2段のバー）。
      for (const title of ["ゲスト公開A", "ゲスト非公開C"]) {
        const checkbox = page.getByRole("checkbox", { name: `「${title}」を選択` });
        await checkbox.check({ force: true });
      }
      const bar = page.getByRole("region", { name: "選択中の操作" });
      await expect(bar).toBeVisible();
      await ownerShot("bar");
      await bar.getByRole("button", { name: "公開" }).click();
      await expect(page.getByRole("menu")).toBeVisible();
      await ownerShot("menu");
      await page.keyboard.press("Escape");
      await page.keyboard.press("Escape");

      // 再生画面の切り替え: 非公開・公開中・送信中・失敗。
      await page.goto(`/videos/${String(video("ゲスト非公開C").id)}`);
      const toggle = page.getByRole("switch", { name: "ログインしていない人に公開する" });
      await expect(toggle).toHaveText("非公開");
      await ownerShot("video-private", true);
      await page.goto(`/videos/${String(video("ゲスト公開A").id)}`);
      await expect(toggle).toHaveText("公開中");
      await ownerShot("video-public", true);
      let release: (() => void) | undefined;
      await page.route("**/api/video-visibility", async (route) => {
        await new Promise<void>((resolve) => (release = resolve));
        await route.fulfill({
          status: 500,
          json: { code: "internal", message: "失敗" },
        });
      });
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-disabled", "true");
      await ownerShot("video-sending", true);
      release?.();
      await expect(page.getByRole("alert")).toHaveText("変更できませんでした");
      await expect(toggle).toHaveText("公開中");
      await ownerShot("video-failed", true);
      await owner.close();
    }
  });
});

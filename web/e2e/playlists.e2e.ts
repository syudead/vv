import {
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";

// 名前付きプレイリストと連続再生（specs/044-playlists、親 #787、子 #903）の流れを、実サーバーと
// 実メディアに通す。動画は run-e2e.mjs が generatePlaylistsFixtures で作る 3 秒の 5 本で、
// 登録フォルダの直下の 2 本（A・B）は単体の動画、「プレイリストのグループ」の 3 本はグループに
// なる。名前の規則、並べ替えの失敗、パネルの読み直しなどの個々の規則はストア・domain・ページの
// テストが持つので、ここでは重ねず流れだけを確かめる（docs/design-docs/web-testing.md の
// Test levels）。括弧の番号は親 Issue の受け入れ条件である。
//
// 3 秒の動画は、末尾とみなす 15 秒（internal/domain/progress.go の CompletionTailMs）より短いので、
// 再生の位置を 1 度でも保存すると見終わったことになる。グループの開始のメンバーは最初の
// 見終わっていないメンバーなので、メンバー1 だけを API で見終わったことにして始める。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
  favorite?: boolean;
}

interface PlaylistSummary {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の PLAYLIST_VIDEOS・PLAYLIST_GROUP・PLAYLIST_GROUP_MEMBERS と同じ名前。
const titleA = "プレイリスト動画A";
const titleB = "プレイリスト動画B";
const groupName = "プレイリストのグループ";
const member1 = "プレイリストのメンバー1";
const member2 = "プレイリストのメンバー2";
const member3 = "プレイリストのメンバー3";
const tonight = "夜に見る";
const themed = "テーマ別";
const librarySearch = `/library?q=${encodeURIComponent("プレイリスト")}`;
const videos = new Map<string, Video>();
const playlists = new Map<string, number>();
let folder: MediaFolder | undefined;

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function playlistId(name: string): number {
  const found = playlists.get(name);
  if (found === undefined) throw new Error(`playlist not created: ${name}`);
  return found;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
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

/**
 * clearProgress はサーバーの DB から、ids の動画の再生位置と見終わったことの記録を消す。記録は
 * 動画の内容に紐づき、登録フォルダを外しても残るので、CI のリトライが同じ DB でこのグループを
 * 初めからやり直すと、前の試行で見終わったメンバー2・3 からグループの開始が変わってしまう。
 * 見終わっていない状態へ戻す API は無いので、DB に直接書く。サーバーは WAL で開いているので、
 * 別の接続から書いてよい。
 */
function clearProgress(ids: number[]) {
  const runRoot = process.env.MDM_E2E_RUN_ROOT;
  if (runRoot === undefined) throw new Error("MDM_E2E_RUN_ROOT is not configured");
  // playwright.config.ts の MDM_DATA_DIR と internal/store/sqlite.go の DatabaseFileName。
  const db = new DatabaseSync(path.join(runRoot, "data", "mdm.db"));
  try {
    db.exec("pragma busy_timeout = 5000");
    db.prepare(
      `delete from playback_progress where content_key in
        (select content_key from videos where id in (${ids.map(() => "?").join(", ")}))`,
    ).run(...ids);
  } finally {
    db.close();
  }
}

/** readPlaylists は API からプレイリストを読み、名前から id を引けるようにする。 */
async function readPlaylists(request: APIRequestContext) {
  const response = await request.get("/api/playlists");
  expect(response.status()).toBe(200);
  for (const item of ((await response.json()) as { items: PlaylistSummary[] }).items) {
    playlists.set(item.name, item.id);
  }
}

/** videoCard はライブラリの、title の動画のカードである。 */
function videoCard(page: Page, title: string): Locator {
  return page.locator("article[data-video-id]").filter({
    has: page.locator("h3", { hasText: new RegExp(`^${escapeRegExp(title)}$`) }),
  });
}

/**
 * addFromCard は、カードの「Add to playlist」から name のプレイリストへ足す。`create` なら
 * 名前を打って「Create "{name}"」で作って足し、そうでなければ一覧のその行を押す。足した結果の
 * 知らせ（text）が出るのを待ち、サーバーの応答の数を返す。前の操作の同じ文の知らせと
 * 取り違えないよう、それが消えてから始める。
 */
async function addFromCard(
  page: Page,
  card: Locator,
  trigger: string,
  name: string,
  text: string,
  create = false,
) {
  const shown = toast(page, text);
  await expect(shown).toHaveCount(0, { timeout: 15_000 });
  const added = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/api\/playlists\/\d+\/items$/.test(new URL(response.url()).pathname),
  );
  await card.hover();
  await card.getByRole("button", { name: trigger }).click();
  const picker = page.getByRole("dialog", { name: "Add to playlist" });
  const field = picker.getByRole("combobox", { name: "Search or create a playlist" });
  await expect(field).toBeFocused();
  await field.fill(name);
  const row = create ? `Create "${name}"` : name;
  await picker.getByRole("option", { name: row, exact: true }).click();
  const response = await added;
  expect(response.status()).toBe(200);
  await expect(picker).toBeHidden();
  await expect(shown).toBeVisible();
  return (await response.json()) as { added: number; existing: number };
}

/** toast は画面の下に出た知らせのうち、text のものである。 */
function toast(page: Page, text: string): Locator {
  return page
    .getByRole("region", { name: /Notifications/ })
    .getByText(text, { exact: true });
}

/** itemRows はプレイリストの画面の項目の、並びのままの読み上げ名（「Play from …」）である。 */
async function itemRows(page: Page): Promise<string[]> {
  const links = page.getByRole("table", { name: "Items" }).getByRole("link");
  return links.evaluateAll((elements) =>
    elements.map((element) => element.getAttribute("aria-label") ?? ""),
  );
}

/** openPlaylist は一覧のカード（名前と項目数が読み上げ名）を押して、そのプレイリストの画面を開く。 */
async function openPlaylist(page: Page, name: string, count: string) {
  await page.goto("/playlists");
  await page
    .locator("article[data-playlist-id]")
    .getByRole("link", { name: `${name}, ${count}` })
    .click();
  await expect(page).toHaveURL(`/playlists/${String(playlistId(name))}`);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
}

/** playlistLine は動画の画面の題名の上の、プレイリストの何番目かを示す行である。 */
function playlistLine(page: Page, position: number): Locator {
  return page.getByRole("link", {
    name: `Playlist "${tonight}", item ${String(position)} of 3. Open the playlist`,
  });
}

function overlay(page: Page): Locator {
  return page.locator("[data-overlay-layer]");
}

/**
 * expectNotice は、今の動画が最後まで再生されて、title を次とする「Up next」の予告が出るのを待つ。
 * 3 秒の動画の読み込みと再生を含むので長めに待つ。
 */
async function expectNotice(page: Page, title: string) {
  const layer = overlay(page);
  await expect(layer.getByRole("button", { name: "Play now" })).toBeVisible({
    timeout: 20_000,
  });
  await expect(layer.getByText("Up next", { exact: true })).toBeVisible();
  await expect(
    layer.getByRole("link", { name: new RegExp(`^${escapeRegExp(title)} `) }),
  ).toBeVisible();
}

/** expectPlaylistVideo は、プレイリストの name の項目 position として title の動画にいることを確かめる。 */
async function expectPlaylistVideo(page: Page, title: string, position: number) {
  await expect(page).toHaveURL(
    new RegExp(
      `/videos/${String(video(title).id)}\\?playlist=${String(playlistId(tonight))}&item=\\d+$`,
    ),
    { timeout: 20_000 },
  );
  await expect(playlistLine(page, position)).toBeVisible();
}

function guestContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({ storageState: { cookies: [], origins: [] } });
}

test.describe.serial("playlists", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    const root = process.env.MDM_E2E_PLAYLISTS_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_PLAYLISTS_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);

    const titles = [titleA, titleB, member1, member2, member3];
    await expect
      .poll(
        async () => {
          const response = await request.get(
            `/api/videos?limit=200&query=${encodeURIComponent("プレイリスト")}`,
          );
          for (const item of ((await response.json()) as { items: Video[] }).items) {
            videos.set(item.title, item);
          }
          return titles.every((title) => {
            const found = videos.get(title);
            return found?.probeState === "done" && found.thumbnailState !== "pending";
          });
        },
        { timeout: 120_000 },
      )
      .toBe(true);

    // 前の試行の記録を消してから、メンバー1 を見終わったことにし、グループの項目が
    // メンバー2 から始まるようにする（5）。
    clearProgress(titles.map((title) => video(title).id));
    const watched = await request.put(
      `/api/videos/${String(video(member1).id)}/progress`,
      { headers: mutationHeaders, data: { positionMs: 3000 } },
    );
    expect(watched.status()).toBe(200);
    expect(((await watched.json()) as { completed: boolean }).completed).toBe(true);

    // 項目を外してもお気に入りが残ることを確かめるため、A をお気に入りにしておく（4）。
    const favorite = await request.put("/api/favorites", {
      headers: mutationHeaders,
      data: { videoIds: [video(titleA).id], favorite: true },
    });
    expect(favorite.status()).toBe(200);
  });

  test.afterAll(async ({ request }) => {
    // テストが id を読む前に止まっても作ったプレイリストを残さないよう、名前で探して消す。
    // 残すと、CI のリトライでピッカーが同じ名前の「Create」を出さない。
    await readPlaylists(request);
    for (const name of [tonight, themed]) {
      const id = playlists.get(name);
      if (id === undefined) continue;
      const removed = await request.delete(`/api/playlists/${String(id)}`, {
        headers: mutationHeaders,
      });
      expect([204, 404]).toContain(removed.status());
    }
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("カードから 2 つのプレイリストに足し、並べ替えは開き直しても残り、外しても動画はライブラリに残る", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch);
    const cardA = videoCard(page, titleA);
    await expect(cardA).toBeVisible();
    const addA = `Add "${titleA}" to a playlist`;

    // 同じ動画を 2 つのプレイリストに足す。プレイリストはピッカーの「Create」で作る（1）。
    const addedTonight = `Added 1 item to "${tonight}"`;
    const one = { added: 1, existing: 0 };
    expect(await addFromCard(page, cardA, addA, tonight, addedTonight, true)).toEqual(
      one,
    );
    expect(
      await addFromCard(page, cardA, addA, themed, `Added 1 item to "${themed}"`, true),
    ).toEqual(one);
    await readPlaylists(request);

    // もう一度足しても項目は増えない（2）。
    expect(
      await addFromCard(page, cardA, addA, tonight, `Already in "${tonight}"`),
    ).toEqual({ added: 0, existing: 1 });

    // B とグループを足す。足した順に末尾へ付くので、並びは A・B・グループになる。
    const cardB = videoCard(page, titleB);
    const addB = `Add "${titleB}" to a playlist`;
    expect(await addFromCard(page, cardB, addB, tonight, addedTonight)).toEqual(one);
    const groupCard = page
      .locator("article[data-group-root]")
      .filter({ hasText: groupName });
    const addGroup = `Add group "${groupName}" to a playlist`;
    expect(await addFromCard(page, groupCard, addGroup, tonight, addedTonight)).toEqual(
      one,
    );

    // サイドバーからプレイリストの一覧へ。A は両方に入っている（1）、A は 1 つだけ（2）。
    await page
      .getByRole("complementary", { name: "Main navigation" })
      .getByRole("link", { name: "Playlists" })
      .click();
    await expect(page).toHaveURL("/playlists");
    await openPlaylist(page, themed, "1 item");
    await expect.poll(() => itemRows(page)).toEqual([`Play from ${titleA}`]);
    await openPlaylist(page, tonight, "3 items");
    await expect
      .poll(() => itemRows(page))
      .toEqual([`Play from ${titleA}`, `Play from ${titleB}`, `Play from ${groupName}`]);

    // グループをキーボードで 1 つ上へ動かし、A・グループ・B にする。並びは開き直しても残る（3）。
    const move = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/playlists\/\d+\/items\/\d+\/move$/.test(new URL(response.url()).pathname),
    );
    await page.getByRole("button", { name: `Reorder "${groupName}"` }).focus();
    await page.keyboard.press("Space");
    // 持ち上げた直後に、その場の上にいることも読み上げられ、持ち上げの文を置き換える。
    await expect(
      page.getByText(`"${groupName}" moved to position 3 of 3.`, { exact: true }),
    ).toBeAttached();
    await page.keyboard.press("ArrowUp");
    await expect(
      page.getByText(`"${groupName}" moved to position 2 of 3.`, { exact: true }),
    ).toBeAttached();
    await page.keyboard.press("Space");
    expect((await move).status()).toBe(204);
    const reordered = [
      `Play from ${titleA}`,
      `Play from ${groupName}`,
      `Play from ${titleB}`,
    ];
    await expect.poll(() => itemRows(page)).toEqual(reordered);
    await page.reload();
    await expect.poll(() => itemRows(page)).toEqual(reordered);

    // テーマ別から A を外す。プレイリストは空になり、A はお気に入りのままライブラリに残る（4）。
    await openPlaylist(page, themed, "1 item");
    await page
      .getByRole("button", { name: `Remove "${titleA}" from the playlist` })
      .click();
    await expect(page.getByText("No items yet", { exact: true })).toBeVisible();
    await page.goto(librarySearch);
    await expect(videoCard(page, titleA)).toBeVisible();
    const after = await request.get(`/api/videos/${String(video(titleA).id)}`);
    expect(after.status()).toBe(200);
    expect(((await after.json()) as Video).favorite).toBe(true);
  });

  test("先頭から再生すると、動画→グループの見終わっていないメンバー→動画と進み、終わりの層で止まる", async ({
    page,
  }) => {
    test.setTimeout(150_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await openPlaylist(page, tonight, "3 items");
    await page.getByRole("link", { name: `Play "${tonight}" from the top` }).click();

    // 1 番目の A。終わるとグループの最初の見終わっていないメンバー2 を予告し、自動で進む（3、5）。
    await expectPlaylistVideo(page, titleA, 1);
    await expectNotice(page, member2);
    await expectPlaylistVideo(page, member2, 2);
    // グループの中は並び順にメンバー3 へ。
    await expectNotice(page, member3);
    await expectPlaylistVideo(page, member3, 2);
    // グループの後は 3 番目の B へ。
    await expectNotice(page, titleB);
    await expectPlaylistVideo(page, titleB, 3);

    // 最後の項目が終わると、終わりの層が出て自動では進まない（7）。
    const end = overlay(page).locator("[data-playlist-end]");
    await expect(end).toBeVisible({ timeout: 20_000 });
    await expect(end).toContainText("End of playlist");
    await expect(end).toContainText(tonight);
    await expect(end.getByRole("button", { name: "Watch again" })).toBeVisible();
    await expect(overlay(page).getByRole("button", { name: "Play now" })).toHaveCount(0);
    const url = page.url();
    await page.waitForTimeout(6_000);
    expect(page.url()).toBe(url);
    await expect(end).toBeVisible();
  });

  test("真ん中の項目から始めると後ろの項目だけを再生し、予告を取り消すとその場に留まる", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await openPlaylist(page, tonight, "3 items");
    // メンバーは前の再生で全部見終わったので、グループの項目は最初のメンバーから始まる。
    await page
      .getByRole("table", { name: "Items" })
      .getByRole("link", { name: `Play from ${groupName}` })
      .click();
    await expectPlaylistVideo(page, member1, 2);
    await expectNotice(page, member2);
    await overlay(page).getByRole("button", { name: "Play now" }).click();
    await expectPlaylistVideo(page, member2, 2);
    await expectNotice(page, member3);
    await overlay(page).getByRole("button", { name: "Play now" }).click();
    await expectPlaylistVideo(page, member3, 2);
    // グループの後は前の A ではなく、後ろの B を予告する。
    await expectNotice(page, titleB);

    // 予告を取り消すと、その場に留まり、「Play next」と「Watch again」の層になる（6）。
    await overlay(page).getByRole("button", { name: "Cancel" }).click();
    const url = page.url();
    await expect(overlay(page).getByRole("button", { name: "Play next" })).toBeVisible();
    await expect(
      overlay(page).getByRole("button", { name: "Watch again" }),
    ).toBeVisible();
    await page.waitForTimeout(6_000);
    expect(page.url()).toBe(url);
    await expect(overlay(page).getByRole("button", { name: "Play now" })).toHaveCount(0);
  });

  test("ライブラリから開いたメンバーは、プレイリストに入っていてもグループの中で進む", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch);
    // グループのカードはグループのページを開き、そのメンバーは開いたグループを文脈に持つ
    // （specs/048-groups/research.md R-8・R-9）。
    await page
      .locator("article[data-group-root]")
      .filter({ hasText: groupName })
      .getByRole("link")
      .first()
      .click();
    await expect(page).toHaveURL(/\/groups\/\d+$/);
    await page
      .locator("article[data-video-id]")
      .filter({ has: page.locator("h3", { hasText: member1 }) })
      .getByRole("link")
      .first()
      .click();
    await expect(page).toHaveURL(
      new RegExp(`/videos/${String(video(member1).id)}\\?group=\\d+$`),
    );
    await expect(page.getByRole("link", { name: /^Playlist "/ })).toHaveCount(0);
    await page.waitForFunction(() => {
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      return element !== null && element.readyState >= 1;
    });
    await page.locator(".vjs-big-play-button").click();

    // グループの自動再生のまま、次のメンバーを予告して進む（8）。
    await expectNotice(page, member2);
    await expect(page).toHaveURL(
      new RegExp(`/videos/${String(video(member2).id)}\\?group=\\d+$`),
      { timeout: 20_000 },
    );
    await expect(page.getByRole("link", { name: /^Playlist "/ })).toHaveCount(0);
  });

  test("ゲストには入口が無く、プレイリストの画面はログイン画面になる", async ({
    browser,
    request,
  }) => {
    // ゲストに見えるカードを用意するため、A を公開にする。フォルダは afterAll で外す。
    const published = await request.put("/api/video-visibility", {
      headers: mutationHeaders,
      data: { videoIds: [video(titleA).id], public: true },
    });
    expect(published.status()).toBe(200);

    const context = await guestContext(browser);
    const page = await context.newPage();
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch);
    const card = videoCard(page, titleA);
    await expect(card).toBeVisible();
    await card.hover();
    await expect(page.getByRole("button", { name: /to a playlist$/ })).toHaveCount(0);
    await expect(
      page
        .getByRole("complementary", { name: "Main navigation" })
        .getByRole("link", { name: "Playlists" }),
    ).toHaveCount(0);

    // 画面を直接開くとログイン画面へ送られ、中身は出ない（9）。
    for (const path of ["/playlists", `/playlists/${String(playlistId(tonight))}`]) {
      await page.goto(path);
      await expect(page).toHaveURL(`/login?next=${encodeURIComponent(path)}`);
      await expect(page.getByText(tonight)).toHaveCount(0);
    }
    await context.close();
  });
});

import { mkdir } from "node:fs/promises";
import path from "node:path";

import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

import { ownerStorageState } from "./owner-account";

// スマートフォルダ（specs/052-smart-folders、親 #1139、子 #1182）の流れを、実サーバーと実メディアに
// 通す。動画は run-e2e.mjs が generateSmartFoldersFixtures で作る 20 秒の 7 本で、名前は
// media-fixtures.mjs の SMART_FOLDER_* と同じ。条件の検証、入れ子の条件、窓の入力の規則、
// URL の運び方、グループの見せ方、子の条件、ゲストの見え方、名前の変更と削除はドメイン・ストア・
// ページのテストが持つので、ここでは重ねない。残すのは、ライブラリから保存して実サーバーの一致を
// 見る流れ、別のタブの変更が画面の知らせ（smartFolder の画面イベント）で届く流れ、実メディアの
// 再生でプレイリストの中のフォルダを進む流れ、画面写真である
// （docs/design-docs/web-testing.md の Test levels）。テストの名前の番号は親 Issue の受け入れ
// 条件で、「画面」は specs/052-smart-folders/ui-design.md の Review criteria の番号である。
//
// タグ「猫」は 猫A・猫B・両方1・両方2・片方1 に付ける。フォルダ「タグ: 猫」とその子「未視聴」は
// beforeAll が API で作る。サーバーとデータベースはほかの e2e と共有するので、作ったフォルダ・
// タグ・プレイリストと登録フォルダは終わりに消し、公開も戻す。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
  tags?: { id: number; name: string }[];
}

interface Named {
  id: number;
  name: string;
}

interface FolderItem {
  kind: "video" | "group" | "smartFolder";
  video?: { id: number };
  group?: { videoIds: number[] };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const screenshotDir = process.env.MDM_E2E_SCREENSHOT_DIR;
// media-fixtures.mjs の SMART_FOLDER_SINGLES・SMART_FOLDER_GROUPS と同じ名前。
const query = "スマート確認";
const catA = "スマート確認の猫A";
const catB = "スマート確認の猫B";
const dog = "スマート確認の犬";
const both1 = "スマート確認の両方1";
const both2 = "スマート確認の両方2";
const half1 = "スマート確認の片方1";
const half2 = "スマート確認の片方2";
const bothGroup = "両方に猫のグループ";
const allTitles = [catA, catB, dog, both1, both2, half1, half2];
const catTitles = [catA, catB, both1, both2, half1];
// テストが作るタグ・フォルダ・プレイリスト。前の試行の残りも名前で探して消す。
const catTag = "猫";
const catFolder = "タグ: 猫";
const unwatchedChild = "未視聴";
const savedFolder = "猫の未視聴";
const folderNames = [catFolder, unwatchedChild, savedFolder];
const playlistName = "スマートフォルダの再生";

const videos = new Map<string, Video>();
const folderIds = new Map<string, number>();
const tagIds = new Map<string, number>();
let mediaFolder: MediaFolder | undefined;

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function folderId(name: string): number {
  const found = folderIds.get(name);
  if (found === undefined) throw new Error(`smart folder not created: ${name}`);
  return found;
}

function tagId(name: string): number {
  const found = tagIds.get(name);
  if (found === undefined) throw new Error(`tag not created: ${name}`);
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

/** readVideos は API から、この e2e の動画を題名で引けるように読む。 */
async function readVideos(request: APIRequestContext): Promise<Map<string, Video>> {
  const response = await request.get(
    `/api/videos?limit=200&query=${encodeURIComponent(query)}`,
  );
  expect(response.status()).toBe(200);
  const { items } = (await response.json()) as { items: Video[] };
  return new Map(items.map((item) => [item.title, item]));
}

/** setWatched は動画を見終えた（true）か、位置 0 の未視聴（false）にする。 */
async function setWatched(request: APIRequestContext, title: string, watched: boolean) {
  const response = await request.put(`/api/videos/${String(video(title).id)}/progress`, {
    headers: mutationHeaders,
    data: { positionMs: watched ? 20_000 : 0 },
  });
  expect(response.status()).toBe(200);
  expect(((await response.json()) as { completed: boolean }).completed).toBe(watched);
}

/** tagVideos は titles の動画に名前 tag のタグを付ける。無ければ作る。 */
async function tagVideos(
  request: APIRequestContext,
  titles: readonly string[],
  tag: string,
) {
  const response = await request.post("/api/video-tags", {
    headers: mutationHeaders,
    data: {
      videoIds: titles.map((title) => video(title).id),
      action: "add",
      tag: { name: tag },
    },
  });
  expect(response.ok()).toBe(true);
}

/** readTagIds は動画に付いたタグから、名前で id を引けるようにする。 */
async function readTagIds(request: APIRequestContext, title: string) {
  const response = await request.get(`/api/videos/${String(video(title).id)}`);
  expect(response.status()).toBe(200);
  for (const tag of ((await response.json()) as Video).tags ?? []) {
    tagIds.set(tag.name, tag.id);
  }
}

/** createFolder は API でスマートフォルダを作り、名前で id を引けるようにする。 */
async function createFolder(
  request: APIRequestContext,
  name: string,
  data: { rule: Record<string, unknown>; parentId?: number },
) {
  const response = await request.post("/api/smart-folders", {
    headers: mutationHeaders,
    data: { name, ...data },
  });
  expect(response.status()).toBe(201);
  folderIds.set(name, ((await response.json()) as Named).id);
}

/** listFolders は所有者のスマートフォルダをすべて読む。 */
async function listFolders(request: APIRequestContext): Promise<Named[]> {
  const response = await request.get("/api/smart-folders");
  expect(response.status()).toBe(200);
  return ((await response.json()) as { smartFolders: Named[] }).smartFolders;
}

/** deleteOwnFolders は、この e2e の名前のフォルダを消す（子は親と一緒に消える）。 */
async function deleteOwnFolders(request: APIRequestContext) {
  for (const folder of await listFolders(request)) {
    if (!folderNames.includes(folder.name)) continue;
    const response = await request.delete(`/api/smart-folders/${String(folder.id)}`, {
      headers: mutationHeaders,
    });
    expect([204, 404]).toContain(response.status());
  }
}

/** deleteOwnTags は、この e2e の名前のタグを消す。 */
async function deleteOwnTags(request: APIRequestContext) {
  const response = await request.get(`/api/tags?q=${encodeURIComponent(catTag)}`);
  expect(response.ok()).toBe(true);
  const { items } = (await response.json()) as { items: Named[] };
  for (const tag of items.filter((item) => item.name === catTag)) {
    await request.delete(`/api/tags/${String(tag.id)}`, { headers: mutationHeaders });
  }
}

/** deleteOwnPlaylists は、この e2e の名前のプレイリストを消す。 */
async function deleteOwnPlaylists(request: APIRequestContext) {
  const response = await request.get("/api/playlists");
  expect(response.status()).toBe(200);
  for (const item of ((await response.json()) as { items: Named[] }).items) {
    if (item.name !== playlistName) continue;
    await request.delete(`/api/playlists/${String(item.id)}`, {
      headers: mutationHeaders,
    });
  }
}

/** playOrder は API から、フォルダの再生の順（グループはメンバーに開いた動画の id）を読む。 */
async function playOrder(request: APIRequestContext, id: number): Promise<number[]> {
  const response = await request.get(`/api/smart-folders/${String(id)}`);
  expect(response.status()).toBe(200);
  const { items } = (await response.json()) as { items: FolderItem[] };
  return items.flatMap((item) => {
    if (item.kind === "video" && item.video !== undefined) return [item.video.id];
    if (item.kind === "group" && item.group !== undefined) return item.group.videoIds;
    return [];
  });
}

/** firstTwo は再生の順の 1 本目と 2 本目の題名である。 */
function firstTwo(order: readonly string[]): [string, string] {
  const [first, second] = order;
  if (first === undefined || second === undefined)
    throw new Error("play order too short");
  return [first, second];
}

function titleOf(id: number): string {
  for (const [title, item] of videos) if (item.id === id) return title;
  throw new Error(`unknown video id ${String(id)}`);
}

/** videoCard は一覧の、title の動画のカードである。 */
function videoCard(page: Page, title: string): Locator {
  return page.locator("article[data-video-id]").filter({
    has: page.locator("h3", { hasText: new RegExp(`^${escapeRegExp(title)}$`) }),
  });
}

/** groupCard は一覧の、name のグループのカードである。 */
function groupCard(page: Page, name: string): Locator {
  return page.locator("article[data-group-id]").filter({
    has: page.getByRole("link", {
      name: new RegExp(`^${escapeRegExp(name)}, group of `),
    }),
  });
}

/** folderCard は一覧の、id のスマートフォルダのカードである。 */
function folderCard(page: Page, id: number): Locator {
  return page.locator(`article[data-smart-folder-id="${String(id)}"]`);
}

/**
 * listedItems は一覧の項目を並びのまま返す。動画のカードは題名、グループとスマートフォルダの
 * カードはリンクの読み上げ名（名前と本数）で表す。
 */
async function listedItems(page: Page): Promise<string[]> {
  return page
    .locator(
      "article[data-video-id], article[data-group-id], article[data-smart-folder-id]",
    )
    .evaluateAll((articles) =>
      articles.map((article) =>
        article.hasAttribute("data-video-id")
          ? (article.querySelector("h3")?.textContent ?? "")
          : (article.querySelector("a[aria-label]")?.getAttribute("aria-label") ?? ""),
      ),
    );
}

/** toast は画面の下に出た知らせのうち、text のものである。 */
function toast(page: Page, text: string): Locator {
  return page
    .getByRole("region", { name: /Notifications/ })
    .getByText(text, { exact: true });
}

/** openFolder はフォルダのページを開き、見出しが name になるのを待つ。 */
async function openFolder(page: Page, name: string) {
  await page.goto(`/smart-folders/${String(folderId(name))}`);
  await expect(page.getByRole("heading", { level: 1, name, exact: true })).toBeVisible();
}

/** saveFromLibrary はライブラリの「Save as smart folder」の窓で name を打ち、Enter で作る。 */
async function saveFromLibrary(page: Page, name: string): Promise<number> {
  await page.getByRole("button", { name: "Save as smart folder" }).click();
  const dialog = page.getByRole("dialog", { name: "New smart folder" });
  const field = dialog.getByRole("textbox", { name: "Name" });
  await expect(field).toBeFocused();
  await field.fill(name);
  const created = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === "/api/smart-folders",
  );
  await page.keyboard.press("Enter");
  const response = await created;
  expect(response.status()).toBe(201);
  await expect(dialog).toBeHidden();
  const id = ((await response.json()) as Named).id;
  folderIds.set(name, id);
  return id;
}

/** folderLine は動画のページの題名の上の、スマートフォルダの何本目かを示す行である。 */
function folderLine(page: Page, name: string, position: number, count: number): Locator {
  return page.getByRole("link", {
    name: `Smart folder "${name}", video ${String(position)} of ${String(count)}. Open the smart folder`,
  });
}

/**
 * pressArrow はプレイヤーの端の前後のつまみ（name）を押す。つまみは操作バーと同じく、
 * プレイヤーを指している間だけ見えるので、先に指す。
 */
async function pressArrow(page: Page, name: RegExp) {
  await page.locator(".video-js").hover();
  await page.getByRole("button", { name }).click();
}

/** playlistLine は動画のページの題名の上の、プレイリストの何番目の項目かを示す行である。 */
function playlistLine(page: Page, position: number): Locator {
  return page.getByRole("link", {
    name: `Playlist "${playlistName}", item ${String(position)} of 2. Open the playlist`,
  });
}

/** itemLinks はプレイリストの画面の項目の、並びのままの読み上げ名（「Play from …」）である。 */
async function itemLinks(rows: Locator): Promise<string[]> {
  return rows
    .getByRole("link")
    .evaluateAll((links) => links.map((link) => link.getAttribute("aria-label") ?? ""));
}

/**
 * scrollToCard は一覧を下へ送り、card が描かれて見えるところまで来るのを待つ。一覧の格子は
 * 画面の近くのカードだけを描くので、狭い幅では下のカードが DOM に無い。
 */
async function scrollToCard(page: Page, card: Locator) {
  await expect
    .poll(async () => {
      if ((await card.count()) > 0) return true;
      await page.mouse.wheel(0, 600);
      return false;
    })
    .toBe(true);
  await card.scrollIntoViewIfNeeded();
  await expect(card.getByRole("link", { name: /smart folder of/ })).toBeVisible();
}

test.describe.serial("smart folders", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(240_000);
    const root = process.env.MDM_E2E_SMART_FOLDERS_MEDIA_DIR;
    if (root === undefined) {
      throw new Error("MDM_E2E_SMART_FOLDERS_MEDIA_DIR is not configured");
    }
    // CI のリトライに備え、前の試行が残したものを先に消す。
    await deleteOwnFolders(request);
    await deleteOwnPlaylists(request);
    await deleteOwnTags(request);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    mediaFolder = (await created.json()) as MediaFolder;
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);
    await expect
      .poll(
        async () => {
          const found = await readVideos(request);
          return allTitles.every((title) => {
            const item = found.get(title);
            return item?.probeState === "done" && item.thumbnailState !== "pending";
          });
        },
        { timeout: 120_000 },
      )
      .toBe(true);
    for (const [title, item] of await readVideos(request)) videos.set(title, item);

    // 視聴の記録は動画の中身に結び付き、登録フォルダを外しても残るので、どれも未視聴に戻す。
    for (const title of allTitles) await setWatched(request, title, false);
    await tagVideos(request, catTitles, catTag);
    await readTagIds(request, catA);
    await createFolder(request, catFolder, { rule: { tags: [tagId(catTag)] } });
    await createFolder(request, unwatchedChild, {
      rule: { watch: "unwatched" },
      parentId: folderId(catFolder),
    });
  });

  test.afterAll(async ({ request }) => {
    if (videos.size > 0) {
      await request.put("/api/video-visibility", {
        headers: mutationHeaders,
        data: { videoIds: [...videos.values()].map((item) => item.id), public: false },
      });
    }
    await deleteOwnFolders(request);
    await deleteOwnPlaylists(request);
    await deleteOwnTags(request);
    if (mediaFolder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(mediaFolder.id)}?version=${String(mediaFolder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.ok()).toBe(true);
    }
  });

  test("5: 猫と未視聴で絞ったライブラリから保存したフォルダは、ライブラリが見せていたものを同じ順に見せる（画面 2・8）", async ({
    page,
  }) => {
    const filtered = `/library?tag=${String(tagId(catTag))}&watch=unwatched`;
    await page.goto(filtered);
    await expect(videoCard(page, catA)).toBeVisible();
    await expect(groupCard(page, bothGroup)).toBeVisible();
    const shown = await listedItems(page);
    expect(shown).toHaveLength(4);

    // 窓は絞り込みと同じ値と形で開く（画面 2）。
    await page.getByRole("button", { name: "Save as smart folder" }).click();
    const dialog = page.getByRole("dialog", { name: "New smart folder" });
    await expect(
      dialog.getByRole("radiogroup", { name: "Watch status" }).getByRole("radio", {
        name: "Unwatched",
      }),
    ).toBeChecked();
    await expect(
      dialog.getByRole("button", { name: `Remove the condition ${catTag}` }),
    ).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toBeHidden();

    const id = await saveFromLibrary(page, savedFolder);
    await expect(toast(page, `Created "${savedFolder}"`)).toBeVisible();
    // 絞り込んだ一覧はそのままで、フォルダのカードは並ばない（R-7）。
    await expect(page).toHaveURL(filtered);
    expect(await listedItems(page)).toEqual(shown);
    await expect(folderCard(page, id)).toHaveCount(0);

    await page
      .getByRole("region", { name: /Notifications/ })
      .getByRole("button", { name: "Open" })
      .click();
    await expect(page).toHaveURL(`/smart-folders/${String(id)}`);
    await expect(
      page.getByRole("heading", { level: 1, name: savedFolder, exact: true }),
    ).toBeVisible();
    await expect.poll(() => listedItems(page)).toEqual(shown);

    // 新しいカードは絞り込まないライブラリに立つ。
    await page.goto("/library");
    await expect(
      folderCard(page, id).getByRole("link", {
        name: `${savedFolder}, smart folder of 5 videos`,
      }),
    ).toBeVisible();
  });

  test("1: 別のタブで猫を付けると、開いた タグ: 猫 のフォルダに再読み込みなしで入り、外すと出る。その間も動画はライブラリにある（画面 6）", async ({
    page,
    context,
  }) => {
    const id = folderId(catFolder);
    await openFolder(page, catFolder);
    await expect(videoCard(page, catA)).toBeVisible();
    await expect(videoCard(page, dog)).toHaveCount(0);
    // 再読み込みすると消える印。最後まで残っていれば、ページは読み直されていない。
    await page.evaluate(() => {
      (window as unknown as { e2eNoReload: boolean }).e2eNoReload = true;
    });

    const library = await context.newPage();
    await library.goto(`/library?q=${encodeURIComponent(dog)}`);
    await expect(videoCard(library, dog)).toBeVisible();
    const cards = await context.newPage();
    await cards.goto("/library");
    const card = folderCard(cards, id);
    await expect(
      card.getByRole("link", { name: `${catFolder}, smart folder of 5 videos` }),
    ).toBeVisible();

    const tagger = await context.newPage();
    await tagger.goto(`/videos/${String(video(dog).id)}`);
    await expect(tagger.getByRole("heading", { level: 1 })).toHaveText(dog);
    const input = tagger.getByRole("combobox", { name: "Add tag" });
    await input.click();
    await input.fill(catTag);
    await tagger.keyboard.press("Enter");
    await expect(tagger.locator(`[title="${catTag}"]`)).toBeVisible();

    await expect(videoCard(page, dog)).toBeVisible();
    await expect(
      card.getByRole("link", { name: `${catFolder}, smart folder of 6 videos` }),
    ).toBeVisible();
    await library.reload();
    await expect(videoCard(library, dog)).toBeVisible();

    await tagger
      .getByRole("button", { name: `Remove ${catTag} from this video` })
      .click();
    await expect(tagger.locator(`[title="${catTag}"]`)).toHaveCount(0);

    await expect(videoCard(page, dog)).toHaveCount(0);
    await expect(videoCard(page, catA)).toBeVisible();
    await expect(
      card.getByRole("link", { name: `${catFolder}, smart folder of 5 videos` }),
    ).toBeVisible();
    await library.reload();
    await expect(videoCard(library, dog)).toBeVisible();
    expect(
      await page.evaluate(
        () => (window as unknown as { e2eNoReload?: boolean }).e2eNoReload,
      ),
    ).toBe(true);
  });

  test("フォルダをプレイリストに足すと 1 つの行になり、その行からフォルダの順に再生する（画面 13）", async ({
    page,
    request,
  }) => {
    const id = folderId(catFolder);
    await page.goto("/library");
    const card = folderCard(page, id);
    await card.hover();
    await card
      .getByRole("button", { name: `Add smart folder "${catFolder}" to a playlist` })
      .click();
    const picker = page.getByRole("dialog", { name: "Add to playlist" });
    const field = picker.getByRole("combobox", { name: "Search or create a playlist" });
    await expect(field).toBeFocused();
    await field.fill(playlistName);
    await picker.getByRole("option", { name: `Create "${playlistName}"` }).click();
    await expect(picker).toBeHidden();
    await expect(toast(page, `Added 1 item to "${playlistName}"`)).toBeVisible();

    const list = await request.get("/api/playlists");
    const playlist = ((await list.json()) as { items: Named[] }).items.find(
      (item) => item.name === playlistName,
    );
    if (playlist === undefined) throw new Error("playlist not created");
    // フォルダの後ろに動画の項目を 1 つ足す。項目の矢印がフォルダの外へ出ることを見る。
    const added = await request.post(`/api/playlists/${String(playlist.id)}/items`, {
      headers: mutationHeaders,
      data: { videoIds: [video(dog).id] },
    });
    expect(added.ok()).toBe(true);
    // 再生はフォルダの最初の見終わっていない動画から始まるので、どれも未視聴に戻す。
    for (const title of catTitles) await setWatched(request, title, false);

    await page.goto(`/playlists/${String(playlist.id)}`);
    await expect(
      page.getByRole("heading", { level: 1, name: playlistName, exact: true }),
    ).toBeVisible();
    const rows = page.getByRole("table", { name: "Items" });
    expect(await itemLinks(rows)).toEqual([`Play from ${catFolder}`, `Play from ${dog}`]);
    await expect(rows.getByRole("row").filter({ hasText: catFolder })).toContainText(
      "5 videos",
    );

    const order = (await playOrder(request, id)).map(titleOf);
    const [first, second] = firstTwo(order);
    const inFolder = (title: string) =>
      new RegExp(
        `/videos/${String(video(title).id)}\\?playlist=${String(playlist.id)}&item=\\d+&smartFolder=${String(id)}$`,
      );
    await rows.getByRole("link", { name: `Play from ${catFolder}` }).click();
    await expect(page).toHaveURL(inFolder(first));
    await expect(playlistLine(page, 1)).toBeVisible();
    await expect(folderLine(page, catFolder, 1, order.length)).toBeVisible();

    // フォルダの動画を終えると、予告はフォルダの中の 2 本目を名指し、進んでもフォルダを保つ。
    await page.locator("video").evaluate((element: HTMLVideoElement) => {
      element.currentTime = Math.max(element.duration - 0.5, 0);
    });
    const layer = page.locator("[data-overlay-layer]");
    await expect(layer.getByRole("button", { name: "Play now" })).toBeVisible({
      timeout: 20_000,
    });
    await expect(
      layer.getByRole("link", { name: new RegExp(`^${escapeRegExp(second)} `) }),
    ).toBeVisible();
    await layer.getByRole("button", { name: "Play now" }).click();
    await expect(page).toHaveURL(inFolder(second));
    await expect(playlistLine(page, 1)).toBeVisible();
    await expect(folderLine(page, catFolder, 2, order.length)).toBeVisible();

    // 項目の矢印は次の項目（動画）へ移り、その URL はフォルダを運ばない。
    await pressArrow(page, /^Next item/);
    await expect(page).toHaveURL(
      new RegExp(
        `/videos/${String(video(dog).id)}\\?playlist=${String(playlist.id)}&item=\\d+$`,
      ),
    );
    await expect(playlistLine(page, 2)).toBeVisible();
    await expect(page.getByRole("link", { name: /^Smart folder "/ })).toHaveCount(0);
  });

  test("画面写真（MDM_E2E_SCREENSHOT_DIR があるときだけ）", async ({ browser }) => {
    test.skip(screenshotDir === undefined, "MDM_E2E_SCREENSHOT_DIR is not set");
    if (screenshotDir === undefined) return;
    test.setTimeout(180_000);
    await mkdir(screenshotDir, { recursive: true });
    for (const width of [360, 768, 1280]) {
      const context = await browser.newContext({
        storageState: ownerStorageState,
        viewport: { width, height: 900 },
      });
      const page = await context.newPage();
      const shot = (name: string, fullPage = false) =>
        page.screenshot({
          path: path.join(screenshotDir, `smart-folders-${name}-${String(width)}.png`),
          fullPage,
        });
      try {
        await page.goto("/library");
        await scrollToCard(page, folderCard(page, folderId(catFolder)));
        await shot("library");

        await page.goto(`/library?tag=${String(tagId(catTag))}&watch=unwatched`);
        await expect(videoCard(page, catA)).toBeVisible();
        await shot("library-filtered");
        await page.getByRole("button", { name: "Save as smart folder" }).click();
        await expect(
          page.getByRole("dialog", { name: "New smart folder" }),
        ).toBeVisible();
        await shot("dialog");
        await page.keyboard.press("Escape");

        await openFolder(page, unwatchedChild);
        await expect(videoCard(page, catA)).toBeVisible();
        await shot("child-page", true);
        await openFolder(page, catFolder);
        await expect(groupCard(page, bothGroup)).toBeVisible();
        await shot("parent-page", true);

        // 表示の切り替えは広い幅でだけ上のバーに出る。リスト表示はその幅で撮る。
        if (width === 1280) {
          const views = page.getByRole("radiogroup", { name: "View" });
          await views.getByRole("radio", { name: "List" }).click();
          await expect(page.getByRole("table")).toBeVisible();
          await shot("parent-list");
          await views.getByRole("radio", { name: "Grid" }).click();
        }

        await page.goto(
          `/videos/${String(video(half1).id)}?smartFolder=${String(folderId(catFolder))}`,
        );
        await expect(page.getByRole("link", { name: /^Smart folder "/ })).toBeVisible();
        await shot("video", true);

        await page.goto("/playlists");
        await page
          .locator("article[data-playlist-id]")
          .getByRole("link", { name: new RegExp(`^${escapeRegExp(playlistName)}, `) })
          .click();
        await expect(page.getByRole("table", { name: "Items" })).toBeVisible();
        await shot("playlist");
      } finally {
        await context.close();
      }
    }
  });
});

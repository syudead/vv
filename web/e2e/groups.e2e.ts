import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";
import { copyFileSync, existsSync, mkdirSync, renameSync, rmSync } from "node:fs";
import path from "node:path";

// 任意のグループ（specs/048-groups、親 #1040、子 #1059）のうち、本物のスキャナーを通る
// 再スキャンに頼るものを、実サーバーと実メディアに通す。動画は run-e2e.mjs が
// generateGroupsFixtures で作る 3 秒の動画で、名前は media-fixtures.mjs の GROUPS_* と同じ。
// 画面からグループを作ってサーバーが残しカードに出る流れは 1 本だけ残し、ほかの受け入れ条件
// （2 つのグループ・入れ子・並べ替え・お気に入りとプレイリスト）は Vitest
// （src/library/LibraryGroups・LibraryFavorites・LibraryPlaylists、src/groups/GroupPage、
// src/player/VideoPage）と Go のストア・ハンドラーのテストが確かめる
// （docs/design-docs/web-testing.md の Test levels）。括弧の番号は親 Issue の受け入れ条件である。
//
// テストは順に状態を引き継ぐ。「三つのフォルダ」（動画1〜3）は 1 で作り 7 で使う。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
  folder?: { rootId: number; path: string };
}

interface GroupSummary {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の GROUPS_SPREAD・GROUPS_SINGLES・GROUPS_AUTO* ・GROUPS_DISSOLVED*・
// GROUPS_SPARE と同じ名前。
const query = "グループ確認";
const video1 = "グループ確認の動画1";
const video2 = "グループ確認の動画2";
const video3 = "グループ確認の動画3";
const singleB = "グループ確認の単体B";
const autoFolder = "自動のグループ";
const auto1 = "グループ確認の自動1";
const auto2 = "グループ確認の自動2";
const auto3 = "グループ確認の自動3";
const auto4 = "グループ確認の自動4";
const dissolvedFolder = "解散するグループ";
const dissolved1 = "グループ確認の解散1";
const dissolved2 = "グループ確認の解散2";
const allTitles = [
  video1,
  video2,
  video3,
  singleB,
  auto1,
  auto2,
  auto3,
  dissolved1,
  dissolved2,
];
// テストが作る、または名前を変えるグループ。
const spread = "三つのフォルダ";
const renamedAuto = "名前を変えた自動グループ";
const groupNames = [spread, renamedAuto, autoFolder, dissolvedFolder];
const librarySearch = `/library?q=${encodeURIComponent(query)}`;
let folder: MediaFolder | undefined;

function mediaRoot(): string {
  const root = process.env.MDM_E2E_GROUPS_MEDIA_DIR;
  if (root === undefined) throw new Error("MDM_E2E_GROUPS_MEDIA_DIR is not configured");
  return root;
}

function spareRoot(): string {
  const root = process.env.MDM_E2E_GROUPS_SPARE_DIR;
  if (root === undefined) throw new Error("MDM_E2E_GROUPS_SPARE_DIR is not configured");
  return root;
}

/** 動画3 の元の場所と、7 で移す先（登録フォルダの下の、ほかの動画の無いフォルダ）。 */
function movedPaths() {
  const root = mediaRoot();
  return {
    from: path.join(root, "フォルダ3", `${video3}.mp4`),
    to: path.join(root, "移動先", `${video3}.mp4`),
  };
}

/** 5 で再スキャンの前に自動グループのフォルダへ写す 1 本の、写した先。 */
function copiedSpare(): string {
  return path.join(mediaRoot(), autoFolder, `${auto4}.mp4`);
}

/**
 * resetFiles は、前の試行が動かしたファイルを元に戻す。fixtures は実行の初めに 1 度だけ
 * 作られるので、CI のリトライは前の試行が移した動画3 と写した自動4 をそのまま見る。
 */
function resetFiles() {
  const { from, to } = movedPaths();
  if (existsSync(to)) renameSync(to, from);
  rmSync(copiedSpare(), { force: true });
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

/** rescan はスキャンを始めて終わるまで待つ。グループの同期はスキャンの中で走る。 */
async function rescan(request: APIRequestContext) {
  const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
  expect(scan.status()).toBe(202);
  await waitForScan(request, ((await scan.json()) as { id: number }).id);
}

/** readVideos は API から、検索に当たる動画を題名で引けるように読む。 */
async function readVideos(request: APIRequestContext): Promise<Map<string, Video>> {
  const response = await request.get(
    `/api/videos?limit=200&query=${encodeURIComponent(query)}`,
  );
  expect(response.status()).toBe(200);
  const { items } = (await response.json()) as { items: Video[] };
  return new Map(items.map((item) => [item.title, item]));
}

/** waitForVideos は titles の動画が読み込まれ、サムネイルの生成を待たなくなるまで待つ。 */
async function waitForVideos(request: APIRequestContext, titles: readonly string[]) {
  await expect
    .poll(
      async () => {
        const found = await readVideos(request);
        return titles.every((title) => {
          const item = found.get(title);
          return item?.probeState === "done" && item.thumbnailState !== "pending";
        });
      },
      { timeout: 120_000 },
    )
    .toBe(true);
}

/**
 * restoreAutoGrouping はフォルダ relativePath（登録フォルダからの相対）の例外を外して自動に
 * 戻す。自動グループの解散は、そのフォルダの例外 ungroup をパスに書き、登録フォルダを外しても
 * 消えない。前の試行の 6 と afterAll が書いた例外が残ると、CI のリトライの再スキャンで自動
 * グループが作られないので、試行の初めに戻す。戻すと同じ取引で自動グループが作り直される。
 */
async function restoreAutoGrouping(
  request: APIRequestContext,
  rootId: number,
  relativePath: string,
) {
  const response = await request.put(
    `/api/folders/${String(rootId)}/grouping?path=${encodeURIComponent(relativePath)}`,
    { headers: mutationHeaders, data: { mode: "auto" } },
  );
  expect(response.status()).toBe(200);
}

/** groupsNamed は API から、name という名前のグループをすべて読む。 */
async function groupsNamed(request: APIRequestContext, name: string) {
  const response = await request.get(
    `/api/groups?limit=200&q=${encodeURIComponent(name)}`,
  );
  expect(response.status()).toBe(200);
  return ((await response.json()) as { groups: GroupSummary[] }).groups.filter(
    (group) => group.name === name,
  );
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

/** toast は画面の下に出た知らせのうち、text のものである。 */
function toast(page: Page, text: string): Locator {
  return page
    .getByRole("region", { name: /Notifications/ })
    .getByText(text, { exact: true });
}

/**
 * openPicker はカードの角の「Add to group」を押してピッカーを開き、name を打つ。カードの
 * 印は指したときに出るので、先に指す。
 */
async function openPicker(card: Locator, trigger: string, name: string) {
  const page = card.page();
  await card.hover();
  await card.getByRole("button", { name: trigger }).click();
  const picker = page.getByRole("dialog", { name: "Add to group" });
  const field = picker.getByRole("combobox", { name: "Search or create a group" });
  await expect(field).toBeFocused();
  await field.fill(name);
  return picker;
}

/**
 * addFromCard はカードの「Add to group」から name のグループへ足す。`create` なら
 * 「Create "{name}"」の行で作り、そうでなければ一覧のその行を押す。知らせが出るのを待つ。
 */
async function addFromCard(card: Locator, trigger: string, name: string, create = false) {
  const page = card.page();
  const picker = await openPicker(card, trigger, name);
  const row = create ? `Create "${name}"` : name;
  await picker.getByRole("option", { name: row, exact: true }).click();
  await expect(picker).toBeHidden();
  await expect(
    toast(page, create ? `Created "${name}"` : `Added to "${name}"`).first(),
  ).toBeVisible();
}

/** openGroup はライブラリの検索から name のグループのカードを押し、そのページを開く。 */
async function openGroup(page: Page, name: string) {
  await page.goto(librarySearch);
  await groupCard(page, name)
    .getByRole("link", { name: new RegExp(`^${escapeRegExp(name)}, group of `) })
    .click();
  await expect(page).toHaveURL(/\/groups\/\d+$/);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
}

/** showList はグループのページの本体をリスト表示（並べ替えの表示）にする。 */
async function showList(page: Page) {
  await page
    .getByRole("radiogroup", { name: "View" })
    .getByRole("radio", { name: "List" })
    .click();
  await expect(page.getByRole("table", { name: "Items" })).toBeVisible();
}

/** itemTitles はリスト表示の項目の、並びのままの題名である。 */
async function itemTitles(page: Page): Promise<string[]> {
  const links = page.getByRole("table", { name: "Items" }).getByRole("link");
  return links.evaluateAll((elements) =>
    elements.map((element) => element.getAttribute("title") ?? ""),
  );
}

/** headerCount はグループのページの見出しの件数（「3 videos · 0:09」）である。 */
function headerCount(page: Page): Locator {
  return page.getByText(/^\d+ videos? · /);
}

test.describe.serial("groups", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    resetFiles();
    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaRoot() },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    await rescan(request);
    await waitForVideos(request, allTitles);
    for (const relativePath of [autoFolder, dissolvedFolder]) {
      await restoreAutoGrouping(request, folder.id, relativePath);
    }
  });

  test.afterAll(async ({ request }) => {
    // 手で作ったグループは登録フォルダを外しても残り、CI のリトライで同じ動画が戻ると
    // また出てくる。名前で探して解散する。
    for (const name of groupNames) {
      for (const group of await groupsNamed(request, name)) {
        const removed = await request.delete(`/api/groups/${String(group.id)}`, {
          headers: mutationHeaders,
        });
        expect([204, 404]).toContain(removed.status());
      }
    }
    if (folder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
    resetFiles();
  });

  test("3 つのフォルダの 3 本を選択バーからグループにすると、1 枚のカードになる", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch);
    for (const title of [video1, video2, video3]) {
      await page.getByRole("checkbox", { name: `Select "${title}"` }).click();
    }
    await page
      .getByRole("region", { name: "Selection actions" })
      .getByRole("button", { name: "New group" })
      .click();
    const dialog = page.getByRole("dialog", { name: "New group" });
    await expect(dialog).toContainText("3 videos become one group");
    await dialog.getByRole("textbox", { name: "Name" }).fill(spread);
    await dialog.getByRole("button", { name: "Create" }).click();
    await expect(dialog).toBeHidden();
    await expect(toast(page, `Created "${spread}"`)).toBeVisible();

    // グループのカードが 1 枚出て、3 本は単体のカードとしては出ない（1）。
    await expect(groupCard(page, spread)).toHaveCount(1);
    await expect(
      groupCard(page, spread).getByRole("link", {
        name: `${spread}, group of 3 videos`,
      }),
    ).toBeVisible();
    for (const title of [video1, video2, video3]) {
      await expect(videoCard(page, title)).toHaveCount(0);
    }
  });

  test("自動グループに足した・外した・名前を変えた結果は再スキャン後も残り、新しいファイルが加わる", async ({
    page,
    request,
  }) => {
    test.setTimeout(150_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    // 別のフォルダ（登録フォルダの直下）の単体B を、ライブラリのカードの「Add to group」で
    // 自動グループに足す。
    await page.goto(librarySearch);
    await addFromCard(
      videoCard(page, singleB),
      `Add "${singleB}" to a group`,
      autoFolder,
    );

    // 自動1 を外し、名前を変える。
    await openGroup(page, autoFolder);
    await expect(page.getByText("From the folder", { exact: true })).toBeVisible();
    await showList(page);
    await expect.poll(() => itemTitles(page)).toEqual([auto1, auto2, auto3, singleB]);
    await page.getByRole("button", { name: `Remove "${auto1}" from the group` }).click();
    await expect.poll(() => itemTitles(page)).toEqual([auto2, auto3, singleB]);
    await page.getByRole("button", { name: "More" }).click();
    await page.getByRole("menuitem", { name: "Rename…" }).click();
    const dialog = page.getByRole("dialog", { name: "Rename group" });
    await dialog.getByRole("textbox", { name: "Name" }).fill(renamedAuto);
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(dialog).toBeHidden();
    await expect(
      page.getByRole("heading", { level: 1, name: renamedAuto }),
    ).toBeVisible();

    // フォルダに新しいファイルを置いて再スキャンする。
    copyFileSync(path.join(spareRoot(), `${auto4}.mp4`), copiedSpare());
    await rescan(request);
    await waitForVideos(request, [auto4]);

    // 変更は残り、新しいファイルが末尾に加わっている（5）。
    await page.reload();
    await expect(
      page.getByRole("heading", { level: 1, name: renamedAuto }),
    ).toBeVisible();
    await expect.poll(() => itemTitles(page)).toEqual([auto2, auto3, singleB, auto4]);
    // 外した自動1 はライブラリに単体で残る。表示形式はライブラリと共有なので、格子に戻す。
    await page
      .getByRole("radiogroup", { name: "View" })
      .getByRole("radio", { name: "Grid" })
      .click();
    await page.goto(librarySearch);
    await expect(videoCard(page, auto1)).toBeVisible();
  });

  test("解散した自動グループは再スキャンしても戻らない", async ({ page, request }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await openGroup(page, dissolvedFolder);
    await page.getByRole("button", { name: "More" }).click();
    await page.getByRole("menuitem", { name: "Dissolve…" }).click();
    const dialog = page.getByRole("alertdialog", {
      name: `Dissolve "${dissolvedFolder}"?`,
    });
    await expect(dialog).toContainText("Its folder is no longer grouped automatically.");
    await dialog.getByRole("button", { name: "Dissolve" }).click();
    await expect(page).toHaveURL("/library");
    await expect(toast(page, `Dissolved "${dissolvedFolder}"`)).toBeVisible();

    // 再スキャンしてもグループは戻らず、2 本は単体のカードで出る（6）。
    await rescan(request);
    await page.goto(librarySearch);
    await expect(videoCard(page, dissolved1)).toBeVisible();
    await expect(videoCard(page, dissolved2)).toBeVisible();
    await expect(groupCard(page, dissolvedFolder)).toHaveCount(0);
    expect(await groupsNamed(request, dissolvedFolder)).toEqual([]);
  });

  test("手で作ったグループの動画のファイルを別のフォルダへ移しても、再スキャン後もグループに残る", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    const { from, to } = movedPaths();
    mkdirSync(path.dirname(to), { recursive: true });
    renameSync(from, to);
    await rescan(request);
    await waitForVideos(request, [video3]);
    await expect
      .poll(async () => (await readVideos(request)).get(video3)?.folder?.path)
      .toBe("移動先");

    // 動画3 は「三つのフォルダ」に入ったまま、並びも変わらない（7）。
    await openGroup(page, spread);
    await expect(headerCount(page)).toHaveText(/^3 videos · /);
    await showList(page);
    await expect.poll(() => itemTitles(page)).toEqual([video1, video2, video3]);
  });
});

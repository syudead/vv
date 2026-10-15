import {
  type APIRequestContext,
  type CDPSession,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

import { ownerStorageState } from "./owner-account";

// ドラッグでの整理（specs/050-drag-organize、親 #1075、子 #1109）を、実サーバーと実メディアに
// 通す。動画は run-e2e.mjs が generateDragOrganizeFixtures で作る 2 秒の動画で、題名は
// media-fixtures.mjs の DRAG_ORGANIZE_VIDEOS と同じ。落とし方の規則、取り消しの像、送る本文の
// 細かな場合はドメイン・ストア・ハンドラー・ページのテストが持つので、ここでは重ねず、親 Issue の
// 受け入れ条件の流れだけを、マウスとタッチで確かめる（docs/design-docs/web-testing.md の
// Test levels）。括弧の番号は親 Issue の受け入れ条件である。
//
// テストの組ごとに題名の頭（「並べ確認」など）を変え、ライブラリの検索でその組だけを出す。
// グループは API で作り、ドラッグは画面で行う。どの操作も、取り消して元に戻るのを確かめて（8）
// から、もう一度行ってその結果を確かめる。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
  thumbnailUrl: string;
  versions?: { count: number; representativeId: number };
}

interface LibraryGroup {
  id: number;
  name: string;
  favorite?: boolean;
}

interface GroupItem {
  id: number;
  kind: "video" | "group";
  video?: Video;
  group?: LibraryGroup;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

// media-fixtures.mjs の DRAG_ORGANIZE_VIDEOS と同じ題名。
const reorderVideos = ["並べ確認の1", "並べ確認の2", "並べ確認の3"] as const;
const makeA = "作る確認のA";
const makeB = "作る確認のB";
const bundleA = "束ね確認のA";
const bundleB = "束ね確認のB";
const mergeX = ["合流確認のX1", "合流確認のX2"] as const;
const mergeY1 = "合流確認のY1";
const nestP1 = "入れ子確認のP1";
const nestQ1 = "入れ子確認のQ1";
const moveA = ["移動確認のA1", "移動確認のA2"] as const;
const moveB1 = "移動確認のB1";
const selected = ["選択確認の1", "選択確認の2", "選択確認の3"] as const;
const selectTarget1 = "選択確認の先1";
const touchVideo = "タッチ確認の1";
const touchTarget1 = "タッチ確認の先1";
const allTitles = [
  ...reorderVideos,
  makeA,
  makeB,
  bundleA,
  bundleB,
  ...mergeX,
  mergeY1,
  nestP1,
  nestQ1,
  ...moveA,
  moveB1,
  ...selected,
  selectTarget1,
  touchVideo,
  touchTarget1,
];

// テストが API で作るグループ。「作る確認のB」は「Make a group」が B の題名で作るグループ。
const reorderGroup = "並べ確認のグループ";
const groupX = "合流確認のX";
const groupY = "合流確認のY";
const groupP = "入れ子確認のP";
const groupQ = "入れ子確認のQ";
const groupA = "移動確認のA";
const groupB = "移動確認のB";
const selectGroup = "選択確認の先";
const touchGroup = "タッチ確認の先";
const groupNames = [
  reorderGroup,
  makeB,
  groupX,
  groupY,
  groupP,
  groupQ,
  groupA,
  groupB,
  selectGroup,
  touchGroup,
];

let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();
const groups = new Map<string, LibraryGroup>();

function mediaRoot(): string {
  const root = process.env.MDM_E2E_DRAG_ORGANIZE_MEDIA_DIR;
  if (root === undefined) {
    throw new Error("MDM_E2E_DRAG_ORGANIZE_MEDIA_DIR is not configured");
  }
  return root;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function videoOf(title: string): Video {
  const video = videos.get(title);
  if (video === undefined) throw new Error(`video ${title} is not loaded`);
  return video;
}

function groupOf(name: string): LibraryGroup {
  const group = groups.get(name);
  if (group === undefined) throw new Error(`group ${name} is not created`);
  return group;
}

function librarySearch(query: string): string {
  return `/?q=${encodeURIComponent(query)}`;
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

/** readVideos は API から、題名に「確認の」を含む動画をすべて題名で引けるように読む。 */
async function readVideos(request: APIRequestContext): Promise<Map<string, Video>> {
  const found = new Map<string, Video>();
  let cursor: string | undefined;
  do {
    const params = new URLSearchParams({ limit: "200", query: "確認の" });
    if (cursor !== undefined) params.set("cursor", cursor);
    const response = await request.get(`/api/videos?${params.toString()}`);
    expect(response.status()).toBe(200);
    const page = (await response.json()) as { items: Video[]; nextCursor?: string };
    for (const item of page.items) found.set(item.title, item);
    cursor = page.nextCursor;
  } while (cursor !== undefined);
  return found;
}

/** waitForVideos はすべての動画が読み込まれ、サムネイルの生成を待たなくなるまで待つ。 */
async function waitForVideos(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const found = await readVideos(request);
        const ready = allTitles.every((title) => {
          const item = found.get(title);
          return item?.probeState === "done" && item.thumbnailState !== "pending";
        });
        if (ready) for (const title of allTitles) videos.set(title, found.get(title)!);
        return ready;
      },
      { timeout: 120_000 },
    )
    .toBe(true);
}

/** groupsNamed は API から、name という名前のグループをすべて読む。 */
async function groupsNamed(request: APIRequestContext, name: string) {
  const response = await request.get(
    `/api/groups?limit=200&q=${encodeURIComponent(name)}`,
  );
  expect(response.status()).toBe(200);
  return ((await response.json()) as { groups: LibraryGroup[] }).groups.filter(
    (group) => group.name === name,
  );
}

/** dissolveAll は name のグループをすべて解散する。前の試行が残したものも消す。 */
async function dissolveAll(request: APIRequestContext) {
  for (const name of groupNames) {
    for (const group of await groupsNamed(request, name)) {
      const removed = await request.delete(`/api/groups/${String(group.id)}`, {
        headers: mutationHeaders,
      });
      expect([204, 404]).toContain(removed.status());
    }
  }
}

/** unbundle は束ねる確認の 2 本を集まりから外す。前の試行が束ねたままでも戻す。 */
async function unbundle(request: APIRequestContext) {
  for (const title of [bundleA, bundleB]) {
    const video = videos.get(title);
    if (video === undefined) continue;
    const response = await request.post(`/api/videos/${String(video.id)}/unbundle`, {
      headers: mutationHeaders,
    });
    expect(response.status()).toBeLessThan(500);
  }
}

/** createGroup は API で、動画 `titles` と子グループ `children` からグループを作る。 */
async function createGroup(
  request: APIRequestContext,
  name: string,
  titles: readonly string[],
  children: readonly string[] = [],
) {
  const response = await request.post("/api/groups", {
    headers: mutationHeaders,
    data: {
      name,
      videoIds: titles.map((title) => videoOf(title).id),
      groupIds: children.map((child) => groupOf(child).id),
    },
  });
  expect(response.status()).toBe(201);
  groups.set(name, (await response.json()) as LibraryGroup);
}

/** readGroup は API から、グループの値とその項目を読む。無ければ null。 */
async function readGroup(request: APIRequestContext, name: string) {
  const response = await request.get(`/api/groups/${String(groupOf(name).id)}`);
  if (response.status() === 404) return null;
  expect(response.status()).toBe(200);
  return (await response.json()) as { group: LibraryGroup; items: GroupItem[] };
}

/** itemNames は API のグループの項目の、並びのままの題名か名前である。無ければ null。 */
async function itemNames(request: APIRequestContext, name: string) {
  const detail = await readGroup(request, name);
  return detail?.items.map((item) => item.video?.title ?? item.group?.name ?? "") ?? null;
}

/** videoCard は一覧の、title の動画のカード（article）である。 */
function videoCard(page: Page, title: string): Locator {
  return page.locator("article[data-video-id]").filter({
    has: page.locator("h3", { hasText: new RegExp(`^${escapeRegExp(title)}$`) }),
  });
}

/** groupCard は一覧の、name のグループのカード（article）である。 */
function groupCard(page: Page, name: string): Locator {
  return page.locator("article[data-group-id]").filter({
    has: page.getByRole("link", {
      name: new RegExp(`^${escapeRegExp(name)}, group of `),
    }),
  });
}

/** dragCard はドラッグの層のカード（落とした先の印を重ねる包み）である。 */
function dragCard(card: Locator): Locator {
  return card.page().locator("[data-drag-card]").filter({ has: card });
}

/** cardTitles は格子の動画のカードの、並びのままの題名である。 */
async function cardTitles(page: Page): Promise<string[]> {
  return page
    .locator("article[data-video-id] h3")
    .evaluateAll((elements) => elements.map((element) => element.textContent ?? ""));
}

function notifications(page: Page): Locator {
  return page.getByRole("region", { name: /Notifications/ });
}

/** toast は画面に出た知らせのうち、text のものである。 */
function toast(page: Page, text: string | RegExp): Locator {
  return notifications(page).getByText(text, { exact: true });
}

/** organized は次の POST /api/organize/{kind} の答えを待つ。 */
function organized(page: Page, kind: string) {
  return page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === `/api/organize/${kind}`,
  );
}

/** undo は知らせの「Undo」を押し、取り消しが受け入れられて「Undone」が出るのを待つ。 */
async function undo(page: Page) {
  const sent = organized(page, "undo");
  await notifications(page).getByRole("button", { name: "Undo" }).click();
  expect((await sent).status()).toBe(204);
  await expect(toast(page, "Undone")).toBeVisible();
}

interface Point {
  x: number;
  y: number;
}

/** pointIn は要素の箱の中の、幅と高さに対する割合 (fx, fy) の点である。 */
async function pointIn(locator: Locator, fx = 0.5, fy = 0.5): Promise<Point> {
  const box = await locator.boundingBox();
  if (box === null) throw new Error("the element has no box");
  return { x: box.x + box.width * fx, y: box.y + box.height * fy };
}

/** preview はポインタに付いてくる写しである。 */
function preview(page: Page): Locator {
  return page.locator("[data-drag-preview]");
}

/**
 * mouseLift はカード `card` のリンク（サムネイル）をマウスで押して 8 px より動かし、写しが
 * 出るのを待つ。離すのは mouseDrop。
 */
async function mouseLift(card: Locator) {
  const page = card.page();
  const start = await pointIn(card.getByRole("link").first());
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.mouse.move(start.x + 12, start.y + 12, { steps: 4 });
  await expect(preview(page)).toBeVisible();
}

/** mouseMoveTo は持ち上げたままポインタを `point` まで動かす。 */
async function mouseMoveTo(page: Page, point: Point) {
  await page.mouse.move(point.x, point.y, { steps: 12 });
  // 最後の 1 歩で当たり判定をもう一度走らせ、印を今の場所に合わせる。
  await page.mouse.move(point.x + 1, point.y + 1);
}

/**
 * dropOnto は `source` をマウスで持ち上げ、`target` の真ん中に運ぶ。落とす前に、落とした先の
 * カードに札 `label` が出るのを確かめてから離す。
 */
async function dropOnto(source: Locator, target: Locator, label: string) {
  const page = source.page();
  await mouseLift(source);
  await mouseMoveTo(page, await pointIn(dragCard(target), 0.5, 0.3));
  await expect(dragCard(target).getByText(label, { exact: true })).toBeVisible();
  await page.mouse.up();
}

/** menuItem は落とした先に開いたメニューの、name の項目である。 */
async function menuItem(page: Page, target: string, name: string): Promise<Locator> {
  const menu = page.getByRole("menu");
  await expect(menu).toContainText(`Dropped on "${target}"`);
  return menu.getByRole("menuitem", { name, exact: true });
}

/** openGroupPage はライブラリの検索から name のグループのカードを押し、そのページを開く。 */
async function openGroupPage(page: Page, query: string, name: string) {
  await page.goto(librarySearch(query));
  await groupCard(page, name).getByRole("link").first().click();
  await expect(page).toHaveURL(/\/groups\/\d+$/);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
}

/** Touch は CDP で 1 本の指のタッチを送る（Playwright の touchscreen は tap しか無い）。 */
class Touch {
  private readonly cdp: CDPSession;

  constructor(cdp: CDPSession) {
    this.cdp = cdp;
  }

  async start(point: Point) {
    await this.cdp.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: point.x, y: point.y }],
    });
  }

  async moveTo(from: Point, to: Point, steps = 12) {
    for (let step = 1; step <= steps; step += 1) {
      const x = from.x + ((to.x - from.x) * step) / steps;
      const y = from.y + ((to.y - from.y) * step) / steps;
      await this.cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y }],
      });
    }
  }

  async end() {
    await this.cdp.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
  }
}

test.describe.serial("drag organize", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaRoot() },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);
    await waitForVideos(request);
    // CI のリトライは前の試行のグループと集まりを見るので、作り直す前に消す。
    await dissolveAll(request);
    await unbundle(request);

    await createGroup(request, reorderGroup, reorderVideos);
    await createGroup(request, groupX, mergeX);
    await createGroup(request, groupY, [mergeY1]);
    await createGroup(request, groupP, [nestP1]);
    await createGroup(request, groupQ, [nestQ1]);
    await createGroup(request, groupB, [moveB1]);
    await createGroup(request, groupA, moveA, [groupB]);
    await createGroup(request, selectGroup, [selectTarget1]);
    await createGroup(request, touchGroup, [touchTarget1]);
    const favorite = await request.put("/api/favorites", {
      headers: mutationHeaders,
      data: { groupIds: [groupOf(groupX).id], favorite: true },
    });
    expect(favorite.status()).toBe(200);
  });

  test.afterAll(async ({ request }) => {
    await dissolveAll(request);
    await unbundle(request);
    if (folder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("グループのページで 3 番目を先頭へ落とすと、再読み込み後もその順で、カードはその動画のサムネイルになる", async ({
    page,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    const [first, second, third] = reorderVideos;
    await openGroupPage(page, "並べ確認", reorderGroup);
    await expect.poll(() => cardTitles(page)).toEqual([first, second, third]);

    /** 3 番目のカードを 1 番目のカードの前の間へ運び、間の棒を確かめて離す。 */
    const reorder = async () => {
      const sent = organized(page, "reorder");
      await mouseLift(videoCard(page, third));
      const target = dragCard(videoCard(page, first));
      await mouseMoveTo(page, await pointIn(target, 0.1, 0.4));
      await expect(target.locator('[data-drop-bar="before"]')).toBeVisible();
      await page.mouse.up();
      expect((await sent).status()).toBe(200);
      await expect(toast(page, `Reordered "${reorderGroup}"`)).toBeVisible();
      await expect.poll(() => cardTitles(page)).toEqual([third, first, second]);
    };

    // 取り消すと前の順に戻る（8）。
    await reorder();
    await undo(page);
    await expect.poll(() => cardTitles(page)).toEqual([first, second, third]);

    // 並べ替えた順は再読み込みしても残る（1）。
    await reorder();
    await page.reload();
    await expect(
      page.getByRole("heading", { level: 1, name: reorderGroup }),
    ).toBeVisible();
    await expect.poll(() => cardTitles(page)).toEqual([third, first, second]);

    // ライブラリのグループのカードは、新しい先頭の動画のサムネイルを使う（1）。
    await page.goto(librarySearch("並べ確認"));
    await expect(groupCard(page, reorderGroup).locator("img").first()).toHaveAttribute(
      "src",
      videoOf(third).thumbnailUrl,
    );
  });

  test("ライブラリで A を B に落として「Make a group」を選ぶと、B の題名のグループのカードが 1 枚できる", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch("作る確認"));
    await expect(videoCard(page, makeA)).toBeVisible();
    await expect(videoCard(page, makeB)).toBeVisible();

    const makeGroup = async () => {
      const sent = organized(page, "group");
      await dropOnto(videoCard(page, makeA), videoCard(page, makeB), "Group or bundle…");
      await (await menuItem(page, makeB, "Make a group")).click();
      expect((await sent).status()).toBe(200);
      await expect(toast(page, `Made the group "${makeB}"`)).toBeVisible();
      // 名前の欄は B の題名を仮の名前として持つ。Esc でそのまま残す。
      const field = page.getByRole("textbox", { name: "Group name" });
      await expect(field).toBeFocused();
      await expect(field).toHaveValue(makeB);
      await page.keyboard.press("Escape");
      await expect(field).toBeHidden();
      await expect(groupCard(page, makeB)).toHaveCount(1);
      await expect(
        groupCard(page, makeB).getByRole("link", { name: `${makeB}, group of 2 videos` }),
      ).toBeVisible();
      await expect(videoCard(page, makeA)).toHaveCount(0);
      await expect(videoCard(page, makeB)).toHaveCount(0);
    };

    // 取り消すとグループは消え、2 本は単体のカードに戻る（8）。
    await makeGroup();
    await undo(page);
    await expect(groupCard(page, makeB)).toHaveCount(0);
    await expect(videoCard(page, makeA)).toBeVisible();
    await expect(videoCard(page, makeB)).toBeVisible();
    expect(await groupsNamed(request, makeB)).toEqual([]);

    // グループは A と B を入れ、名前は B の題名である（2）。
    await makeGroup();
    const made = await groupsNamed(request, makeB);
    expect(made).toHaveLength(1);
    groups.set(makeB, made[0]!);
    expect(await itemNames(request, makeB)).toEqual([makeB, makeA]);
  });

  test("A を B に落として「Bundle as versions」を選ぶと、ダイアログを出さずに B を代表に束ねる", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch("束ね確認"));
    await expect(videoCard(page, bundleA)).toBeVisible();

    const readVersions = async (title: string) => {
      const response = await request.get(`/api/videos/${String(videoOf(title).id)}`);
      expect(response.status()).toBe(200);
      return ((await response.json()) as Video).versions ?? null;
    };
    const bundle = async () => {
      const sent = organized(page, "bundle");
      await dropOnto(
        videoCard(page, bundleA),
        videoCard(page, bundleB),
        "Group or bundle…",
      );
      await (await menuItem(page, bundleB, "Bundle as versions")).click();
      expect((await sent).status()).toBe(200);
      await expect(
        toast(page, `Bundled 2 videos as versions of "${bundleB}"`),
      ).toBeVisible();
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(videoCard(page, bundleA)).toHaveCount(0);
      await expect(videoCard(page, bundleB)).toBeVisible();
    };

    // 取り消すと集まりは無くなり、A のカードが戻る（8）。
    await bundle();
    await undo(page);
    await expect(videoCard(page, bundleA)).toBeVisible();
    expect(await readVersions(bundleB)).toBeNull();

    // B が代表の 2 本の集まりになる（3）。
    await bundle();
    expect(await readVersions(bundleA)).toEqual({
      count: 2,
      representativeId: videoOf(bundleB).id,
    });
  });

  test("グループ X を Y に落として「Merge」を選ぶと、Y の末尾に X のメンバーが並び、X のお気に入りは Y になる", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch("合流確認"));
    await expect(groupCard(page, groupX)).toBeVisible();

    const merge = async () => {
      const sent = organized(page, "merge");
      await dropOnto(
        groupCard(page, groupX),
        groupCard(page, groupY),
        "Put in or merge…",
      );
      const item = await menuItem(page, groupY, "Merge");
      await expect(
        page.getByRole("menuitem", { name: "Put in as a child group", exact: true }),
      ).toBeVisible();
      await item.click();
      expect((await sent).status()).toBe(200);
      await expect(toast(page, `Merged 1 group into "${groupY}"`)).toBeVisible();
      await expect(groupCard(page, groupX)).toHaveCount(0);
    };

    // 取り消すと X はお気に入りのまま戻り、Y は元のメンバーだけになる（8）。
    await merge();
    expect(await readGroup(request, groupX)).toBeNull();
    await undo(page);
    await expect(groupCard(page, groupX)).toBeVisible();
    expect((await readGroup(request, groupX))?.group.favorite).toBe(true);
    expect((await readGroup(request, groupY))?.group.favorite).toBe(false);
    expect(await itemNames(request, groupY)).toEqual([mergeY1]);

    // Y の末尾に X のメンバーが X の順で並び、X は消える（4）。
    await merge();
    await openGroupPage(page, "合流確認", groupY);
    await expect.poll(() => cardTitles(page)).toEqual([mergeY1, ...mergeX]);
    expect(await readGroup(request, groupX)).toBeNull();
    // X をお気に入りにしていたので、Y がお気に入りに出る（4）。
    await page.goto(`${librarySearch("合流確認")}&fav=1`);
    await expect(groupCard(page, groupY)).toBeVisible();
  });

  test("グループ P を Q に落として「Put in as a child group」を選ぶと、Q を開くと P が出る", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch("入れ子確認"));
    await expect(groupCard(page, groupP)).toBeVisible();

    const nest = async () => {
      const sent = organized(page, "move");
      await dropOnto(
        groupCard(page, groupP),
        groupCard(page, groupQ),
        "Put in or merge…",
      );
      await (await menuItem(page, groupQ, "Put in as a child group")).click();
      expect((await sent).status()).toBe(200);
      await expect(toast(page, `Moved 1 item to "${groupQ}"`)).toBeVisible();
      await expect(groupCard(page, groupP)).toHaveCount(0);
    };

    // 取り消すと P はいちばん上に戻る（8）。
    await nest();
    await undo(page);
    await expect(groupCard(page, groupP)).toBeVisible();
    expect(await itemNames(request, groupQ)).toEqual([nestQ1]);

    // Q を開くと P が出る（5）。
    await nest();
    await openGroupPage(page, "入れ子確認", groupQ);
    await expect(groupCard(page, groupP)).toBeVisible();
    await expect(videoCard(page, nestQ1)).toBeVisible();
  });

  test("グループ A の中の動画を子のグループ B に落とすと、A から消えて B に出る", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    const [moved, kept] = moveA;
    await openGroupPage(page, "移動確認", groupA);
    await expect.poll(() => cardTitles(page)).toEqual([moved, kept]);
    await expect(groupCard(page, groupB)).toBeVisible();

    const move = async () => {
      const sent = organized(page, "move");
      await dropOnto(videoCard(page, moved), groupCard(page, groupB), "Put in");
      expect((await sent).status()).toBe(200);
      await expect(toast(page, `Moved 1 item to "${groupB}"`)).toBeVisible();
      await expect(videoCard(page, moved)).toHaveCount(0);
    };

    // 取り消すと A に戻り、B は元のままになる（8）。
    await move();
    await undo(page);
    await expect.poll(() => cardTitles(page)).toEqual([moved, kept]);
    expect(await itemNames(request, groupB)).toEqual([moveB1]);

    // A から消え、B を開くと出る（6）。
    await move();
    expect(await itemNames(request, groupA)).toEqual([kept, groupB]);
    await groupCard(page, groupB).getByRole("link").first().click();
    await expect(page.getByRole("heading", { level: 1, name: groupB })).toBeVisible();
    await expect.poll(() => cardTitles(page)).toEqual([moveB1, moved]);
  });

  test("選んだ 3 枚をグループに落とすと、3 枚とも元の順のままそのグループに入る", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(librarySearch("選択確認"));
    for (const title of selected) await expect(videoCard(page, title)).toBeVisible();
    // 画面の順（ライブラリの並び替えの順）が、グループに入る順である。
    const screenOrder = (await cardTitles(page)).filter((title) =>
      (selected as readonly string[]).includes(title),
    );
    expect(screenOrder).toHaveLength(3);

    const putIn = async () => {
      for (const title of selected) {
        await page.getByRole("checkbox", { name: `Select "${title}"` }).click();
      }
      const sent = organized(page, "move");
      await mouseLift(videoCard(page, screenOrder[1]!));
      // 持ち上げたカードの上では拒否の印が出る。落とす先の上で写しに件数が出る。
      await expect(preview(page).getByLabel("Can't drop here")).toBeVisible();
      await mouseMoveTo(
        page,
        await pointIn(dragCard(groupCard(page, selectGroup)), 0.5, 0.3),
      );
      await expect(
        dragCard(groupCard(page, selectGroup)).getByText("Put in", { exact: true }),
      ).toBeVisible();
      await expect(preview(page).getByLabel("3 items")).toBeVisible();
      await page.mouse.up();
      expect((await sent).status()).toBe(200);
      await expect(toast(page, `Moved 3 items to "${selectGroup}"`)).toBeVisible();
      for (const title of selected) await expect(videoCard(page, title)).toHaveCount(0);
    };

    // 取り消すと 3 枚はライブラリに戻り、グループは元のままになる（8）。
    await putIn();
    await undo(page);
    for (const title of selected) await expect(videoCard(page, title)).toBeVisible();
    expect(await itemNames(request, selectGroup)).toEqual([selectTarget1]);

    // 3 枚とも画面の順のまま末尾に入る（7）。
    await putIn();
    expect(await itemNames(request, selectGroup)).toEqual([
      selectTarget1,
      ...screenOrder,
    ]);
  });

  test("タッチでも、長押しで持ち上げた動画をグループに落として入れ、取り消せる", async ({
    browser,
    request,
  }) => {
    test.setTimeout(90_000);
    const context = await browser.newContext({
      storageState: ownerStorageState,
      hasTouch: true,
      isMobile: true,
      viewport: { width: 360, height: 800 },
    });
    const page = await context.newPage();
    try {
      const touch = new Touch(await context.newCDPSession(page));
      await page.goto(librarySearch("タッチ確認"));
      const source = videoCard(page, touchVideo);
      const target = dragCard(groupCard(page, touchGroup));
      await expect(source).toBeVisible();
      await expect(target).toBeVisible();

      const putIn = async () => {
        const sent = organized(page, "move");
        const start = await pointIn(source.getByRole("link").first());
        await touch.start(start);
        // 200 ms の長押しで持ち上がる。
        await expect(preview(page)).toBeVisible();
        const end = await pointIn(target, 0.5, 0.3);
        await touch.moveTo(start, end);
        await expect(target.getByText("Put in", { exact: true })).toBeVisible();
        await touch.end();
        expect((await sent).status()).toBe(200);
        await expect(toast(page, `Moved 1 item to "${touchGroup}"`)).toBeVisible();
        await expect(source).toHaveCount(0);
      };

      // 取り消すと動画はライブラリに戻る（8）。
      await putIn();
      const sent = organized(page, "undo");
      await notifications(page).getByRole("button", { name: "Undo" }).tap();
      expect((await sent).status()).toBe(204);
      await expect(source).toBeVisible();
      expect(await itemNames(request, touchGroup)).toEqual([touchTarget1]);

      await putIn();
      expect(await itemNames(request, touchGroup)).toEqual([touchTarget1, touchVideo]);
    } finally {
      await context.close();
    }
  });

  test("動画のページでは、カードもメンバーの一覧もドラッグできない", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    const requests: string[] = [];
    page.on("request", (request) => {
      const { pathname } = new URL(request.url());
      if (pathname.startsWith("/api/organize/")) requests.push(pathname);
    });
    const [first, second] = reorderVideos;
    await openGroupPage(page, "並べ確認", reorderGroup);
    await videoCard(page, first).getByRole("link").first().click();
    await expect(page).toHaveURL(/\/videos\/\d+\?group=\d+$/);
    const upNext = page.getByRole("region", { name: "Up next" });
    const member = upNext.getByRole("link", {
      name: new RegExp(`^${escapeRegExp(second)}\\b`),
    });
    await expect(member).toBeVisible();

    // ドラッグの層に加わったカードや行は無い（9）。
    await expect(page.locator("[data-drag-card]")).toHaveCount(0);

    // メンバーを押して動かしても、何も持ち上がらない（9）。
    const start = await pointIn(member);
    await page.mouse.move(start.x, start.y);
    await page.mouse.down();
    await page.mouse.move(start.x + 40, start.y + 40, { steps: 8 });
    await expect(preview(page)).toHaveCount(0);
    await page.mouse.up();
    await expect(page).toHaveURL(/\/videos\/\d+\?group=\d+$/);
    expect(requests).toEqual([]);
  });
});

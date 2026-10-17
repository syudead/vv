import {
  type APIRequestContext,
  type CDPSession,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

import { ownerStorageState } from "./owner-account";

// ドラッグでの整理を、実サーバーと実メディアに
// 通す。動画は run-e2e.mjs が generateDragOrganizeFixtures で作る 2 秒の動画で、題名は
// media-fixtures.mjs の DRAG_ORGANIZE_VIDEOS と同じ。落とし方の規則、取り消しの像、送る本文と、
// 束ね・マージ・入れ子・移動・複数の選択の結果（受け入れ条件 3〜7・9）はドメイン・ストア・
// ハンドラー・ページのテストが持つので、ここでは重ねない。残すのは、本物の配置とポインタで
// 間に落とす並べ替え（1）、カードの上に落としてメニューを開く「Make a group」（2）、タッチの
// 長押し（docs/design-docs/web-testing.md の Test levels）である。括弧の番号は親 Issue の
// 受け入れ条件である。
//
// テストの組ごとに題名の頭（「並べ確認」など）を変え、ライブラリの検索でその組だけを出す。
// グループは API で作り、ドラッグは画面で行う。取り消し（8）・再読み込みの後の順・グループの
// カードのサムネイルは、ストア・ハンドラー・ページのテストが持つ。

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
}

interface LibraryGroup {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

// media-fixtures.mjs の DRAG_ORGANIZE_VIDEOS と同じ題名。
const reorderVideos = ["並べ確認の1", "並べ確認の2", "並べ確認の3"] as const;
const makeA = "作る確認のA";
const makeB = "作る確認のB";
const touchVideo = "タッチ確認の1";
const touchTarget1 = "タッチ確認の先1";
const allTitles = [...reorderVideos, makeA, makeB, touchVideo, touchTarget1];

// テストが API で作るグループ。「作る確認のB」は「Make a group」が B の題名で作るグループ。
const reorderGroup = "並べ確認のグループ";
const touchGroup = "タッチ確認の先";
const groupNames = [reorderGroup, makeB, touchGroup];

let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();

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

/** createGroup は API で、動画 `titles` からグループを作る。 */
async function createGroup(
  request: APIRequestContext,
  name: string,
  titles: readonly string[],
) {
  const response = await request.post("/api/groups", {
    headers: mutationHeaders,
    data: { name, videoIds: titles.map((title) => videoOf(title).id), groupIds: [] },
  });
  expect(response.status()).toBe(201);
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
    // CI のリトライは前の試行のグループを見るので、作り直す前に消す。
    await dissolveAll(request);

    await createGroup(request, reorderGroup, reorderVideos);
    await createGroup(request, touchGroup, [touchTarget1]);
  });

  test.afterAll(async ({ request }) => {
    await dissolveAll(request);
    if (folder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("グループのページで 3 番目を先頭の前の間へ運ぶと間の棒が出て、落とすとその順になる", async ({
    page,
  }) => {
    test.setTimeout(90_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    const [first, second, third] = reorderVideos;
    await openGroupPage(page, "並べ確認", reorderGroup);
    await expect.poll(() => cardTitles(page)).toEqual([first, second, third]);

    const sent = organized(page, "reorder");
    await mouseLift(videoCard(page, third));
    const target = dragCard(videoCard(page, first));
    await mouseMoveTo(page, await pointIn(target, 0.1, 0.4));
    await expect(target.locator('[data-drop-bar="before"]')).toBeVisible();
    await page.mouse.up();
    expect((await sent).status()).toBe(200);
    await expect(toast(page, `Reordered "${reorderGroup}"`)).toBeVisible();
    await expect.poll(() => cardTitles(page)).toEqual([third, first, second]);
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

    const sent = organized(page, "group");
    await dropOnto(videoCard(page, makeA), videoCard(page, makeB), "Group or bundle…");
    await (await menuItem(page, makeB, "Make a group")).click();
    expect((await sent).status()).toBe(200);
    await expect(toast(page, `Made the group "${makeB}"`)).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(
      groupCard(page, makeB).getByRole("link", { name: `${makeB}, group of 2 videos` }),
    ).toBeVisible();
    const made = await groupsNamed(request, makeB);
    expect(made).toHaveLength(1);
  });

  test("タッチでも、長押しで持ち上げた動画をグループに落として入れる", async ({
    browser,
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
    } finally {
      await context.close();
    }
  });
});

import {
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

// ホーム（specs/057-home、親 #1140 受け入れ条件 1・2・5・6・7・8、子 #1323）を実サーバーと実メディア
// に通す。動画は run-e2e.mjs が generateHomeFixtures で作る22本で、どれもフォルダの直下に置く。
// 棚の並び・読み込み・「See all」の行き先などの規則はロジックテストとページテストが確かめ、
// ここでは重ねない（docs/design-docs/web-testing.md）。
//
// 走査の終わりの棚の追加は、待ち行列が 2 分空のまま続いてから走る（research.md R-9）ので、
// 受け入れ条件 5 の確かめは数分かかる。

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

interface Named {
  id: number;
  name: string;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
}

interface HomeShelf {
  key: string;
  kind: string;
  name: string;
  folderId?: number;
  byOrganizer?: boolean;
}

interface HomeOrganizeRun {
  run: string;
  outcome: string;
  finishedAt: string;
}

interface Home {
  shelves: HomeShelf[];
  organizer?: { running: boolean; pending?: string; last?: HomeOrganizeRun };
}

interface SmartFolder {
  id: number;
  name: string;
  rule: Record<string, unknown>;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の generateHomeFixtures と同じ題名。
const query = "ホーム確認";
const fillers = Array.from(
  { length: 21 },
  (_, index) => `ホーム確認 ${String(index + 1).padStart(2, "0")}`,
);
const inProgress = "ホーム確認の途中";
const allTitles = [...fillers, inProgress];
// 受け入れ条件 8 で公開にする2本。ほかの e2e の動画はどれも非公開である。
const publicTitles = fillers.slice(0, 2);
const privateTitles = allTitles.filter((title) => !publicTitles.includes(title));
// 受け入れ条件 5 で 5 本（research.md R-2 の下限）に付けるタグ。
const shelfTag = "ホーム確認のタグ";
const taggedTitles = fillers.slice(0, 5);
const videos = new Map<string, Video>();
let mediaFolder: MediaFolder | undefined;
// 受け入れ条件 5 で整理役が足した棚（スマートフォルダ）の id。
let addedShelfId: number | undefined;

function mediaRoot(): string {
  const root = process.env.MDM_E2E_HOME_MEDIA_DIR;
  if (root === undefined) throw new Error("MDM_E2E_HOME_MEDIA_DIR is not configured");
  return root;
}

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

async function startScan(request: APIRequestContext) {
  const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
  expect(scan.status()).toBe(202);
  const scanId = ((await scan.json()) as { id: number }).id;
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const current = (await response.json()) as { id: number; state: string };
        return current.id === scanId && current.state !== "running"
          ? current.state
          : "running";
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

async function readHome(request: APIRequestContext): Promise<Home> {
  const response = await request.get("/api/home");
  expect(response.status()).toBe(200);
  return (await response.json()) as Home;
}

async function readFolder(request: APIRequestContext, id: number): Promise<SmartFolder> {
  const response = await request.get(`/api/smart-folders/${String(id)}`);
  expect(response.status()).toBe(200);
  const detail = (await response.json()) as {
    smartFolder: { id: number; name: string };
    settings: { rule: Record<string, unknown> };
  };
  return { ...detail.smartFolder, rule: detail.settings.rule };
}

/**
 * organizerSettled は整理役が走っておらず待ちも無く、最後の実行が after より後に終わって
 * いれば、その実行を返す。そうでなければ undefined を返す。
 */
async function organizerSettled(
  request: APIRequestContext,
  after: string | undefined,
): Promise<HomeOrganizeRun | undefined> {
  const organizer = (await readHome(request)).organizer;
  if (organizer === undefined || organizer.running || organizer.pending !== undefined) {
    return undefined;
  }
  const last = organizer.last;
  if (last === undefined || last.finishedAt === after) return undefined;
  return last;
}

async function setProgress(request: APIRequestContext, title: string, positionMs: number) {
  const response = await request.put(`/api/videos/${String(video(title).id)}/progress`, {
    headers: mutationHeaders,
    data: { positionMs },
  });
  expect(response.status()).toBe(200);
}

async function setPublic(request: APIRequestContext, titles: string[], shown: boolean) {
  const response = await request.put("/api/video-visibility", {
    headers: mutationHeaders,
    data: { videoIds: titles.map((title) => video(title).id), public: shown },
  });
  expect(response.ok()).toBe(true);
}

/** removeOwnShelves は、この e2e のタグの名前の棚（スマートフォルダ）とタグを消す。 */
async function removeOwnShelves(request: APIRequestContext) {
  const folders = await request.get("/api/smart-folders");
  expect(folders.status()).toBe(200);
  for (const folder of ((await folders.json()) as { smartFolders: Named[] })
    .smartFolders) {
    if (folder.name !== shelfTag) continue;
    const response = await request.delete(`/api/smart-folders/${String(folder.id)}`, {
      headers: mutationHeaders,
    });
    expect([204, 404]).toContain(response.status());
  }
  const tags = await request.get(`/api/tags?q=${encodeURIComponent(shelfTag)}`);
  expect(tags.ok()).toBe(true);
  for (const tag of ((await tags.json()) as { items: Named[] }).items) {
    if (tag.name !== shelfTag) continue;
    await request.delete(`/api/tags/${String(tag.id)}`, { headers: mutationHeaders });
  }
}

/** removeOwnMediaFolder は、この e2e のメディアフォルダの登録を外す（前の試行の分も）。 */
async function removeOwnMediaFolder(request: APIRequestContext) {
  const response = await request.get("/api/media-folders");
  expect(response.status()).toBe(200);
  for (const folder of (await response.json()) as MediaFolder[]) {
    if (folder.path !== mediaRoot()) continue;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.ok()).toBe(true);
  }
}

/** shelfRow はホームの、見出しが title の棚の行である。 */
function shelfRow(page: Page, title: string): Locator {
  return page
    .locator('[data-slot="shelf-row"]')
    .filter({ has: page.getByRole("heading", { level: 2, name: title, exact: true }) });
}

/**
 * showAllShelves はホームを下まで送り、どの棚も項目を読み終えるのを待つ。棚の項目は画面に
 * 入った棚の分だけ読む（research.md R-12）ので、最後の行を画面に入れ続ける。
 */
async function showAllShelves(page: Page) {
  await expect(page.getByRole("region", { name: "Shelves" })).toBeVisible();
  await expect(async () => {
    await page.locator('[data-slot="shelf-row"]').last().scrollIntoViewIfNeeded();
    await expect(page.locator('[data-slot="shelf-row"] [aria-busy="true"]')).toHaveCount(
      0,
      { timeout: 1_000 },
    );
  }).toPass({ timeout: 30_000 });
}

/** cardTitles は locator の中の動画のカードの題名を、並びに依らず比べられるよう並べ替えて返す。 */
async function cardTitles(scope: Locator | Page): Promise<string[]> {
  const titles = await scope.locator("article[data-video-id] h3").allTextContents();
  return titles.sort();
}

function guestContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({ storageState: { cookies: [], origins: [] } });
}

test.describe.serial("home", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(240_000);
    // CI のリトライに備え、前の試行が残したものを先に消す。
    await removeOwnShelves(request);
    await removeOwnMediaFolder(request);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaRoot() },
    });
    expect(created.status()).toBe(201);
    mediaFolder = (await created.json()) as MediaFolder;
    await startScan(request);
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
    // 視聴の記録は動画の中身に結び付き、登録フォルダを外しても残るので、未視聴に戻す。
    await setProgress(request, inProgress, 0);
  });

  test.afterAll(async ({ request }) => {
    if (videos.size > 0) {
      await setPublic(request, allTitles, false);
      await setProgress(request, inProgress, 0);
    }
    await removeOwnShelves(request);
    if (mediaFolder !== undefined) await removeOwnMediaFolder(request);
  });

  test("1: トップを開くとホームが出て、棚の動画を1回押すと再生が始まる", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(page).toHaveURL("/");
    const recent = shelfRow(page, "Recently added");
    const card = recent.getByRole("link", { name: inProgress, exact: true });
    await expect(card).toBeVisible();

    await card.click();
    await expect(page).toHaveURL(`/videos/${String(video(inProgress).id)}`);
    await page.waitForFunction(
      () => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      },
      undefined,
      { timeout: 30_000 },
    );
  });

  test("2: ホームの検索欄に入力すると、ライブラリの画面に検索の結果が出る", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(page.getByRole("region", { name: "Shelves" })).toBeVisible();
    const field = page.getByRole("searchbox", { name: "Search the library" });
    await field.fill(query);
    await field.press("Enter");

    await expect(page).toHaveURL(`/library?q=${encodeURIComponent(query)}`);
    await expect(page.getByRole("searchbox", { name: "Search videos" })).toHaveValue(query);
    await expect.poll(() => cardTitles(page)).toEqual([...allTitles].sort());
  });

  test("6: 途中まで見た動画が「Continue watching」に出る", async ({ page, request }) => {
    await setProgress(request, inProgress, 10_000);
    await page.goto("/");
    await showAllShelves(page);
    const shelf = shelfRow(page, "Continue watching");
    await expect(shelf.getByRole("link", { name: inProgress, exact: true })).toBeVisible();
    // 見ていない動画は出ない。
    await expect(shelf.getByRole("link", { name: fillers[20], exact: true })).toHaveCount(
      0,
    );
  });

  test("8: 閲覧者のホームには、非公開の動画がどの棚にも出ない", async ({
    browser,
    request,
  }) => {
    await setPublic(request, allTitles, false);
    await setPublic(request, publicTitles, true);
    const context = await guestContext(browser);
    try {
      const page = await context.newPage();
      await page.goto("/");
      await showAllShelves(page);
      await expect(shelfRow(page, "Recently added")).toBeVisible();
      const shown = await cardTitles(page);
      for (const title of publicTitles) expect(shown).toContain(title);
      for (const title of privateTitles) expect(shown).not.toContain(title);
      // 閲覧者には所有者の棚が無い。
      await expect(shelfRow(page, "Continue watching")).toHaveCount(0);
    } finally {
      await context.close();
      await setPublic(request, publicTitles, false);
    }
  });

  test("5: 同じタグの動画が一定数を超えると、走査の終わりにそのタグの棚が足され、前の棚は変わらない", async ({
    page,
    request,
  }) => {
    test.setTimeout(420_000);
    // 一度も終わっていなければ走査の終わりは組み直しになるので、先に組み直しを終えておく
    // （research.md R-9）。
    const before = (await readHome(request)).organizer?.last?.finishedAt;
    const organize = await request.post("/api/home/organize", { headers: mutationHeaders });
    expect(organize.status()).toBe(202);
    await expect
      .poll(async () => (await organizerSettled(request, before))?.run, {
        timeout: 60_000,
      })
      .toBe("rebuild");
    const rebuilt = (await readHome(request)).organizer?.last?.finishedAt;
    const shelvesBefore = new Map<number, SmartFolder>();
    for (const shelf of (await readHome(request)).shelves) {
      if (shelf.folderId === undefined) continue;
      shelvesBefore.set(shelf.folderId, await readFolder(request, shelf.folderId));
    }

    const tagged = await request.post("/api/video-tags", {
      headers: mutationHeaders,
      data: {
        videoIds: taggedTitles.map((title) => video(title).id),
        action: "add",
        tag: { name: shelfTag },
      },
    });
    expect(tagged.ok()).toBe(true);
    await startScan(request);

    // 待ち行列が 2 分空のまま続いてから、棚にない動画から棚を足す（R-9・R-10）。
    await expect
      .poll(
        async () => {
          const home = await readHome(request);
          return home.shelves.find((shelf) => shelf.name === shelfTag)?.folderId;
        },
        { timeout: 300_000, intervals: [5_000] },
      )
      .toBeDefined();
    const home = await readHome(request);
    expect(home.organizer?.last?.run).toBe("extend");
    expect(home.organizer?.last?.finishedAt).not.toBe(rebuilt);
    const added = home.shelves.find((shelf) => shelf.name === shelfTag)!;
    expect(added.byOrganizer).toBe(true);
    addedShelfId = added.folderId;

    // それまでの棚は名前も条件も変わらず、足した棚はその後ろに並ぶ。
    const folderIds = home.shelves.flatMap((shelf) =>
      shelf.folderId === undefined ? [] : [shelf.folderId],
    );
    for (const [id, folder] of shelvesBefore) {
      expect(folderIds).toContain(id);
      const now = await readFolder(request, id);
      expect({ name: now.name, rule: now.rule }).toEqual({
        name: folder.name,
        rule: folder.rule,
      });
    }
    expect(folderIds.indexOf(addedShelfId!)).toBeGreaterThan(
      Math.max(-1, ...[...shelvesBefore.keys()].map((id) => folderIds.indexOf(id))),
    );

    await page.goto("/");
    await showAllShelves(page);
    const shelf = shelfRow(page, shelfTag);
    await expect.poll(() => cardTitles(shelf)).toEqual([...taggedTitles].sort());
  });

  test("7: 整理役の棚の条件を手で直してから「Reorganize」を押すと、その棚は直したまま残る", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    expect(addedShelfId).toBeDefined();
    const id = addedShelfId!;
    const original = await readFolder(request, id);
    const rule = { ...original.rule, playable: true };
    const changed = await request.patch(`/api/smart-folders/${String(id)}`, {
      headers: mutationHeaders,
      data: { rule },
    });
    expect(changed.ok()).toBe(true);
    const edited = (await readHome(request)).shelves.find((shelf) => shelf.folderId === id);
    expect(edited?.byOrganizer).toBe(false);

    const before = (await readHome(request)).organizer?.last?.finishedAt;
    await page.goto("/settings");
    const reorganize = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/home/organize") &&
        response.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Reorganize", exact: true }).click();
    expect((await reorganize).status()).toBe(202);
    await expect
      .poll(async () => (await organizerSettled(request, before))?.run, {
        timeout: 60_000,
      })
      .toBe("rebuild");
    await expect(page.getByText(/^Last organized /)).toBeVisible();

    const kept = await readFolder(request, id);
    expect({ name: kept.name, rule: kept.rule }).toEqual({ name: original.name, rule });
    const shelves = (await readHome(request)).shelves;
    expect(shelves.find((shelf) => shelf.folderId === id)?.byOrganizer).toBe(false);
  });
});

import {
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

// Home playback, search, watch progress and guest visibility use the real server
// and three videos from generateHomeFixtures (docs/design-docs/home.md).
// 棚の並び・読み込み・「See all」の行き先などの規則はロジックテストとページテストが確かめ、
// ここでは重ねない（docs/design-docs/web-testing.md）。

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
}

interface HomeShelf {
  key: string;
  name: string;
  folderId: number;
}

interface ShelfItem {
  kind: string;
  video?: { id: number; title: string };
  group?: { videoIds: number[] };
}

interface HomeShelfItems {
  key: string;
  items: ShelfItem[];
  total: number;
}

interface Home {
  shelves: HomeShelf[];
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の generateHomeFixtures と同じ題名。
const query = "ホーム確認";
const fillers = Array.from(
  { length: 2 },
  (_, index) => `ホーム確認 ${String(index + 1).padStart(2, "0")}`,
);
const inProgress = "ホーム確認の途中";
const allTitles = [...fillers, inProgress];
// 受け入れ条件 8 で公開にする2本。ほかの e2e の動画はどれも非公開である。
const publicTitles = fillers.slice(0, 2);
const privateTitles = allTitles.filter((title) => !publicTitles.includes(title));
const videos = new Map<string, Video>();
let mediaFolder: MediaFolder | undefined;
let shelfId: number | undefined;
let previousShelfIds: number[] = [];
const shelfName = "Home playback test";

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

/**
 * readAllShelfItems は見る人のホームのどの棚も、listHomeShelfItems の上限の 8 件ずつ鍵を
 * 送って項目を読む。画面がまだ描いていない棚も、読み損ねた棚も含む。
 */
async function readAllShelfItems(request: APIRequestContext): Promise<HomeShelfItems[]> {
  const keys = (await readHome(request)).shelves.map((shelf) => shelf.key);
  const all: HomeShelfItems[] = [];
  for (let start = 0; start < keys.length; start += 8) {
    const batch = keys.slice(start, start + 8).join(",");
    const response = await request.get(`/api/home/shelves?keys=${batch}`);
    expect(response.status()).toBe(200);
    all.push(...((await response.json()) as { shelves: HomeShelfItems[] }).shelves);
  }
  return all;
}

/** shelfVideoIds は棚の項目の動画と、グループの子孫の動画の id である。 */
function shelfVideoIds(shelf: HomeShelfItems): number[] {
  return shelf.items.flatMap((item) => [
    ...(item.video === undefined ? [] : [item.video.id]),
    ...(item.group?.videoIds ?? []),
  ]);
}

async function setProgress(
  request: APIRequestContext,
  title: string,
  positionMs: number,
) {
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
    previousShelfIds = (await readHome(request)).shelves.map((shelf) => shelf.folderId);
    const folder = await request.post("/api/smart-folders", {
      headers: mutationHeaders,
      data: { name: shelfName, rule: { q: query }, sort: "addedDesc" },
    });
    expect(folder.status()).toBe(201);
    shelfId = ((await folder.json()) as { id: number }).id;
    const saved = await request.put("/api/home/shelves", {
      headers: mutationHeaders,
      data: { folderIds: [shelfId] },
    });
    expect(saved.status()).toBe(204);
    await setProgress(request, inProgress, 0);
  });

  test.afterAll(async ({ request }) => {
    if (videos.size > 0) {
      await setPublic(request, allTitles, false);
      await setProgress(request, inProgress, 0);
    }
    if (shelfId !== undefined) {
      const restored = await request.put("/api/home/shelves", {
        headers: mutationHeaders,
        data: { folderIds: previousShelfIds },
      });
      expect(restored.status()).toBe(204);
      const removed = await request.delete(`/api/smart-folders/${String(shelfId)}`, {
        headers: mutationHeaders,
      });
      expect(removed.ok()).toBe(true);
    }
    if (mediaFolder !== undefined) await removeOwnMediaFolder(request);
  });

  test("1: トップを開くとホームが出て、棚の動画を1回押すと再生が始まる", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(page).toHaveURL("/");
    const recent = shelfRow(page, shelfName);
    const card = recent.getByRole("link", { name: inProgress, exact: true });
    await expect(card).toBeVisible();

    await card.click();
    await expect(page).toHaveURL(
      `/videos/${String(video(inProgress).id)}?smartFolder=${String(shelfId)}`,
    );
    await page.waitForFunction(
      () => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      },
      undefined,
      { timeout: 30_000 },
    );
  });

  test("2: 検索しても棚を残し、その下の一覧を絞り込む", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("region", { name: "Shelves" })).toBeVisible();
    const field = page.getByRole("searchbox", { name: "Search videos" });
    await field.fill(query);
    await field.press("Enter");

    await expect(page).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("q") === query,
    );
    await expect(page.getByRole("searchbox", { name: "Search videos" })).toHaveValue(
      query,
    );
    await expect(page.getByRole("region", { name: "Shelves" })).toBeVisible();
    await expect
      .poll(() => cardTitles(page.getByRole("region", { name: "Library", exact: true })))
      .toEqual([...allTitles].sort());
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
      // 閲覧者に見せる棚のすべてを API から読み、画面がまだ描いていない棚も確かめる。
      const shelves = await readAllShelfItems(context.request);
      const served = new Set(shelves.flatMap(shelfVideoIds));
      for (const title of publicTitles) expect(served).toContain(video(title).id);
      for (const title of privateTitles) expect(served).not.toContain(video(title).id);

      await page.goto("/");
      await showAllShelves(page);
      await expect(shelfRow(page, shelfName)).toBeVisible();
      // 画面は読み損ねた棚を残さず、描いた棚にも非公開の動画を出さない。
      await expect(
        page
          .getByRole("region", { name: "Shelves" })
          .getByRole("button", { name: "Retry", exact: true }),
      ).toHaveCount(0);
      const shown = await cardTitles(page.getByRole("region", { name: "Shelves" }));
      for (const title of publicTitles) expect(shown).toContain(title);
      for (const title of privateTitles) expect(shown).not.toContain(title);
    } finally {
      await context.close();
      await setPublic(request, publicTitles, false);
    }
  });
});

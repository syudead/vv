import {
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

// Inline smart-folder playback, search and guest visibility against the real server.

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

function folderCard(page: Page): Locator {
  return page.locator(`article[data-smart-folder-id="${String(shelfId)}"]`);
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
    const folder = await request.post("/api/smart-folders", {
      headers: mutationHeaders,
      data: { name: shelfName, rule: { q: query }, sort: "addedDesc" },
    });
    expect(folder.status()).toBe(201);
    shelfId = ((await folder.json()) as { id: number }).id;
    await setProgress(request, inProgress, 0);
  });

  test.afterAll(async ({ request }) => {
    if (videos.size > 0) {
      await setPublic(request, allTitles, false);
      await setProgress(request, inProgress, 0);
    }
    if (shelfId !== undefined) {
      const removed = await request.delete(`/api/smart-folders/${String(shelfId)}`, {
        headers: mutationHeaders,
      });
      expect(removed.ok()).toBe(true);
    }
    if (mediaFolder !== undefined) await removeOwnMediaFolder(request);
  });

  test("inline folder previews start playback in one click", async ({ page }) => {
    await page.goto("/");
    await expect(folderCard(page)).toHaveCount(1);
    await expect(page.getByRole("region", { name: "Shelves" })).toHaveCount(0);
    const preview = folderCard(page)
      .getByRole("link", { name: /^Play "/ })
      .first();
    await expect(preview).toBeVisible();
    const target = await preview.getAttribute("href");
    expect(target).toContain(`?smartFolder=${String(shelfId)}`);
    await preview.click();
    await expect(page).toHaveURL(target as string);
    await page.waitForFunction(
      () => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      },
      undefined,
      { timeout: 30_000 },
    );
  });

  test("search filters the single home list", async ({ page }) => {
    await page.goto("/");
    await expect(folderCard(page)).toBeVisible();
    const field = page.getByRole("searchbox", { name: "Search videos" });
    await field.fill(query);
    await field.press("Enter");
    await expect(page).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("q") === query,
    );
    await expect
      .poll(() => cardTitles(page.getByRole("region", { name: "Library", exact: true })))
      .toEqual([...allTitles].sort());
    await expect(folderCard(page)).toHaveCount(0);
  });

  test("guest previews contain only public videos", async ({ browser, request }) => {
    await setPublic(request, allTitles, false);
    await setPublic(request, publicTitles, true);
    const context = await guestContext(browser);
    try {
      const page = await context.newPage();
      await page.goto("/");
      const card = folderCard(page);
      await expect(card.getByRole("link", { name: /^Play "/ })).toHaveCount(2);
      for (const title of publicTitles) {
        await expect(
          card.getByRole("link", {
            name: `Play "${title}" from "${shelfName}"`,
            exact: true,
          }),
        ).toBeVisible();
      }
      for (const title of privateTitles) {
        await expect(card.getByText(title, { exact: true })).toHaveCount(0);
      }
    } finally {
      await context.close();
      await setPublic(request, publicTitles, false);
    }
  });
});

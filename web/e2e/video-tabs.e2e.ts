import {
  type APIRequestContext,
  type Browser,
  expect,
  type Page,
  test,
} from "@playwright/test";

// The tabbed video page with real layout, a real clipboard and a guest session; the tab rules,
// including the bare video's tabs and the return to Overview, are covered by VideoPage.tabs.test.tsx
// and videoTabs.test.ts.
// The server and database survive a retry, so each attempt removes what the previous one left.

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

interface Video {
  id: number;
  title: string;
  location?: { path: string };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

// The titles of generateVideoTabsFixtures in media-fixtures.mjs.
const representative = "タブ確認A";
const otherVersions = ["タブ確認B", "タブ確認C"];
const bare = "タブ確認D";
const shared = "タブ確認E";
const allTitles = [representative, ...otherVersions, bare, shared];

/** Worker replacement reloads this module and gives retries a new marker. */
const run = Date.now().toString(36);
const performer = `タブe2e出演${run}`;

const videos = new Map<string, Video>();

function mediaRoot(): string {
  const root = process.env.MDM_E2E_VIDEO_TABS_MEDIA_DIR;
  if (root === undefined)
    throw new Error("MDM_E2E_VIDEO_TABS_MEDIA_DIR is not configured");
  return root;
}

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

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

async function runScan(request: APIRequestContext) {
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

/** Read every fixture video, including versions a previous attempt bundled out of the list. */
async function readVideos(request: APIRequestContext) {
  const list = await request.get(
    `/api/videos?limit=200&query=${encodeURIComponent("タブ確認")}`,
  );
  expect(list.status()).toBe(200);
  for (const item of ((await list.json()) as { items: Video[] }).items) {
    const versions = await request.get(`/api/videos/${String(item.id)}/versions`);
    expect(versions.status()).toBe(200);
    for (const version of ((await versions.json()) as { items: Video[] }).items) {
      videos.set(version.title, version);
    }
  }
  expect([...videos.keys()].sort()).toEqual([...allTitles].sort());
}

async function put(request: APIRequestContext, url: string, data: object) {
  const response = await request.put(url, { headers: mutationHeaders, data });
  expect(response.ok()).toBe(true);
}

async function prepareLibrary(request: APIRequestContext) {
  for (const title of allTitles) {
    await put(request, `/api/videos/${String(video(title).id)}/details`, {
      displayName: "",
      workInfo: {},
    });
  }
  await put(request, "/api/video-visibility", {
    videoIds: allTitles.map((title) => video(title).id),
    public: false,
  });
  await put(request, "/api/video-visibility", {
    videoIds: [video(shared).id],
    public: true,
  });

  const bundled = await request.post("/api/video-bundles", {
    headers: mutationHeaders,
    data: {
      videoIds: [representative, ...otherVersions].map((title) => video(title).id),
      representativeId: video(representative).id,
    },
  });
  expect(bundled.ok()).toBe(true);

  const person = await request.post("/api/people", {
    headers: mutationHeaders,
    data: { name: performer },
  });
  expect(person.status()).toBe(201);
  const { id } = (await person.json()) as { id: number };
  // A code is a Work item and a performer is a People credit.
  await put(request, `/api/videos/${String(video(representative).id)}/work-info`, {
    code: `TAB-${run}`,
    people: [{ role: "performer", person: { id } }],
  });
}

async function openVideo(page: Page, title: string) {
  await page.goto(`/videos/${String(video(title).id)}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
}

function tabs(page: Page) {
  return page.getByRole("tablist", { name: "About this video" }).getByRole("tab");
}

function selectedTab(page: Page) {
  return page.getByRole("tab", { selected: true });
}

/** Controls after the title and its pencil, up to the panel, whose visible text is empty. */
function iconOnlyControls(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const title = document.querySelector("h1");
    const panel = document.querySelector('[role="tabpanel"]');
    if (title === null || panel === null) throw new Error("title or panel missing");
    const controls = document.querySelectorAll<HTMLElement>(
      'button, a[href], [role="switch"]',
    );
    return [...controls]
      .filter(
        (control) =>
          title.compareDocumentPosition(control) & Node.DOCUMENT_POSITION_FOLLOWING &&
          panel.compareDocumentPosition(control) & Node.DOCUMENT_POSITION_PRECEDING &&
          // The title's own pencil is part of the title, not an action row.
          control.getAttribute("aria-label") !== "Edit name" &&
          control.checkVisibility() &&
          control.innerText.trim() === "" &&
          // A switch's word stands beside it in the label that holds both.
          (control.closest("label")?.innerText.trim() ?? "") === "",
      )
      .map((control) => control.getAttribute("aria-label") ?? control.outerHTML);
  });
}

function pageOverflows(page: Page): Promise<boolean> {
  return page.evaluate(
    () =>
      window.scrollX !== 0 || document.documentElement.scrollWidth > window.innerWidth,
  );
}

async function asGuest(browser: Browser, check: (page: Page) => Promise<void>) {
  const context = await browser.newContext({
    storageState: { cookies: [], origins: [] },
  });
  try {
    await check(await context.newPage());
  } finally {
    await context.close();
  }
}

test.describe.serial("video page tabs", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    await removeOwnMediaFolder(request);
    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaRoot() },
    });
    expect(created.status()).toBe(201);
    await runScan(request);
    await readVideos(request);
    await prepareLibrary(request);
  });

  test.afterAll(async ({ request }) => {
    await removeOwnMediaFolder(request);
  });

  test("作品情報・出演者・3本のバージョンがある所有者の動画は6つのタブを並べて Overview を選び、題名からパネルまでにアイコンだけのボタンが無い（受け入れ条件 1）", async ({
    page,
  }) => {
    await openVideo(page, representative);
    await expect(tabs(page)).toHaveText([
      "Overview",
      "Work",
      "People1",
      "Versions3",
      "Thumbnail",
      "File & media",
    ]);
    await expect(selectedTab(page)).toHaveText("Overview");
    expect(await iconOnlyControls(page)).toEqual([]);
  });

  test("ゲストの公開動画は Overview と File & media だけを並べ、場所・タグ・公開の切り替え・編集の操作を出さない（受け入れ条件 8）", async ({
    browser,
  }) => {
    await asGuest(browser, async (page) => {
      await openVideo(page, shared);
      await expect(tabs(page)).toHaveText(["Overview", "File & media"]);
      await expect(page.getByRole("switch")).toHaveCount(0);
      await expect(page.getByRole("link", { name: "Edit details" })).toHaveCount(0);
      await expect(page.getByRole("combobox", { name: "Add tag" })).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Edit name" })).toHaveCount(0);

      await tabs(page).filter({ hasText: "File & media" }).click();
      await expect(page.getByRole("region", { name: "Media" })).toBeVisible();
      await expect(page.getByRole("region", { name: "Location" })).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Copy path" })).toHaveCount(0);
    });
  });

  test("390px の画面で、ページを横にはみ出さずにタブ列のスクロールで6つのタブすべてにたどり着ける（受け入れ条件 10）", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openVideo(page, representative);
    await expect(tabs(page)).toHaveCount(6);
    for (const tab of await tabs(page).all()) {
      await tab.click();
      await expect(tab).toHaveAttribute("aria-selected", "true");
      await expect(tab).toBeInViewport({ ratio: 1 });
      expect(await pageOverflows(page)).toBe(false);
    }
  });

  test("File & media の Copy path でそのファイルのパスがクリップボードに入る（受け入れ条件 6）", async ({
    page,
    context,
    request,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    const detail = await request.get(`/api/videos/${String(video(bare).id)}`);
    const path = ((await detail.json()) as Video).location?.path;
    expect(path).toBeDefined();

    await openVideo(page, bare);
    await tabs(page).filter({ hasText: "File & media" }).click();
    await page.getByRole("button", { name: "Copy path" }).click();
    await expect(page.getByText("Copied the path")).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(path);
  });
});

import {
  type Browser,
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";

// Verify description collapsing and overflow with real layout, links and descriptions across reloads and tabs, and
// what guests see of virtual videos and merging one into a file video; lower-level suites cover work-info behavior.
// Use per-attempt person names and clear persisted details before each retry because the server and database survive worker replacement.

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
}

interface Credit {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

/** Worker replacement reloads this module and gives retries a new marker. */
const run = Date.now().toString(36);

/** Use names distinct from other e2e fixtures. */
const haruka = `作品e2e春香${run}`;

let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();
const people = new Map<string, Credit>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function person(name: string): Credit {
  const found = people.get(name);
  if (found === undefined) throw new Error(`person not known: ${name}`);
  return found;
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

async function createCredit(
  request: APIRequestContext,
  kind: "people" | "organizations",
  data: { name: string },
): Promise<Credit> {
  const response = await request.post(`/api/${kind}`, { headers: mutationHeaders, data });
  expect(response.status()).toBe(201);
  return (await response.json()) as Credit;
}

async function setWorkInfo(request: APIRequestContext, id: number, workInfo: object) {
  const response = await request.put(`/api/videos/${String(id)}/work-info`, {
    headers: mutationHeaders,
    data: workInfo,
  });
  expect(response.ok()).toBe(true);
}

/** Clear persisted details from prior attempts and restore filename-derived titles. */
async function clearDetails(request: APIRequestContext, id: number): Promise<Video> {
  const response = await request.put(`/api/videos/${String(id)}/details`, {
    headers: mutationHeaders,
    data: { displayName: "", workInfo: {} },
  });
  expect(response.ok()).toBe(true);
  return (await response.json()) as Video;
}

function overview(page: Page) {
  return page.getByRole("tabpanel", { name: "Overview" });
}

/** The Work tab's link to `link`, after selecting the tab. */
async function workLink(page: Page, link: string) {
  await page.getByRole("tab", { name: "Work" }).click();
  return page
    .getByRole("tabpanel", { name: "Work" })
    .getByRole("link", { name: link.replace(/^http:\/\//, "") });
}

interface VirtualVideo extends Video {
  contentVersion: string;
}

/** Create a virtual video with links, saved through the details endpoint as the editor saves them. */
async function createVirtual(
  request: APIRequestContext,
  title: string,
  urls: string[],
): Promise<VirtualVideo> {
  const created = await request.post("/api/virtual-videos", {
    headers: mutationHeaders,
    data: { title },
  });
  expect(created.status()).toBe(201);
  const video = (await created.json()) as VirtualVideo;
  const saved = await request.put(`/api/videos/${String(video.id)}/details`, {
    headers: mutationHeaders,
    data: {
      displayName: title,
      workInfo: { urls },
      contentVersion: video.contentVersion,
    },
  });
  expect(saved.ok()).toBe(true);
  return (await saved.json()) as VirtualVideo;
}

async function deleteVirtual(request: APIRequestContext, video: VirtualVideo) {
  await request.delete(
    `/api/videos/${String(video.id)}?contentVersion=${encodeURIComponent(video.contentVersion)}`,
    { headers: mutationHeaders },
  );
}

async function setPublic(request: APIRequestContext, video: Video, shown: boolean) {
  const response = await request.put("/api/video-visibility", {
    headers: mutationHeaders,
    data: { videoIds: [video.id], public: shown },
  });
  expect(response.ok()).toBe(true);
}

function guestPage(browser: Browser) {
  return browser.newContext({ storageState: { cookies: [], origins: [] } });
}

async function openVideo(page: Page, id: number, title: string) {
  await page.goto(`/videos/${String(id)}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
}

test.describe.serial("work information", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const root = process.env.MDM_E2E_WORK_INFO_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_WORK_INFO_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    await runScan(request);

    // Virtual videos have no folder, so a run that stopped before deleting one must not change this count.
    const list = await request.get("/api/videos?limit=200&kind=file");
    const page = (await list.json()) as { items: Video[] };
    expect(page.items).toHaveLength(4);
    for (const item of page.items) {
      const cleared = await clearDetails(request, item.id);
      videos.set(cleared.title, cleared);
    }

    people.set(haruka, await createCredit(request, "people", { name: haruka }));
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("Edge Case: 長い説明は折りたたまれたままで、360px でページに横スクロールが出ない", async ({
    page,
    request,
  }) => {
    const b = video("作品動画B");
    const longName = `作品e2e長い名前${run}${"あ".repeat(120)}`;
    const long = await createCredit(request, "people", { name: longName });
    const paragraph = "長い説明の一文です。".repeat(30);
    const description = [
      paragraph,
      `https://example.com/${"a".repeat(240)}`,
      paragraph,
      paragraph,
    ].join("\n");
    await setWorkInfo(request, b.id, {
      code: `LONG-${"9".repeat(60)}`,
      description,
      people: [
        { role: "performer", person: { id: person(haruka).id } },
        { role: "performer", person: { id: long.id } },
      ],
    });

    await page.setViewportSize({ width: 360, height: 800 });
    await openVideo(page, b.id, b.title);
    const text = overview(page).locator("p").filter({ hasText: "長い説明の一文です。" });
    await expect(text).toBeVisible();
    await expect(page.getByRole("button", { name: "Show more" })).toBeVisible();
    expect(await text.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(
      true,
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);

    await page.getByRole("button", { name: "Show more" }).click();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.getByRole("button", { name: "Show less" }).click();
    await expect(page.getByRole("button", { name: "Show more" })).toBeVisible();
  });

  test("受け入れ条件 3: ファイルのある動画で保存した説明とリンクは、再読み込みの後も出る", async ({
    page,
    request,
  }) => {
    const c = video("作品動画C");
    const link = `${origin}/settings?from=${run}`;
    try {
      await openVideo(page, c.id, c.title);
      await page.getByRole("link", { name: "Edit" }).click();
      await page.getByRole("textbox", { name: "Description" }).fill(`説明${run}`);
      await page.getByRole("button", { name: "Add link" }).click();
      await page.getByRole("textbox", { name: "Link" }).fill(link);
      await page.getByRole("button", { name: "Save" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(c.title);

      await page.reload();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(c.title);
      await expect(overview(page).getByText(`説明${run}`)).toBeVisible();
      await expect(await workLink(page, link)).toHaveAttribute("href", link);
    } finally {
      await clearDetails(request, c.id);
    }
  });

  test("受け入れ条件 2: バーチャルな動画のリンクはファイルのある動画と同じく Work タブにあり、新しいタブで開く", async ({
    page,
    request,
  }) => {
    const link = `${origin}/settings?virtual=${run}`;
    const virtual = await createVirtual(request, `作品e2eバーチャル${run}`, [link]);
    try {
      await openVideo(page, virtual.id, virtual.title);
      await expect(page.getByText("Virtual video", { exact: true })).toBeVisible();
      // The frame opens the first link, as a file video's frame plays.
      await expect(
        page
          .locator("[data-player-frame]")
          .getByRole("link", { name: "Open on 127.0.0.1:15173" }),
      ).toHaveAttribute("href", link);
      const button = await workLink(page, link);
      const opened = page.context().waitForEvent("page");
      await button.click();
      const tab = await opened;
      await expect(tab).toHaveURL(link);
      // The virtual video page stays where it was, on the Work tab its address names.
      await expect(page).toHaveURL(`/videos/${String(virtual.id)}?tab=work`);
      await tab.close();
    } finally {
      await deleteVirtual(request, virtual);
    }
  });

  test("受け入れ条件 7: ゲストには公開のバーチャルな動画が見え、非公開のものは見えない", async ({
    browser,
    request,
  }) => {
    const link = `${origin}/settings?guest=${run}`;
    const shown = await createVirtual(request, `作品e2e公開${run}`, [link]);
    const hidden = await createVirtual(request, `作品e2e非公開${run}`, []);
    const context = await guestPage(browser);
    try {
      await setPublic(request, shown, true);
      const page = await context.newPage();
      await openVideo(page, shown.id, shown.title);
      await expect(page.getByRole("button", { name: "More" })).toHaveCount(0);

      await page.goto(`/videos/${String(hidden.id)}`);
      await expect(page.getByText("This video can't be opened")).toBeVisible();
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(0);
    } finally {
      await context.close();
      await deleteVirtual(request, shown);
      await deleteVirtual(request, hidden);
    }
  });

  test("受け入れ条件 5: タグとリンクを付けたバーチャルな動画をマージすると、再生できる 1 件が両方を持つ", async ({
    page,
    request,
  }) => {
    const d = video("作品動画D");
    const link = `${origin}/settings?merged=${run}`;
    const tagName = `作品e2eマージ${run}`;
    const virtual = await createVirtual(request, `作品e2eマージ元${run}`, [link]);
    let tagId: number | undefined;
    try {
      const tagged = await request.post("/api/video-tags", {
        headers: mutationHeaders,
        data: { videoIds: [virtual.id], action: "add", tag: { name: tagName } },
      });
      expect(tagged.ok()).toBe(true);
      tagId = ((await tagged.json()) as { tag: { id: number } }).tag.id;

      await page.setViewportSize({ width: 1280, height: 800 });
      await openVideo(page, virtual.id, virtual.title);
      await page.getByRole("button", { name: "More" }).click();
      await page.getByRole("menuitem", { name: "Merge into a file video…" }).click();
      const dialog = page.getByRole("dialog", { name: "Merge into a file video" });
      await dialog.getByRole("combobox", { name: "Search file videos" }).fill(d.title);
      await dialog.getByRole("option", { name: new RegExp(d.title) }).click();
      await dialog.getByRole("button", { name: "Merge" }).click();
      await expect(page).toHaveURL(`/videos/${String(d.id)}`);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(d.title);

      // The tag's name finds the merged video alone: the virtual video is gone.
      await page.goto("/");
      const field = page.getByRole("combobox", { name: "Search videos" });
      await field.fill(tagName);
      await field.press("Enter");
      const results = page.getByRole("region", { name: "Search results", exact: true });
      await expect(results.locator("article[data-video-id]")).toHaveCount(1);
      const card = results.locator(`article[data-video-id="${String(d.id)}"]`);
      await card.getByRole("link", { name: d.title, exact: true }).click();

      await expect(page.getByRole("heading", { level: 1 })).toHaveText(d.title);
      await expect(page.getByText(tagName, { exact: true }).first()).toBeVisible();
      await expect(await workLink(page, link)).toHaveAttribute("href", link);
      await page.locator(".vjs-big-play-button").click();
      await page.waitForFunction(() => {
        const element = document.querySelector("video");
        return element !== null && !element.paused && element.currentTime > 0.1;
      });
    } finally {
      await deleteVirtual(request, virtual);
      await clearDetails(request, d.id);
      if (tagId !== undefined)
        await request.delete(`/api/tags/${String(tagId)}`, { headers: mutationHeaders });
    }
  });
});

import {
  type APIRequestContext,
  type CDPSession,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

import { ownerStorageState } from "./owner-account";

// Exercise real pointer geometry for reorder, Make a group, and touch long-press; lower-level suites cover organizing rules.
// Fixture title prefixes isolate each scenario; see docs/design-docs/web-testing.md.

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

// Keep titles aligned with DRAG_ORGANIZE_VIDEOS in media-fixtures.mjs.
const reorderVideos = ["並べ確認の1", "並べ確認の2", "並べ確認の3"] as const;
const makeA = "作る確認のA";
const makeB = "作る確認のB";
const touchVideo = "タッチ確認の1";
const touchTarget1 = "タッチ確認の先1";
const allTitles = [...reorderVideos, makeA, makeB, touchVideo, touchTarget1];

// Include the group named after video B by Make a group.
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

/** Index fixture videos by title. */
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

/** Wait for import and thumbnail generation to finish. */
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

async function groupsNamed(request: APIRequestContext, name: string) {
  const response = await request.get(
    `/api/groups?limit=200&q=${encodeURIComponent(name)}`,
  );
  expect(response.status()).toBe(200);
  return ((await response.json()) as { groups: LibraryGroup[] }).groups.filter(
    (group) => group.name === name,
  );
}

/** Remove all matching groups, including leftovers from retries. */
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

function videoCard(page: Page, title: string): Locator {
  return page.locator("article[data-video-id]").filter({
    has: page.locator("h3", { hasText: new RegExp(`^${escapeRegExp(title)}$`) }),
  });
}

function groupCard(page: Page, name: string): Locator {
  return page.locator("article[data-group-id]").filter({
    has: page.getByRole("link", {
      name: new RegExp(`^${escapeRegExp(name)}, group of `),
    }),
  });
}

/** The drag-layer wrapper includes the drop indicator. */
function dragCard(card: Locator): Locator {
  return card.page().locator("[data-drag-card]").filter({ has: card });
}

async function cardTitles(page: Page): Promise<string[]> {
  return page
    .locator("article[data-video-id] h3")
    .evaluateAll((elements) => elements.map((element) => element.textContent ?? ""));
}

function notifications(page: Page): Locator {
  return page.getByRole("region", { name: /Notifications/ });
}

function toast(page: Page, text: string | RegExp): Locator {
  return notifications(page).getByText(text, { exact: true });
}

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

async function pointIn(locator: Locator, fx = 0.5, fy = 0.5): Promise<Point> {
  const box = await locator.boundingBox();
  if (box === null) throw new Error("the element has no box");
  return { x: box.x + box.width * fx, y: box.y + box.height * fy };
}

function preview(page: Page): Locator {
  return page.locator("[data-drag-preview]");
}

/** Move beyond the 8 px activation threshold and wait for the drag preview; mouseDrop releases it. */
async function mouseLift(card: Locator) {
  const page = card.page();
  const start = await pointIn(card.getByRole("link").first());
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.mouse.move(start.x + 12, start.y + 12, { steps: 4 });
  await expect(preview(page)).toBeVisible();
}

async function mouseMoveTo(page: Page, point: Point) {
  await page.mouse.move(point.x, point.y, { steps: 12 });
  // Trigger collision detection once more at the final position.
  await page.mouse.move(point.x + 1, point.y + 1);
}

/** Release only after the target displays the expected drop label. */
async function dropOnto(source: Locator, target: Locator, label: string) {
  const page = source.page();
  await mouseLift(source);
  await mouseMoveTo(page, await pointIn(dragCard(target), 0.5, 0.3));
  await expect(dragCard(target).getByText(label, { exact: true })).toBeVisible();
  await page.mouse.up();
}

async function menuItem(page: Page, target: string, name: string): Promise<Locator> {
  const menu = page.getByRole("menu");
  await expect(menu).toContainText(`Dropped on "${target}"`);
  return menu.getByRole("menuitem", { name, exact: true });
}

async function openGroupPage(page: Page, query: string, name: string) {
  await page.goto(librarySearch(query));
  await groupCard(page, name).getByRole("link").first().click();
  await expect(page).toHaveURL(/\/groups\/\d+$/);
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
}

/** Use CDP for single-finger gestures; Playwright touchscreen only supports taps. */
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
    // CI retries reuse the server, so remove previous groups first.
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
      // The touch activation delay is 200 ms.
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

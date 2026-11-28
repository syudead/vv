import path from "node:path";

import {
  type APIRequestContext,
  type Browser,
  type BrowserContext,
  expect,
  type Page,
  test,
} from "@playwright/test";

import { ownerAccount } from "./owner-account";

// Exercise setup with a real server, media scan and ordinary smart-folder navigation.

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

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// The titles of ONBOARDING_VIDEOS in media-fixtures.mjs.
const taggedTitle = "オンボード確認A";
const untaggedTitle = "オンボード確認B";
const videoTitles = [taggedTitle, untaggedTitle];
// The two interests picked (internal/domain/interest_catalog.json); only the Cooking video is tagged.
const cooking = "Cooking";
const travel = "Travel";
const interests = [cooking, travel];

function mediaRoot(): string {
  const root = process.env.MDM_E2E_ONBOARDING_MEDIA_DIR;
  if (root === undefined)
    throw new Error("MDM_E2E_ONBOARDING_MEDIA_DIR is not configured");
  return root;
}

/**
 * Open another browser that signs in itself, so each device has its own session; closing it leaves
 * the other suites' sessions signed in.
 */
async function deviceContext(browser: Browser): Promise<BrowserContext> {
  const context = await browser.newContext({
    storageState: { cookies: [], origins: [] },
  });
  const login = await context.request.post("/api/auth/login", {
    headers: mutationHeaders,
    data: ownerAccount,
  });
  expect(login.status()).toBe(200);
  return context;
}

async function listMediaFolders(request: APIRequestContext): Promise<MediaFolder[]> {
  const response = await request.get("/api/media-folders");
  expect(response.status()).toBe(200);
  return (await response.json()) as MediaFolder[];
}

/** Unregister this suite's media folder, including one left by an earlier attempt. */
async function removeOwnMediaFolder(request: APIRequestContext) {
  for (const folder of await listMediaFolders(request)) {
    if (folder.path !== mediaRoot()) continue;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.ok()).toBe(true);
  }
}

/** Delete the smart folders and tags of the picked interests. */
async function removeInterests(request: APIRequestContext) {
  const folders = await request.get("/api/smart-folders");
  expect(folders.status()).toBe(200);
  for (const folder of ((await folders.json()) as { smartFolders: Named[] })
    .smartFolders) {
    if (!interests.includes(folder.name)) continue;
    const response = await request.delete(`/api/smart-folders/${String(folder.id)}`, {
      headers: mutationHeaders,
    });
    expect([204, 404]).toContain(response.status());
  }
  for (const name of interests) {
    for (const tag of await tagsNamed(request, name)) {
      await request.delete(`/api/tags/${String(tag.id)}`, { headers: mutationHeaders });
    }
  }
}

async function tagsNamed(request: APIRequestContext, name: string): Promise<Named[]> {
  const response = await request.get(`/api/tags?q=${encodeURIComponent(name)}`);
  expect(response.ok()).toBe(true);
  const { items } = (await response.json()) as { items: Named[] };
  return items.filter((item) => item.name === name);
}

async function readVideos(request: APIRequestContext): Promise<Video[]> {
  const response = await request.get("/api/videos?limit=200");
  expect(response.status()).toBe(200);
  return ((await response.json()) as { items: Video[] }).items;
}

async function writeStep(request: APIRequestContext, step: string) {
  const response = await request.put("/api/onboarding", {
    headers: mutationHeaders,
    data: { step },
  });
  expect(response.status()).toBe(200);
}

/** The current step's item in the steps strip. */
function currentStep(page: Page) {
  return page
    .getByRole("navigation", { name: "Setup steps" })
    .locator('[aria-current="step"]');
}

/**
 * Pick this suite's folder in the `Add folder` dialog. Only the server's directory listing the dialog
 * reads is replaced; adding goes to the real server, as in settings.e2e.ts.
 */
async function addFolderFromPicker(page: Page) {
  const root = mediaRoot();
  await page.route("**/api/directories*", async (route) => {
    const requested = new URL(route.request().url()).searchParams.get("path");
    const body =
      requested === root
        ? { currentPath: root, parentPath: path.dirname(root), directories: [] }
        : { directories: [{ name: path.basename(root), path: root }] };
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  await page.getByRole("button", { name: "Add folder" }).click();
  const dialog = page.getByRole("dialog", { name: "Add media folder" });
  await dialog.getByRole("button", { name: path.basename(root) }).click();
  const created = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/media-folders") &&
      response.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "Add this folder" }).click();
  expect((await created).status()).toBe(201);
  await expect(page.getByRole("list", { name: "Added folders" })).toContainText(root);
}

/** Wait until this suite's videos are probed and have thumbnails. */
async function waitForVideos(request: APIRequestContext): Promise<Map<string, Video>> {
  let found = new Map<string, Video>();
  await expect
    .poll(
      async () => {
        found = new Map((await readVideos(request)).map((item) => [item.title, item]));
        return videoTitles.every((title) => {
          const item = found.get(title);
          return item?.probeState === "done" && item.thumbnailState !== "pending";
        });
      },
      { timeout: 120_000 },
    )
    .toBe(true);
  return found;
}

/** Link to an interest's folder in the smart folder list. */
function interestLink(page: Page, name: string) {
  return page
    .getByRole("table", { name: "Smart folders" })
    .getByRole("link", { name: new RegExp(`^${name}, smart folder`) });
}

test.describe.serial("onboarding", () => {
  test.beforeAll(async ({ request }) => {
    // A CI retry starts by removing what an earlier attempt left.
    await removeInterests(request);
    await removeOwnMediaFolder(request);
  });

  test.afterAll(async ({ request }) => {
    // The other suites start from a home without the interests' smart folders or this folder's videos.
    await removeInterests(request);
    await removeOwnMediaFolder(request);
    await writeStep(request, "done");
  });

  test("3・4: 動画のない状態で始め、途中で閉じて別の端末で続け、選んだ分野がタグとスマートフォルダになる", async ({
    browser,
    request,
  }) => {
    test.setTimeout(240_000);
    expect(await readVideos(request)).toEqual([]);
    expect(await listMediaFolders(request)).toEqual([]);
    // A new owner's step; auth.setup.ts sets it to done for the other suites.
    await writeStep(request, "folders");

    await test.step("4: 1台目でフォルダを足して閉じる", async () => {
      const first = await deviceContext(browser);
      const page = await first.newPage();
      await page.goto("/");
      await expect(page).toHaveURL("/welcome");
      await expect(
        page.getByRole("heading", { level: 1, name: "Where are your videos?" }),
      ).toBeVisible();
      await expect(currentStep(page)).toContainText("Media folders");
      await addFolderFromPicker(page);
      const written = page.waitForResponse(
        (response) =>
          response.url().endsWith("/api/onboarding") &&
          response.request().method() === "PUT",
      );
      await page.getByRole("button", { name: "Continue", exact: true }).click();
      expect((await written).status()).toBe(200);
      await expect(
        page.getByRole("heading", { level: 1, name: "Scan your library" }),
      ).toBeVisible();
      await first.close();
    });

    const second = await deviceContext(browser);
    try {
      const page = await second.newPage();
      await test.step("4: 2台目で別の画面を開くと、閉じた手順から再開する", async () => {
        await page.goto("/settings");
        await expect(page).toHaveURL("/welcome");
        await expect(
          page.getByRole("heading", { level: 1, name: "Scan your library" }),
        ).toBeVisible();
        await expect(currentStep(page)).toContainText("Scan");
      });

      await test.step("3: 取り込みを始め、待つ間に分野を2つ選んで終える", async () => {
        await page.getByRole("button", { name: "Scan library", exact: true }).click();
        await expect(
          page.getByRole("heading", { level: 1, name: "What do you watch?" }),
        ).toBeVisible();
        await expect(
          page.getByText(/^(Scanning: \d+ of \d+|Scan finished)$/),
        ).toBeVisible();
        const chips = page.getByRole("toolbar", { name: "Interests" });
        for (const name of interests) {
          const chip = chips.getByRole("button", { name, exact: true });
          await chip.click();
          await expect(chip).toHaveAttribute("aria-pressed", "true");
        }
        await page.getByRole("button", { name: "Finish", exact: true }).click();
        await expect(page).toHaveURL("/");
        await expect(page.getByText("Added 2 smart folders.")).toBeVisible();
      });

      const videos = await waitForVideos(request);

      await test.step("3: 分野のタグが種類 Genre のタグカードとして一覧に並ぶ", async () => {
        await page.goto("/tags");
        for (const name of interests) {
          // A tag card names its kind on its second line.
          const card = page
            .locator("a[data-tag-id]")
            .filter({ has: page.getByTitle(name, { exact: true }) });
          await expect(card).toContainText("Genre");
        }
      });

      await test.step("3: 分野の動画にタグが付くと、スマートフォルダの一覧から開くフォルダに表示される", async () => {
        // Interest folders are not favorites, so they appear here rather than in the home band.
        await page.goto("/smart-folders");
        for (const name of interests) {
          await expect(interestLink(page, name)).toBeVisible();
        }

        const [cookingTag] = await tagsNamed(request, cooking);
        expect(cookingTag).toBeDefined();
        const tagged = await request.post("/api/video-tags", {
          headers: mutationHeaders,
          data: {
            videoIds: [videos.get(taggedTitle)!.id],
            action: "add",
            tag: { name: cooking },
          },
        });
        expect(tagged.ok()).toBe(true);
        // The existing tag is applied; no second tag of the same name is created.
        expect((await tagsNamed(request, cooking)).map(({ id }) => id)).toEqual([
          cookingTag!.id,
        ]);

        await page.reload();
        await interestLink(page, cooking).click();
        await expect(page).toHaveURL(/\/smart-folders\/\d+$/);
        const folder = page.locator("main");
        await expect(
          folder.getByRole("link", { name: taggedTitle, exact: true }),
        ).toBeVisible();
        await expect(
          folder.getByRole("link", { name: untaggedTitle, exact: true }),
        ).toHaveCount(0);
      });
    } finally {
      await second.close();
    }
  });
});

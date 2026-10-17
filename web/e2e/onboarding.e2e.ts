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

// オンボード（specs/057-home、親 #1140 受け入れ条件 3・4、子 #1323）を実サーバーと実メディアに
// 通す。playwright.config.ts の onboarding プロジェクトとして、ほかの e2e がフォルダを足す前の、
// 動画のないライブラリで走る。動画は run-e2e.mjs が generateOnboardingFixtures で作る2本である。
//
// 自動タグ付けは e2e では切なので、分野の動画にタグが付くことは API でタグを付けて置き換える
// （specs/057-home/plan.md の Project Structure）。手順ごとの規則（Skip の書く段階、ゲストを送らない
// こと、失敗の表示）はページテストが確かめ、ここでは重ねない（docs/design-docs/web-testing.md）。

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
// media-fixtures.mjs の ONBOARDING_VIDEOS と同じ題名。
const videoTitles = ["オンボード確認A", "オンボード確認B"];
// 選ぶ2つの分野（internal/domain/interest_catalog.json）。
const interests = ["Cooking", "Travel"];

function mediaRoot(): string {
  const root = process.env.MDM_E2E_ONBOARDING_MEDIA_DIR;
  if (root === undefined) throw new Error("MDM_E2E_ONBOARDING_MEDIA_DIR is not configured");
  return root;
}

/**
 * deviceContext は、自分でログインした別のブラウザを開く。端末ごとに別のセッションになる。
 * ここで閉じても、ほかの e2e のセッションは続く。
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

/** removeOwnMediaFolder は、この e2e のメディアフォルダの登録を外す（前の試行の分も）。 */
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

/** removeInterests は、選んだ分野の棚（スマートフォルダ）とタグを消す。 */
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

/** currentStep は手順の帯の、今の手順の項目である。 */
function currentStep(page: Page) {
  return page
    .getByRole("navigation", { name: "Setup steps" })
    .locator('[aria-current="step"]');
}

/**
 * addFolderFromPicker は「Add folder」の選択ダイアログでこの e2e のフォルダを選んで足す。
 * ダイアログが読むサーバーのディレクトリの一覧だけを差し替え、足すのは実サーバーに送る
 * （settings.e2e.ts と同じ）。
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

/** waitForVideos は、この e2e の動画が解析とサムネイルを終えるのを待つ。 */
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

/** shelfRow はホームの、見出しが title の棚の行である。 */
function shelfRow(page: Page, title: string) {
  return page
    .locator('[data-slot="shelf-row"]')
    .filter({ has: page.getByRole("heading", { level: 2, name: title, exact: true }) });
}

test.describe.serial("onboarding", () => {
  test.beforeAll(async ({ request }) => {
    // CI のリトライに備え、前の試行が残したものを先に消す。
    await removeInterests(request);
    await removeOwnMediaFolder(request);
  });

  test.afterAll(async ({ request }) => {
    // ほかの e2e は、分野の棚もこのフォルダの動画も無いホームから始める。
    await removeInterests(request);
    await removeOwnMediaFolder(request);
    await writeStep(request, "done");
  });

  test("3・4: 動画のない状態で始め、途中で閉じて別の端末で続け、選んだ分野がタグと棚になる", async ({
    browser,
    request,
  }) => {
    test.setTimeout(240_000);
    expect(await readVideos(request)).toEqual([]);
    expect(await listMediaFolders(request)).toEqual([]);
    // 新しい所有者の段階（auth.setup.ts は既存の e2e のために done にしている）。
    await writeStep(request, "folders");

    await test.step("4: 1台目でフォルダを足して閉じる", async () => {
      const first = await deviceContext(browser);
      const page = await first.newPage();
      await page.goto("/library");
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
        await expect(page.getByText(/^(Scanning: \d+ of \d+|Scan finished)$/)).toBeVisible();
        const chips = page.getByRole("toolbar", { name: "Interests" });
        for (const name of interests) {
          const chip = chips.getByRole("button", { name, exact: true });
          await chip.click();
          await expect(chip).toHaveAttribute("aria-pressed", "true");
        }
        await page.getByRole("button", { name: "Finish", exact: true }).click();
        await expect(page).toHaveURL("/");
        await expect(page.getByText("Added 2 shelves to your home.")).toBeVisible();
      });

      const videos = await waitForVideos(request);

      await test.step("3: 分野のタグが種類 Genre でタグ管理画面に並ぶ", async () => {
        await page.goto("/tags");
        for (const name of interests) {
          const row = page
            .getByRole("link", {
              name: `Open the library filtered by ${name}`,
              exact: true,
            })
            .locator("xpath=ancestor::*[@data-tag-id][1]");
          await expect(row.getByTitle("Kind: Genre")).toBeVisible();
        }
      });

      await test.step("3: 分野の動画にタグが付くと、ホームにその分野の棚が出る", async () => {
        // 選んだ分野の棚は、中身が入るまでホームに出ない（親 #1140 Edge Cases）。
        await page.goto("/");
        await expect(page.getByRole("region", { name: "Shelves" })).toBeVisible();
        await expect(shelfRow(page, "Recently added")).toBeVisible();
        await expect(shelfRow(page, interests[0])).toHaveCount(0);

        const [cooking] = await tagsNamed(request, interests[0]);
        expect(cooking).toBeDefined();
        const tagged = await request.post("/api/video-tags", {
          headers: mutationHeaders,
          data: {
            videoIds: [videos.get(videoTitles[0])!.id],
            action: "add",
            tag: { name: interests[0] },
          },
        });
        expect(tagged.ok()).toBe(true);
        // 今あるタグに付き、同じ名前の2つ目のタグはできない。
        expect((await tagsNamed(request, interests[0])).map(({ id }) => id)).toEqual([
          cooking!.id,
        ]);

        await page.reload();
        const shelf = shelfRow(page, interests[0]);
        await expect(
          shelf.getByRole("link", { name: videoTitles[0], exact: true }),
        ).toBeVisible();
        await expect(
          shelf.getByRole("link", { name: videoTitles[1], exact: true }),
        ).toHaveCount(0);
        await expect(shelfRow(page, interests[1])).toHaveCount(0);
      });
    } finally {
      await second.close();
    }
  });
});

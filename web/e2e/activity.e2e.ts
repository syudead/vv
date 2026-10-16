import { expect, test } from "@playwright/test";

// アクティビティの画面（specs/055-activity/ui-design.md「Activity page」、子 #1244）を実際の
// サーバーとブラウザに通す。動画は run-e2e.mjs が generateActivityFixtures で作る 1 本で、
// 画面を開いたまま取り込み、その import のカードが読み直しなしに done になることを確かめる。

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const video = "activity-import-check";

test.describe.serial("activity", () => {
  let registered: MediaFolder | undefined;

  test.afterAll(async ({ request }) => {
    if (registered === undefined) return;
    const response = await request.get("/api/media-folders");
    const current = (await response.json()) as MediaFolder[];
    const folder = current.find((item) => item.id === registered?.id);
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("an imported file's import card turns done without a reload", async ({
    page,
    request,
  }) => {
    test.setTimeout(180_000);
    const root = process.env.MDM_E2E_ACTIVITY_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_ACTIVITY_MEDIA_DIR is not configured");

    await page.goto(`/activity?q=${video}`);
    await expect(page.getByRole("heading", { level: 1, name: "Activity" })).toBeVisible();
    const sidebar = page.getByRole("complementary", { name: "Main navigation" });
    await expect(sidebar.getByRole("link", { name: "Activity" })).toHaveAttribute(
      "href",
      "/activity",
    );
    await expect(page.getByText("Nothing matches these filters")).toBeVisible();

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    registered = (await created.json()) as MediaFolder;
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);

    const card = page
      .getByRole("list", { name: "Activity log" })
      .getByRole("listitem")
      .filter({ hasText: video })
      .filter({ hasText: "Import" });
    await expect(card).toHaveCount(1, { timeout: 60_000 });
    await expect(card.getByRole("img", { name: "Done" })).toBeVisible({
      timeout: 120_000,
    });
    await expect(card.getByRole("link", { name: "Open" })).toBeVisible();
  });

  test("a guest has no entry point and is sent to sign in from /activity", async ({
    browser,
  }) => {
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto("/");
    const sidebar = page.getByRole("complementary", { name: "Main navigation" });
    await expect(sidebar.getByRole("link", { name: "Library" })).toBeVisible();
    await expect(sidebar.getByRole("link", { name: "Activity" })).toHaveCount(0);

    await page.goto("/activity");
    await expect(page).toHaveURL(/\/login\?next=%2Factivity$/);
    await context.close();
  });
});

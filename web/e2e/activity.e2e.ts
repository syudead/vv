import { type APIRequestContext, expect, test } from "@playwright/test";

// アクティビティの画面（specs/055-activity/ui-design.md「Activity page」、子 #1244）を実際の
// サーバーとブラウザに通す。動画は run-e2e.mjs が generateActivityFixtures で作る 1 本で、
// 画面を開いたまま取り込み、その import のカードが読み直しなしに done になることを確かめる。
// 自動の束ねの取り消し（子 #1245）は generateActivityVersionFixtures の 2 本を判定させて確かめる。

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

interface ActivityEntry {
  id: number;
  status: string;
  outcome?: string;
  subject: { items: { label: string; videoId?: number }[] };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const video = "activity-import-check";
// generateActivityVersionFixtures が作る、指紋の規則で同じ動画になる 2 本の名前の共通部分。
const versionVideos = "activity-version";

async function removeFolder(request: APIRequestContext, registered: MediaFolder) {
  const response = await request.get("/api/media-folders");
  const current = (await response.json()) as MediaFolder[];
  const folder = current.find((item) => item.id === registered.id);
  if (folder === undefined) return;
  const removed = await request.delete(
    `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
    { headers: mutationHeaders },
  );
  expect(removed.status()).toBe(204);
}

async function listEntries(
  request: APIRequestContext,
  query: string,
): Promise<ActivityEntry[]> {
  const response = await request.get(`/api/activity?${query}`);
  expect(response.status()).toBe(200);
  return ((await response.json()) as { items: ActivityEntry[] }).items;
}

test.describe.serial("activity", () => {
  let registered: MediaFolder | undefined;
  let versionsFolder: MediaFolder | undefined;
  let judgingTurnedOn = false;

  test.afterAll(async ({ request }) => {
    if (judgingTurnedOn) {
      const off = await request.put("/api/settings/version-judging", {
        headers: mutationHeaders,
        data: { enabled: false },
      });
      expect(off.status()).toBe(200);
    }
    if (versionsFolder !== undefined) await removeFolder(request, versionsFolder);
    if (registered !== undefined) await removeFolder(request, registered);
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

  test("undoing an automatic bundle takes the version off the video page", async ({
    page,
    request,
  }) => {
    test.setTimeout(240_000);
    const root = process.env.MDM_E2E_ACTIVITY_VERSIONS_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_ACTIVITY_VERSIONS_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    versionsFolder = (await created.json()) as MediaFolder;
    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    // 指紋まで取り込み終えてから判定を入れる。入れると待っている組をすべて判定する。
    await expect
      .poll(
        async () =>
          (await listEntries(request, `feature=import&status=done&q=${versionVideos}`))
            .length,
        { timeout: 150_000 },
      )
      .toBe(2);
    const on = await request.put("/api/settings/version-judging", {
      headers: mutationHeaders,
      data: { enabled: true },
    });
    expect(on.status()).toBe(200);
    judgingTurnedOn = true;

    let bundled: ActivityEntry | undefined;
    await expect
      .poll(
        async () => {
          const judged = await listEntries(
            request,
            `feature=duplicate_check&q=${versionVideos}`,
          );
          bundled = judged.find((item) => item.outcome === "same");
          return bundled?.status;
        },
        { timeout: 60_000 },
      )
      .toBe("done");
    const videoId = bundled?.subject.items.find(
      (item) => item.videoId !== undefined,
    )?.videoId;
    expect(videoId).toBeDefined();
    const showVersions = page.getByRole("button", { name: /versions of this video/ });

    await page.goto(`/videos/${String(videoId)}`);
    await expect(showVersions).toBeVisible();

    await page.goto(`/activity?feature=duplicate_check&q=${versionVideos}`);
    const card = page
      .getByRole("list", { name: "Activity log" })
      .getByRole("listitem")
      .filter({ hasText: "Same video · bundled by rule" });
    await expect(card).toHaveCount(1);
    await card.getByRole("button", { name: "Undo" }).click();
    await expect(
      page.getByText("Undone. vv won't decide this again on its own"),
    ).toBeVisible();
    await expect(card.getByText("Undone by you")).toBeVisible();
    await expect(card.getByRole("button", { name: "Undo" })).toHaveCount(0);

    await page.goto(`/videos/${String(videoId)}`);
    await expect(page.getByRole("list", { name: "File details" })).toBeVisible();
    await expect(showVersions).toHaveCount(0);
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

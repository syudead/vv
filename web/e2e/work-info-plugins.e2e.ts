import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 作品情報プラグインで動画の題名とタグを埋める流れ（親 Issue #968、子 #1084）を実ブラウザに通す。
// プラグインは run-e2e.mjs が internal/wasmplugin/testdata/e2eworks からビルドしたもので、
// ネットワークを使わずに決まったデータで答え、必須の秘密の設定 token を宣言する。fetch は
// token を受け取ったときだけ「プラグインe2e鍵タグ」を返すので、そのタグが付けば秘密の値が
// プラグインに届いている。動画は generateWorkInfoPluginFixtures が作る 1 本である。
// プラグイン・作ったタグ・表示名はサーバーの中で他の e2e と共有するので、前後で消す。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
}

interface Plugin {
  id: number;
  name?: string;
}

interface Tag {
  id: number;
  name: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

const pluginName = "e2e-works";
const secret = "e2e-secret-value";
const videoTitle = "プラグイン動画";
const fetchedTitle = "プラグインe2e題名";
const fetchedTag = "プラグインe2eタグ";
const secretTag = "プラグインe2e鍵タグ";

let folder: MediaFolder | undefined;
let video: Video | undefined;

function current(): Video {
  if (video === undefined) throw new Error("video not indexed");
  return video;
}

async function runScan(request: APIRequestContext) {
  const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
  expect(scan.status()).toBe(202);
  const scanId = ((await scan.json()) as { id: number }).id;
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        const state = (await response.json()) as { id: number; state: string };
        return state.id === scanId && state.state !== "running" ? state.state : "running";
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

/** deletePlugins は前の試行が残したテスト用プラグインを消す。 */
async function deletePlugins(request: APIRequestContext) {
  const response = await request.get("/api/work-info-plugins");
  expect(response.ok()).toBe(true);
  const { plugins } = (await response.json()) as { plugins: Plugin[] };
  for (const plugin of plugins.filter((item) => item.name === pluginName)) {
    const removed = await request.delete(`/api/work-info-plugins/${String(plugin.id)}`, {
      headers: { Origin: origin },
    });
    expect(removed.status()).toBe(204);
  }
}

/** deleteTags は反映で作られたタグを消す。他の e2e がタグの一覧を数えるためである。 */
async function deleteTags(request: APIRequestContext) {
  const response = await request.get("/api/tags");
  expect(response.ok()).toBe(true);
  const { items } = (await response.json()) as { items: Tag[] };
  for (const tag of items.filter((item) => [fetchedTag, secretTag].includes(item.name))) {
    const removed = await request.delete(`/api/tags/${String(tag.id)}`, {
      headers: { Origin: origin },
    });
    expect(removed.ok()).toBe(true);
  }
}

function pluginRow(page: Page) {
  return page.locator("[data-plugin-id]").filter({ hasText: pluginName });
}

function fetchEntry(page: Page) {
  return page.getByRole("button", { name: "Fetch work info from a plugin" });
}

/** openVideo は動画ページを開き、プラグインの一覧を読み終えるまで待つ。 */
async function openVideo(page: Page, title: string) {
  const plugins = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/work-info-plugins" && response.ok(),
  );
  await page.goto(`/videos/${String(current().id)}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
  await plugins;
}

test.describe.serial("work-info plugins", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const root = process.env.MDM_E2E_WORK_INFO_PLUGINS_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_WORK_INFO_PLUGINS_MEDIA_DIR is not configured");
    await deletePlugins(request);
    await deleteTags(request);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    await runScan(request);

    const list = await request.get("/api/videos?limit=200");
    const { items } = (await list.json()) as { items: Video[] };
    expect(items).toHaveLength(1);
    const found = items[0];
    if (found === undefined) throw new Error("video not indexed");
    // 前の試行が付けた表示名は動画の中身に付いて残るので、消してファイル名の題名へ戻す。
    const cleared = await request.put(`/api/videos/${String(found.id)}/details`, {
      headers: mutationHeaders,
      data: { displayName: "", workInfo: {} },
    });
    expect(cleared.ok()).toBe(true);
    video = (await cleared.json()) as Video;
    expect(video.title).toBe(videoTitle);
  });

  test.afterAll(async ({ request }) => {
    await deletePlugins(request);
    await deleteTags(request);
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("設定画面で追加・設定・承認したプラグインで、検索して選んだ作品の題名とタグを反映する", async ({
    page,
  }) => {
    const file = process.env.MDM_E2E_WORK_INFO_PLUGIN;
    if (file === undefined) throw new Error("MDM_E2E_WORK_INFO_PLUGIN is not configured");

    // 設定画面でプラグインを追加し、秘密の設定値を入れる。承認はまだしない。
    await page.goto("/settings#work-info-plugins");
    await page.getByLabel("Plugin file").setInputFiles(file);
    await expect(pluginRow(page)).toContainText("Not approved");
    await pluginRow(page).getByRole("button", { name: "Settings" }).click();
    const settings = page.getByRole("dialog", { name: `Settings for ${pluginName}` });
    await settings.getByLabel("API token").fill(secret);
    await settings.getByRole("button", { name: "Save" }).click();
    await expect(settings).toBeHidden();

    // 承認の前は、動画ページに取得の入口が無い。
    await openVideo(page, videoTitle);
    await expect(fetchEntry(page)).toHaveCount(0);

    await page.goto("/settings#work-info-plugins");
    await pluginRow(page).getByRole("button", { name: "Review and approve" }).click();
    const approval = page.getByRole("dialog", { name: `Approve ${pluginName}?` });
    await approval.getByRole("button", { name: "Approve" }).click();
    await expect(approval).toBeHidden();
    await expect(pluginRow(page)).toContainText("Approved");

    // 承認の後は入口があり、ページの題名で検索して候補を 1 つ選ぶ。
    await openVideo(page, videoTitle);
    await fetchEntry(page).click();
    const dialog = page.getByRole("dialog", { name: "Fetch work info" });
    await expect(
      dialog.getByRole("textbox", { name: "Search words or the work's page URL" }),
    ).toHaveValue(videoTitle);
    await dialog.getByRole("button", { name: "Search" }).click();
    await expect(dialog.getByText("2 matches")).toBeVisible();
    await dialog.getByRole("button", { name: /プラグインe2e作品/ }).click();

    // 比較の段: 題名とタグの両方が選ばれている。
    const titleBox = dialog.getByRole("checkbox", { name: "Title" });
    const tagsBox = dialog.getByRole("checkbox", { name: "Tags" });
    await expect(titleBox).toBeChecked();
    await expect(tagsBox).toBeChecked();
    await expect(dialog.getByText("2 new")).toBeVisible();
    await dialog.getByRole("button", { name: "Apply selected" }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText("Applied to the video.")).toBeVisible();

    // 新しい表示名と、仮のタグとして付いた 2 つのタグ。鍵のタグは秘密の値が届いた印である。
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(fetchedTitle);
    for (const name of [fetchedTag, secretTag])
      await expect(
        page.getByRole("link", { name: `Filter by ${name} (tentative)` }),
      ).toBeVisible();

    // 読み直しても残る。
    await page.reload();
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(fetchedTitle);
    await expect(
      page.getByRole("link", { name: `Filter by ${secretTag} (tentative)` }),
    ).toBeVisible();
  });
});

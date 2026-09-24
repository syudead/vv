import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 再生画面のタグ（issue 268、親 Issue #193 の受け入れ条件 1・2・6・13）を実ブラウザに
// 通す。動画は run-e2e.mjs が generateTagsFixtures で作る 2 本（タグ動画A・
// タグ動画B）。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
}

interface Tag {
  id: number;
  name: string;
  synonyms: string[];
  videoCount: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
let folder: MediaFolder | undefined;
const videos = new Map<string, Video>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
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

async function createTag(request: APIRequestContext, name: string): Promise<Tag> {
  const response = await request.post("/api/tags", {
    headers: mutationHeaders,
    data: { name },
  });
  expect(response.status()).toBe(201);
  return (await response.json()) as Tag;
}

async function addSynonym(request: APIRequestContext, id: number, name: string) {
  const response = await request.post(`/api/tags/${String(id)}/synonyms`, {
    headers: mutationHeaders,
    data: { name },
  });
  expect(response.ok()).toBe(true);
}

function addInput(page: Page) {
  return page.getByRole("combobox", { name: "タグを追加" });
}

function chip(page: Page, name: string) {
  return page.locator(`[title="${name}"]`);
}

test.describe.serial("video tags", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const root = process.env.MDM_E2E_TAGS_MEDIA_DIR;
    if (root === undefined) throw new Error("MDM_E2E_TAGS_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    const scanId = ((await scan.json()) as { id: number }).id;
    await waitForScan(request, scanId);

    const list = await request.get("/api/videos?limit=200");
    const page = (await list.json()) as { items: Video[] };
    expect(page.items).toHaveLength(2);
    for (const item of page.items) videos.set(item.title, item);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("1: 新しい名前で確定すると付いて表示され、別の動画の候補にも出る", async ({
    page,
  }) => {
    const a = video("タグ動画A");
    const b = video("タグ動画B");

    await page.goto(`/videos/${String(a.id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("タグ動画A");

    const input = addInput(page);
    await input.click();
    await input.fill("e2e新規タグ");
    await page.keyboard.press("Enter");

    await expect(chip(page, "e2e新規タグ")).toBeVisible();
    await expect(input).toHaveValue("");

    // 別の動画の再生画面でも、作ったタグが候補に出る。
    await page.goto(`/videos/${String(b.id)}`);
    await addInput(page).click();
    await expect(page.getByRole("option", { name: /e2e新規タグ/ })).toBeVisible();
  });

  test("2: 外すボタンでタグが消え、再読み込みしても消えたまま", async ({
    page,
    request,
  }) => {
    const a = video("タグ動画A");
    const tag = await createTag(request, "e2e外すタグ");
    const attach = await request.post("/api/video-tags", {
      headers: mutationHeaders,
      data: { videoIds: [a.id], action: "add", tag: { id: tag.id } },
    });
    expect(attach.ok()).toBe(true);

    await page.goto(`/videos/${String(a.id)}`);
    const removeButton = page.getByRole("button", {
      name: "e2e外すタグをこの動画から外す",
    });
    await expect(removeButton).toBeVisible();
    await removeButton.click();
    await expect(chip(page, "e2e外すタグ")).toHaveCount(0);

    await page.reload();
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(chip(page, "e2e外すタグ")).toHaveCount(0);
  });

  test("6: `anime`はシノニム未登録なら別タグになり、無効な入力は確定できず理由が出る", async ({
    page,
    request,
  }) => {
    await createTag(request, "Anime");
    const a = video("タグ動画A");
    await page.goto(`/videos/${String(a.id)}`);
    const input = addInput(page);

    // 空白だけは Enter で確定できない。
    await input.click();
    await input.fill("   ");
    await page.keyboard.press("Enter");
    await expect(page.getByText("名前を入力してください")).toBeVisible();

    // 改行を含む貼り付けは取り込まない。
    await input.fill("");
    await input.evaluate((element) => {
      const data = new DataTransfer();
      data.setData("text/plain", "旅行\n2024");
      element.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
    await expect(page.getByText("改行やタブは使えません")).toBeVisible();
    await expect(input).toHaveValue("");

    // 101 文字は確定できない。
    await input.fill("あ".repeat(101));
    await expect(
      page.getByText("100 文字以内にしてください（今 101 文字）"),
    ).toBeVisible();
    await page.keyboard.press("Enter");
    await expect(chip(page, "あ".repeat(101))).toHaveCount(0);

    // `anime`はシノニム未登録の`Anime`とは別のタグとして作られる。
    await input.fill("");
    await input.fill("anime");
    await page.keyboard.press("Enter");
    await expect(chip(page, "anime")).toBeVisible();
    await expect(chip(page, "Anime")).toHaveCount(0);
  });

  test("13: シノニム登録済みの名前を入力すると、元のタグが付く", async ({
    page,
    request,
  }) => {
    const tag = await createTag(request, "e2eシノニム元");
    await addSynonym(request, tag.id, "e2eシノニムべつ");
    const b = video("タグ動画B");

    await page.goto(`/videos/${String(b.id)}`);
    const input = addInput(page);
    await input.click();
    await input.fill("e2eシノニムべつ");
    await page.keyboard.press("Enter");

    await expect(chip(page, "e2eシノニム元")).toBeVisible();
    await expect(chip(page, "e2eシノニムべつ")).toHaveCount(0);
  });

  test("矢印キーで候補を選び、Enterで付けられる（キーボードだけ）", async ({
    page,
    request,
  }) => {
    await createTag(request, "e2e候補タグ");
    const a = video("タグ動画A");
    await page.goto(`/videos/${String(a.id)}`);

    const input = addInput(page);
    await input.click();
    // 「候補」に絞ると、この動画にはまだ無いこのタグ1件だけが残る。
    await input.fill("候補");
    await expect(page.getByRole("option", { name: /e2e候補タグ/ })).toBeVisible();
    await expect(page.getByRole("option")).toHaveCount(1);
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("Enter");

    await expect(chip(page, "e2e候補タグ")).toBeVisible();
  });
});

import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

// ComfyUI で動画の1コマを編集してサムネイルにする流れ（specs/045-comfyui-thumbnails、
// 親 #942、子 #960）を、本物のサーバーとアダプタに偽の ComfyUI（fake-comfyui.mjs）を
// つないで実ブラウザに通す。個々の規則はアダプタ・app・画面のテストが持つので、ここでは
// 設定から適用・取り消しまでの流れだけを確かめる（docs/design-docs/web-testing.md）。
//
// 動画は run-e2e.mjs が generateThumbnailEditFixtures で作る1本で、色の混ざった
// testsrc2 である。偽物は一色（OUTPUT_COLOR）の画像を返すので、サムネイルがその一色か
// どうかで、生成した画像とコマから作った画像を見分ける。
//
// 接続先の設定を消すルートは無いので、ここで保存した偽物の接続先は後の e2e に残る。
// 残っても変わるのは動画ページの編集の入口が押せることだけである。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
  thumbnailUrl?: string;
  thumbnailGenerated?: boolean;
}

interface RecordedPrompt {
  prompt: Record<string, { inputs: Record<string, unknown>; _meta?: { title?: string } }>;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const title = "サムネイル編集の確認";
// fake-comfyui.mjs の OUTPUT_COLOR と同じ色。
const outputColor = [255, 0, 255] as const;
// contracts/workflow.md の既定の指示。塗って指示を空にしたときに送られる。
const defaultInstruction =
  "Remove any text, logo or object in the marked area and restore the background behind it so it matches its surroundings.";

const comfyURL = process.env.MDM_E2E_COMFYUI_URL;
let folder: MediaFolder | undefined;
let video: Video | undefined;

function fake(): string {
  if (comfyURL === undefined) throw new Error("MDM_E2E_COMFYUI_URL is not configured");
  return comfyURL;
}

function target(): Video {
  if (video === undefined) throw new Error("the video is not indexed");
  return video;
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

async function readVideo(request: APIRequestContext): Promise<Video> {
  const response = await request.get(`/api/videos/${String(target().id)}`);
  expect(response.status()).toBe(200);
  return (await response.json()) as Video;
}

/** setTiming は偽物の実行が待ち行列と実行中に留まる長さを変える。 */
async function setTiming(
  request: APIRequestContext,
  timing: { queuedMs: number; runningMs: number },
) {
  const response = await request.put(`${fake()}/__fake/timing`, { data: timing });
  expect(response.status()).toBe(200);
}

async function prompts(request: APIRequestContext): Promise<RecordedPrompt[]> {
  const response = await request.get(`${fake()}/__fake/prompts`);
  expect(response.status()).toBe(200);
  return (await response.json()) as RecordedPrompt[];
}

/** inputOf はプロンプトの、_meta.title が nodeTitle のノードの入力 input を返す。 */
function inputOf(recorded: RecordedPrompt, nodeTitle: string, input: string): unknown {
  const node = Object.values(recorded.prompt).find(
    (candidate) => candidate._meta?.title === nodeTitle,
  );
  return node?.inputs[input];
}

/**
 * thumbnailKind は img の画像を読み、偽物の一色の画像なら "generated"、そうでなければ
 * "frame" を返す。まだ読み込んでいなければ null を返す。
 */
function thumbnailKind(image: Locator): Promise<"generated" | "frame" | null> {
  return image.evaluate((element, color) => {
    const img = element as HTMLImageElement;
    if (!img.complete || img.naturalWidth === 0) return null;
    const canvas = document.createElement("canvas");
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext("2d");
    if (context === null) return null;
    context.drawImage(img, 0, 0);
    const points: [number, number][] = [
      [0.2, 0.2],
      [0.8, 0.2],
      [0.5, 0.5],
      [0.2, 0.8],
      [0.8, 0.8],
    ];
    const solid = points.every(([x, y]) => {
      const pixel = context.getImageData(
        Math.floor(x * canvas.width),
        Math.floor(y * canvas.height),
        1,
        1,
      ).data;
      return color.every((value, index) => Math.abs((pixel[index] ?? 0) - value) <= 40);
    });
    return solid ? "generated" : "frame";
  }, outputColor);
}

/** libraryCardImage はライブラリのカードの、サムネイルの URL を出す img である。 */
function libraryCardImage(page: Page, url: string | undefined): Locator {
  if (url === undefined) throw new Error("the video has no thumbnail URL");
  return page.locator(
    `article[data-video-id="${String(target().id)}"] img[src="${url}"]`,
  );
}

/**
 * expectLibraryThumbnail はライブラリのカードが kind の画像を出すまで待つ。サムネイルを
 * 作り直す間は前の画像のことがあるので、ページを読み直して確かめ直す。
 */
async function expectLibraryThumbnail(
  page: Page,
  request: APIRequestContext,
  kind: "generated" | "frame",
) {
  await expect(async () => {
    const { thumbnailUrl } = await readVideo(request);
    await page.goto(`/?q=${encodeURIComponent(title)}`);
    const image = libraryCardImage(page, thumbnailUrl);
    await image.scrollIntoViewIfNeeded();
    await expect.poll(() => thumbnailKind(image), { timeout: 5_000 }).toBe(kind);
  }).toPass({ timeout: 30_000 });
}

/** openVideo は動画ページを開き、編集の入口が押せるようになるまで待つ。 */
async function openVideo(page: Page): Promise<Locator> {
  await page.goto(`/videos/${String(target().id)}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
  const entry = page.getByRole("button", { name: "Edit thumbnail with ComfyUI" });
  await expect(entry).toBeVisible();
  await expect(entry).not.toHaveAttribute("aria-disabled", "true", { timeout: 30_000 });
  return entry;
}

/** startEdit は入口を押し、編集画面がコマを出すまで待つ。 */
async function startEdit(entry: Locator): Promise<Locator> {
  const page = entry.page();
  await entry.click();
  const editor = page.getByRole("dialog", { name: "Edit thumbnail" });
  await expect(editor).toBeVisible();
  await expect(editor.getByRole("img", { name: "Painted area" })).toBeVisible();
  return editor;
}

test.describe.serial("thumbnail edit with ComfyUI", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    const root = process.env.MDM_E2E_THUMBNAIL_EDIT_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_THUMBNAIL_EDIT_MEDIA_DIR is not configured");

    // 偽物が起動して答えるまで待つ。
    await expect
      .poll(
        async () => {
          try {
            return (await request.get(`${fake()}/system_stats`)).status();
          } catch {
            return 0;
          }
        },
        { timeout: 30_000 },
      )
      .toBe(200);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);

    await expect
      .poll(
        async () => {
          const response = await request.get(
            `/api/videos?limit=20&query=${encodeURIComponent(title)}`,
          );
          const page = (await response.json()) as { items: Video[] };
          video = page.items.find((item) => item.title === title);
          return video?.probeState === "done" && video.thumbnailState !== "pending";
        },
        { timeout: 120_000 },
      )
      .toBe(true);

    // ゲストの確かめのために公開にする。
    const published = await request.put("/api/video-visibility", {
      headers: mutationHeaders,
      data: { videoIds: [target().id], public: true },
    });
    expect(published.status()).toBe(200);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  // 受け入れ条件 1・3・5・6: 偽物を接続先として保存して確かめ、範囲を塗って実行し、
  // 状態が変わるのを見て適用する。ライブラリのカードと動画ページが、読み直しの後も
  // フルスキャンの後も偽物の画像を出す。
  test("持ち主が接続先を保存し、塗って実行し、適用した画像が読み直しとスキャンの後も残る", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    await setTiming(request, { queuedMs: 1500, runningMs: 2500 });

    await page.goto("/settings#comfyui");
    const section = page.locator("#comfyui");
    await expect(section.getByRole("heading", { name: "ComfyUI" })).toBeVisible();
    await section.getByRole("radio", { name: "My own ComfyUI" }).check();
    await section.getByLabel("ComfyUI URL").fill(fake());
    await section.getByRole("button", { name: "Save and test connection" }).click();
    await expect(section.getByText("Connected to ComfyUI.")).toBeVisible();

    const before = await readVideo(request);
    expect(before.thumbnailGenerated).not.toBe(true);
    const entry = await openVideo(page);
    const editor = await startEdit(entry);

    // 範囲を塗る。指示は空のままにし、既定の指示が送られるのを確かめる。
    const layer = editor.getByRole("img", { name: "Painted area" });
    const box = await layer.boundingBox();
    if (box === null) throw new Error("the painting layer has no box");
    await page.mouse.move(box.x + box.width * 0.3, box.y + box.height * 0.3);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width * 0.6, box.y + box.height * 0.6, {
      steps: 8,
    });
    await page.mouse.up();
    await expect(editor.getByRole("button", { name: "Undo" })).toBeEnabled();

    const sent = (await prompts(request)).length;
    await editor.getByRole("button", { name: "Generate", exact: true }).click();
    const status = editor.getByRole("status");
    await expect(status).toContainText("Waiting in the queue");
    await expect(status).toContainText("Generating");
    await expect(editor.getByRole("img", { name: "Generated" })).toBeVisible({
      timeout: 15_000,
    });

    // 偽物に届いたのは参考画像の無いワークフローで、上げたコマとマスクと既定の指示を持つ。
    const recorded = await prompts(request);
    expect(recorded).toHaveLength(sent + 1);
    const last = recorded.at(-1)!;
    expect(inputOf(last, "vv:prompt", "text")).toBe(defaultInstruction);
    expect(inputOf(last, "vv:frame", "image")).toMatch(/^vv-[0-9a-f]+-frame\.png$/);
    expect(inputOf(last, "vv:mask", "image")).toMatch(/^vv-[0-9a-f]+-mask\.png$/);
    expect(inputOf(last, "vv:reference", "image")).toBeUndefined();

    await editor.getByRole("button", { name: "Use as thumbnail" }).click();
    await expect(editor).toBeHidden();
    const generated = page.getByRole("img", { name: "Generated thumbnail" });
    await expect(generated).toBeVisible();
    await expect.poll(() => thumbnailKind(generated)).toBe("generated");
    const applied = await readVideo(request);
    expect(applied.thumbnailGenerated).toBe(true);
    expect(applied.thumbnailUrl).not.toBe(before.thumbnailUrl);

    await page.reload();
    await expect(page.getByRole("img", { name: "Generated thumbnail" })).toBeVisible();
    await expect
      .poll(() => thumbnailKind(page.getByRole("img", { name: "Generated thumbnail" })))
      .toBe("generated");
    await expectLibraryThumbnail(page, request, "generated");

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);
    expect((await readVideo(request)).thumbnailGenerated).toBe(true);
    await expectLibraryThumbnail(page, request, "generated");
    await page.goto(`/videos/${String(target().id)}`);
    await expect
      .poll(() => thumbnailKind(page.getByRole("img", { name: "Generated thumbnail" })))
      .toBe("generated");
  });

  // 受け入れ条件 8: 「Use the frame instead」でコマから作るサムネイルに戻る。
  test("Use the frame instead でコマから作るサムネイルに戻る", async ({
    page,
    request,
  }) => {
    await page.goto(`/videos/${String(target().id)}`);
    await expect(page.getByRole("img", { name: "Generated thumbnail" })).toBeVisible();
    await page.getByRole("button", { name: "Use the frame instead" }).click();
    await expect(page.getByRole("img", { name: "Generated thumbnail" })).toHaveCount(0);
    expect((await readVideo(request)).thumbnailGenerated).not.toBe(true);
    await expectLibraryThumbnail(page, request, "frame");
  });

  // 受け入れ条件 7: 中止と破棄ではサムネイルが変わらない。
  test("実行の中止と結果の破棄ではサムネイルが変わらない", async ({ page, request }) => {
    test.setTimeout(90_000);
    const before = await readVideo(request);
    expect(before.thumbnailGenerated).not.toBe(true);

    // 中止: 実行中のうちに中止する。
    await setTiming(request, { queuedMs: 500, runningMs: 60_000 });
    const entry = await openVideo(page);
    let editor = await startEdit(entry);
    await editor.getByRole("button", { name: "Generate", exact: true }).click();
    await expect(editor.getByRole("status")).toContainText("Generating", {
      timeout: 10_000,
    });
    await editor.getByRole("button", { name: "Cancel" }).click();
    await expect(
      editor.getByText("Cancelled. The thumbnail is unchanged."),
    ).toBeVisible();
    await editor.getByRole("button", { name: "Close" }).click();
    await expect(editor).toBeHidden();
    let after = await readVideo(request);
    expect(after.thumbnailGenerated).not.toBe(true);
    expect(after.thumbnailUrl).toBe(before.thumbnailUrl);

    // 破棄: 結果が出てから捨てる。
    await setTiming(request, { queuedMs: 300, runningMs: 800 });
    editor = await startEdit(entry);
    await editor.getByRole("button", { name: "Generate", exact: true }).click();
    await expect(editor.getByRole("img", { name: "Generated" })).toBeVisible({
      timeout: 15_000,
    });
    await editor.getByRole("button", { name: "Discard" }).click();
    await expect(editor).toBeHidden();
    after = await readVideo(request);
    expect(after.thumbnailGenerated).not.toBe(true);
    expect(after.thumbnailUrl).toBe(before.thumbnailUrl);
    await expect(page.getByRole("img", { name: "Generated thumbnail" })).toHaveCount(0);
    await expectLibraryThumbnail(page, request, "frame");
  });

  // 受け入れ条件 9: ゲストには編集の入口が出ない。
  test("ゲストには編集の入口が出ない", async ({ browser }) => {
    const context = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto(`/videos/${String(target().id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
    await expect(page.locator("video")).toHaveCount(1);
    await expect(
      page.getByRole("button", { name: "Edit thumbnail with ComfyUI" }),
    ).toHaveCount(0);
    await context.close();
  });
});

import path from "node:path";

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
// 設定から適用し、読み直しとスキャンの後も残るまでの流れと、実ブラウザで塗ったマスクが
// サーバーを通って偽物に届き、塗った範囲だけが変わることを確かめる
// （docs/design-docs/web-testing.md）。中止・破棄・取り消し・収め方・ゲストは
// Vitest と Go のテストが持つ。
//
// 動画は run-e2e.mjs が generateThumbnailEditFixtures で作る1本で、色の混ざった
// testsrc2 である。偽物は一色（OUTPUT_COLOR）の画像を返すので、サムネイルがその一色か
// どうかで、生成した画像とコマから作った画像を見分ける。
//
// 手元の画像をサムネイルにする流れ（specs/046-thumbnail-image-upload、
// 親 #980、子 #988）も同じ動画で確かめる。上げる画像は run-e2e.mjs が
// generateThumbnailUploadFixtures で置く一色の PNG・JPEG・WebP で、その色でサムネイルが
// 上げた画像かどうかを見分ける。3 形式を実ブラウザで読んで送れることを確かめ、縦長の PNG で
// 収め方と、関連動画・読み直し・スキャンの後も残ることを見る。関連動画は同じフォルダの2本目の
// 動画のページで見る。
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
  thumbnailImage?: "generated" | "uploaded";
}

interface RecordedPrompt {
  prompt: Record<string, { inputs: Record<string, unknown>; _meta?: { title?: string } }>;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const title = "サムネイル編集の確認";
// media-fixtures.mjs の THUMBNAIL_EDIT_NEIGHBOR と同じ題名。
const neighborTitle = "サムネイル編集の隣の動画";

type RGB = readonly [number, number, number];

// fake-comfyui.mjs の OUTPUT_COLOR と同じ色。
const outputColor: RGB = [255, 0, 255];
// media-fixtures.mjs の THUMBNAIL_UPLOADS と同じファイルと色。PNG だけ縦長である。
const uploads = {
  png: { file: "thumbnail-upload.png", color: [0, 200, 0] as RGB },
  jpeg: { file: "thumbnail-upload.jpg", color: [0, 90, 255] as RGB },
  webp: { file: "thumbnail-upload.webp", color: [255, 200, 0] as RGB },
};
type Upload = { file: string };
// 動画（testsrc2 の 320×180）の縦横比。
const videoAspect = 16 / 9;
// contracts/workflow.md の既定の指示。塗って指示を空にしたときに送られる。
const defaultInstruction =
  "Remove any text, logo or object in the marked area and restore the background behind it so it matches its surroundings.";

const comfyURL = process.env.MDM_E2E_COMFYUI_URL;
let folder: MediaFolder | undefined;
let video: Video | undefined;
let neighbor: Video | undefined;

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

/** spread は画像が一色かを確かめるときに読む、幅と高さに対する割合の点である。 */
const spread: [number, number][] = [
  [0.2, 0.2],
  [0.8, 0.2],
  [0.5, 0.5],
  [0.2, 0.8],
  [0.8, 0.8],
];

/**
 * pixelsAt は img の画像を元の大きさで読み、points（幅と高さに対する割合）の画素の RGB を返す。
 * まだ読み込んでいなければ null を返す。
 */
function pixelsAt(
  image: Locator,
  points: [number, number][],
): Promise<number[][] | null> {
  return image.evaluate((element, at) => {
    const img = element as HTMLImageElement;
    if (!img.complete || img.naturalWidth === 0) return null;
    const canvas = document.createElement("canvas");
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext("2d");
    if (context === null) return null;
    context.drawImage(img, 0, 0);
    return at.map(([x, y]) => [
      ...context
        .getImageData(Math.floor(x * canvas.width), Math.floor(y * canvas.height), 1, 1)
        .data.slice(0, 3),
    ]);
  }, points);
}

function near(pixel: number[], color: RGB): boolean {
  return color.every((value, index) => Math.abs((pixel[index] ?? 0) - value) <= 40);
}

/** solidIn は img の画像が一色 color なら true を返す。まだ読み込んでいなければ null を返す。 */
async function solidIn(image: Locator, color: RGB): Promise<boolean | null> {
  const pixels = await pixelsAt(image, spread);
  return pixels === null ? null : pixels.every((pixel) => near(pixel, color));
}

/**
 * thumbnailKind は img の画像を読み、偽物の一色の画像なら "generated"、そうでなければ
 * "frame" を返す。まだ読み込んでいなければ null を返す。
 */
async function thumbnailKind(image: Locator): Promise<"generated" | "frame" | null> {
  const solid = await solidIn(image, outputColor);
  if (solid === null) return null;
  return solid ? "generated" : "frame";
}

/**
 * strokeOver は img の画像が、paintStroke で塗った線の上は偽物の色で、四隅は background の
 * 色なら true を返す。まだ読み込んでいなければ null を返す。
 */
async function strokeOver(image: Locator, background: RGB): Promise<boolean | null> {
  const pixels = await pixelsAt(image, [
    [0.45, 0.45],
    [0.1, 0.1],
    [0.9, 0.1],
    [0.1, 0.9],
    [0.9, 0.9],
  ]);
  if (pixels === null) return null;
  const [inside, ...outside] = pixels;
  return (
    inside !== undefined &&
    near(inside, outputColor) &&
    outside.every((pixel) => near(pixel, background))
  );
}

/** naturalAspect は img の画像の元の縦横比を返す。まだ読み込んでいなければ null を返す。 */
function naturalAspect(image: Locator): Promise<number | null> {
  return image.evaluate((element) => {
    const img = element as HTMLImageElement;
    if (!img.complete || img.naturalHeight === 0) return null;
    return img.naturalWidth / img.naturalHeight;
  });
}

/** libraryCardImage はライブラリのカードの、サムネイルの URL を出す img である。 */
function libraryCardImage(page: Page, url: string | undefined): Locator {
  if (url === undefined) throw new Error("the video has no thumbnail URL");
  return page.locator(
    `article[data-video-id="${String(target().id)}"] img[src="${url}"]`,
  );
}

/**
 * expectLibraryImage はライブラリのカードのサムネイルを read で読んだ値が expected になるまで
 * 待つ。サムネイルを作り直す間は前の画像のことがあるので、ページを読み直して確かめ直す。
 */
async function expectLibraryImage<T>(
  page: Page,
  request: APIRequestContext,
  read: (image: Locator) => Promise<T>,
  expected: T,
) {
  await expect(async () => {
    const { thumbnailUrl } = await readVideo(request);
    await page.goto(`/?q=${encodeURIComponent(title)}`);
    const image = libraryCardImage(page, thumbnailUrl);
    await image.scrollIntoViewIfNeeded();
    await expect.poll(() => read(image), { timeout: 5_000 }).toBe(expected);
  }).toPass({ timeout: 30_000 });
}

/** expectLibraryThumbnail はライブラリのカードが kind の画像を出すまで待つ。 */
async function expectLibraryThumbnail(
  page: Page,
  request: APIRequestContext,
  kind: "generated" | "frame",
) {
  await expectLibraryImage(page, request, thumbnailKind, kind);
}

/**
 * expectRelatedColor は2本目の動画のページの関連動画で、この動画のカードが一色 color の
 * サムネイルを出すまで待つ。
 */
async function expectRelatedColor(page: Page, request: APIRequestContext, color: RGB) {
  if (neighbor === undefined) throw new Error("the neighbouring video is not indexed");
  const neighborId = neighbor.id;
  await expect(async () => {
    const { thumbnailUrl } = await readVideo(request);
    if (thumbnailUrl === undefined) throw new Error("the video has no thumbnail URL");
    await page.goto(`/videos/${String(neighborId)}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(neighborTitle);
    const image = page.locator(`aside img[src="${thumbnailUrl}"]`);
    await image.scrollIntoViewIfNeeded();
    await expect.poll(() => solidIn(image, color), { timeout: 5_000 }).toBe(true);
  }).toPass({ timeout: 30_000 });
}

/** rescan はフルスキャンを始め、終わるまで待つ。 */
async function rescan(request: APIRequestContext) {
  const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
  expect(scan.status()).toBe(202);
  await waitForScan(request, ((await scan.json()) as { id: number }).id);
}

/** Adjust はプレビューで収め方を変える手順である。 */
type Adjust = (preview: Locator, page: Page) => Promise<void>;

/** fillFrame はプレビューを「Fill frame」（枠いっぱいに広げる）にする。 */
const fillFrame: Adjust = async (preview) => {
  await preview.getByRole("radio", { name: "Fill frame" }).click();
  await expect(preview.getByRole("slider", { name: "Zoom" })).toBeVisible();
};

/**
 * uploadFromPage は開いている動画ページの「Upload an image as thumbnail」からファイルピッカーで
 * upload を選び、開いたプレビューで adjust（無ければ既定の「Whole image」のまま）してから
 * 「Use as thumbnail」を押し、送信が 200 で答えるまで待つ。プレビューを開いただけでは何も送らない。
 */
async function uploadFromPage(page: Page, upload: Upload, adjust?: Adjust) {
  const root = process.env.MDM_E2E_THUMBNAIL_UPLOAD_DIR;
  if (root === undefined)
    throw new Error("MDM_E2E_THUMBNAIL_UPLOAD_DIR is not configured");
  const entry = page.getByRole("button", { name: "Upload an image as thumbnail" });
  await expect(entry).toBeVisible();
  await expect(entry).not.toHaveAttribute("aria-disabled", "true");
  const chooser = page.waitForEvent("filechooser");
  let posted = 0;
  const route = `/api/videos/${String(target().id)}/thumbnail-image`;
  const count = (request: { method: () => string; url: () => string }) => {
    if (request.method() === "POST" && new URL(request.url()).pathname === route)
      posted++;
  };
  page.on("request", count);
  await entry.click();
  await (await chooser).setFiles(path.join(root, upload.file));
  const preview = page.getByRole("dialog", { name: "Preview thumbnail" });
  await expect(preview).toBeVisible();
  await expect(preview.getByRole("radio", { name: "Whole image" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  if (adjust !== undefined) await adjust(preview, page);
  expect(posted).toBe(0);
  page.off("request", count);
  const sent = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === route,
  );
  await preview.getByRole("button", { name: "Use as thumbnail" }).click();
  expect((await sent).status()).toBe(200);
  await expect(preview).toBeHidden();
}

/** expectUploadedItem は動画ページの情報の行が、一色 color の上げた画像の項目を出すまで待つ。 */
async function expectUploadedItem(page: Page, color: RGB) {
  const item = page.getByRole("img", { name: "Uploaded thumbnail" });
  await expect(item).toBeVisible();
  await expect.poll(() => solidIn(item, color)).toBe(true);
}

/** startThumbnailEdit は入口を押してメニューから今のサムネイルを選び、編集画面を開く。 */
async function startThumbnailEdit(entry: Locator): Promise<Locator> {
  const page = entry.page();
  await entry.click();
  await page.getByRole("menuitem", { name: "Current thumbnail" }).click();
  const editor = page.getByRole("dialog", { name: "Edit thumbnail" });
  await expect(editor).toBeVisible();
  await expect(editor.getByRole("img", { name: "Painted area" })).toBeVisible();
  return editor;
}

/** paintStroke は塗りの層の (0.3, 0.3) から (0.6, 0.6) まで（割合）を一筆で塗る。 */
async function paintStroke(editor: Locator) {
  const page = editor.page();
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
}

/** generate は「Generate」を押し、比較に生成した画像が出るまで待つ。 */
async function generate(editor: Locator) {
  await editor.getByRole("button", { name: "Generate", exact: true }).click();
  await expect(editor.getByRole("img", { name: "Generated" })).toBeVisible({
    timeout: 15_000,
  });
}

/** setComposite は偽物がマスクを通して合成するかを変える。 */
async function setComposite(request: APIRequestContext, enabled: boolean) {
  const response = await request.put(`${fake()}/__fake/composite`, { data: { enabled } });
  expect(response.status()).toBe(200);
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

/** startEdit は入口を押してメニューからコマを選び、編集画面がコマを出すまで待つ。 */
async function startEdit(entry: Locator): Promise<Locator> {
  const page = entry.page();
  // コマの項目は画面にコマがあるとき（キャプチャのボタンが押せるとき）だけ押せる。
  await expect(
    page.getByRole("button", { name: "Use current frame as thumbnail" }),
  ).not.toHaveAttribute("aria-disabled", "true", { timeout: 30_000 });
  await entry.click();
  await page.getByRole("menuitem", { name: /^Frame at / }).click();
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

    await rescan(request);

    const indexed = async (name: string): Promise<Video | undefined> => {
      const response = await request.get(
        `/api/videos?limit=20&query=${encodeURIComponent(name)}`,
      );
      const page = (await response.json()) as { items: Video[] };
      const found = page.items.find((item) => item.title === name);
      return found?.probeState === "done" && found.thumbnailState !== "pending"
        ? found
        : undefined;
    };
    await expect
      .poll(
        async () => {
          video = await indexed(title);
          neighbor = await indexed(neighborTitle);
          return video !== undefined && neighbor !== undefined;
        },
        { timeout: 120_000 },
      )
      .toBe(true);
  });

  test.afterAll(async ({ request }) => {
    await setComposite(request, false);
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

    await page.goto("/settings/connections#comfyui");
    const section = page.locator("#comfyui");
    await expect(section.getByRole("heading", { name: "ComfyUI" })).toBeVisible();
    await section.getByRole("radio", { name: "My own ComfyUI" }).check();
    await section.getByLabel("ComfyUI URL").fill(fake());
    await section.getByRole("button", { name: "Save and test connection" }).click();
    await expect(section.getByText("Connected to ComfyUI.")).toBeVisible();

    const before = await readVideo(request);
    expect(before.thumbnailImage).toBeUndefined();
    const entry = await openVideo(page);
    const editor = await startEdit(entry);

    // 範囲を塗る。指示は空のままにし、既定の指示が送られるのを確かめる。
    await paintStroke(editor);

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
    expect(applied.thumbnailImage).toBe("generated");
    expect(applied.thumbnailUrl).not.toBe(before.thumbnailUrl);

    await page.reload();
    await expect(page.getByRole("img", { name: "Generated thumbnail" })).toBeVisible();
    await expect
      .poll(() => thumbnailKind(page.getByRole("img", { name: "Generated thumbnail" })))
      .toBe("generated");
    await expectLibraryThumbnail(page, request, "generated");

    await rescan(request);
    expect((await readVideo(request)).thumbnailImage).toBe("generated");
    await expectLibraryThumbnail(page, request, "generated");
    await page.goto(`/videos/${String(target().id)}`);
    await expect
      .poll(() => thumbnailKind(page.getByRole("img", { name: "Generated thumbnail" })))
      .toBe("generated");
  });

  // 046 受け入れ条件 1・3: ページから PNG・JPEG・WebP をそれぞれ上げると、実ブラウザで読んで
  // 描いた画像が動画ページとライブラリのカードのサムネイルになり、読み直しの後も残る。縦長の
  // PNG は「Fill frame」で動画の形に切り抜かれて帯の無いままカードを埋め、別の動画の関連動画にも
  // 出て、フルスキャンの後も残る。動画と同じ比率の JPEG・WebP は既定の「Whole image」のまま送る。
  test("3 形式の画像をページから上げるとサムネイルになり、読み直しとスキャンの後も残る", async ({
    page,
    request,
  }) => {
    test.setTimeout(180_000);
    for (const upload of [uploads.png, uploads.jpeg, uploads.webp]) {
      const before = await readVideo(request);
      await page.goto(`/videos/${String(target().id)}`);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
      await uploadFromPage(page, upload, upload === uploads.png ? fillFrame : undefined);
      await expectUploadedItem(page, upload.color);
      const after = await readVideo(request);
      expect(after.thumbnailImage).toBe("uploaded");
      expect(after.thumbnailUrl).not.toBe(before.thumbnailUrl);
      await page.reload();
      await expectUploadedItem(page, upload.color);

      await expectLibraryImage(
        page,
        request,
        (image) => solidIn(image, upload.color),
        true,
      );
      if (upload !== uploads.png) continue;

      await expectLibraryImage(
        page,
        request,
        async (image) => {
          const aspect = await naturalAspect(image);
          return aspect === null ? null : Math.abs(aspect - videoAspect) < 0.02;
        },
        true,
      );
      await expectRelatedColor(page, request, upload.color);

      await rescan(request);
      expect((await readVideo(request)).thumbnailImage).toBe("uploaded");
      await expectLibraryImage(
        page,
        request,
        (image) => solidIn(image, upload.color),
        true,
      );
      await page.goto(`/videos/${String(target().id)}`);
      await expectUploadedItem(page, upload.color);
    }
  });

  // 046 受け入れ条件 5・6: 上げた画像から始めた編集はそれを元として出し、マスク付きの実行の
  // 後は、塗った範囲の中が偽物の色、外が上げた画像の色になる。偽物はここから、ワークフローの
  // ようにマスクを通して合成する。
  test("上げた画像から編集を始め、塗った範囲だけが変わった画像を適用する", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    await setTiming(request, { queuedMs: 300, runningMs: 800 });
    await setComposite(request, true);

    await page.goto(`/videos/${String(target().id)}`);
    await uploadFromPage(page, uploads.jpeg);
    await expectUploadedItem(page, uploads.jpeg.color);

    const entry = await openVideo(page);
    const editor = await startThumbnailEdit(entry);
    await expect(editor).toContainText("Current thumbnail, saved image");
    await paintStroke(editor);

    const sent = (await prompts(request)).length;
    await generate(editor);
    const recorded = await prompts(request);
    expect(recorded).toHaveLength(sent + 1);
    expect(inputOf(recorded.at(-1)!, "vv:mask", "image")).toMatch(
      /^vv-[0-9a-f]+-mask\.png$/,
    );

    const source = editor.getByRole("img", { name: "Thumbnail" });
    await expect.poll(() => solidIn(source, uploads.jpeg.color)).toBe(true);
    const result = editor.getByRole("img", { name: "Generated" });
    await expect.poll(() => strokeOver(result, uploads.jpeg.color)).toBe(true);

    await editor.getByRole("button", { name: "Use as thumbnail" }).click();
    await expect(editor).toBeHidden();
    await expect(page.getByRole("img", { name: "Generated thumbnail" })).toBeVisible();
    expect((await readVideo(request)).thumbnailImage).toBe("generated");
  });
});

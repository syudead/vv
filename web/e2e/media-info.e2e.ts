import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 動画ページの追加の情報のまとまり（親 Issue #1227 の受け入れ条件 2 と 4、子 #1252）を、本物の
// ffprobe の取り込みから実ブラウザまで通す。動画は run-e2e.mjs が generateMediaInfoFixtures で作る
// 「追加情報動画」の 1 本で、言語の印の違う音声 2 本と、題名・作者・ジャンルの埋め込みのタグを持つ。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  mediaInfo?: { audioTracks?: unknown[] };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const videoTitle = "追加情報動画";

let folder: MediaFolder | undefined;
let video: Video | undefined;

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

function groupItems(page: Page, name: string) {
  return page.getByRole("list", { name, exact: true }).getByRole("listitem");
}

test.describe.serial("media information", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const root = process.env.MDM_E2E_MEDIA_INFO_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_MEDIA_INFO_MEDIA_DIR is not configured");
    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    await runScan(request);

    const list = await request.get("/api/videos?limit=200");
    const { items } = (await list.json()) as { items: Video[] };
    video = items.find((item) => item.title === videoTitle);
    if (video === undefined) throw new Error("video not indexed");
    // 解析の job が終わり、詳細に mediaInfo が載るまで待つ。一覧の項目には載らない。
    expect(video.mediaInfo).toBeUndefined();
    const id = video.id;
    await expect
      .poll(
        async () => {
          const detail = await request.get(`/api/videos/${String(id)}`);
          return ((await detail.json()) as Video).mediaInfo?.audioTracks?.length ?? 0;
        },
        { timeout: 60_000 },
      )
      .toBe(2);
  });

  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("2 本の音声は言語の名前で、埋め込みのタグはラベル付きで技術情報の行の下に出る", async ({
    page,
  }) => {
    if (video === undefined) throw new Error("video not indexed");
    await page.goto(`/videos/${String(video.id)}`);
    // 埋め込みの題名はページの題名を置き換えない（要件 10）。
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(videoTitle);

    await expect(groupItems(page, "Audio")).toHaveText([
      "1 · AAC · Stereo · Japanese · Default",
      "2 · AAC · Stereo · English · Commentary",
    ]);
    await expect(groupItems(page, "Embedded")).toHaveText([
      /^Title\s*埋め込みの題名$/,
      /^Author\s*E2E Author$/,
      /^Genre\s*Documentary$/,
    ]);
    await expect(groupItems(page, "Video").first()).toHaveText("SDR");
    await expect(page.getByRole("list", { name: "Subtitles" })).toHaveCount(0);
    await expect(page.getByRole("list", { name: "Broadcast" })).toHaveCount(0);

    // まとまりは技術情報の行の後に並ぶ。
    const technical = page.getByRole("list", { name: "Technical details" });
    const audio = page.getByRole("list", { name: "Audio", exact: true });
    const technicalBox = await technical.boundingBox();
    const audioBox = await audio.boundingBox();
    expect(technicalBox).not.toBeNull();
    expect(audioBox).not.toBeNull();
    expect(audioBox!.y).toBeGreaterThan(technicalBox!.y);
  });
});

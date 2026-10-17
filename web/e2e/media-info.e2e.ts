import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// 動画ページの追加の情報のまとまり（親 Issue #1227 の受け入れ条件 2 と 4、子 #1252）を、本物の
// ffprobe の取り込みから実ブラウザまで通す。動画は run-e2e.mjs が generateMediaInfoFixtures で作る
// 「追加情報動画」の 1 本で、言語の印の違う音声 2 本と、題名・作者・ジャンルの埋め込みのタグを持つ。
// プレーヤーのチャプター（受け入れ条件 5、子 #1253）は同じフォルダーの「チャプター動画」で確かめる。

interface MediaFolder {
  id: number;
  version: number;
}

interface Video {
  id: number;
  title: string;
  mediaInfo?: { audioTracks?: unknown[]; chapters?: unknown[] };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const videoTitle = "追加情報動画";
const chaptersTitle = "チャプター動画";

let folder: MediaFolder | undefined;
let video: Video | undefined;
let chaptersVideo: Video | undefined;

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
    chaptersVideo = items.find((item) => item.title === chaptersTitle);
    if (video === undefined || chaptersVideo === undefined)
      throw new Error("video not indexed");
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
    const chaptersId = chaptersVideo.id;
    await expect
      .poll(
        async () => {
          const detail = await request.get(`/api/videos/${String(chaptersId)}`);
          return ((await detail.json()) as Video).mediaInfo?.chapters?.length ?? 0;
        },
        { timeout: 60_000 },
      )
      .toBe(3);
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

  test("チャプターの印にポインターを重ねるとタイトルが出て、メニューで選んだチャプターの先頭から再生する", async ({
    page,
  }) => {
    if (chaptersVideo === undefined) throw new Error("video not indexed");
    await page.setViewportSize({ width: 1280, height: 820 });
    await page.goto(`/videos/${String(chaptersVideo.id)}`);
    // 操作バーは再生を始めてから出る。始めたら止め、止めたままの選択を確かめる。
    await page.locator(".vjs-big-play-button").click();
    await expect(page.locator(".video-js")).toHaveClass(/vjs-playing/);
    await page.locator(".video-js").focus();
    await page.keyboard.press("Space");
    await expect(page.locator(".video-js")).toHaveClass(/vjs-paused/);

    // 6 秒の動画で、チャプターは 0・2・4 秒に始まる。3 つ目の終わりは長さで切られる。
    const marks = page.locator(".vjs-progress-holder .vv-chapter-mark");
    await expect(marks).toHaveCount(3);
    await page.locator(".video-js").hover();
    const holder = await page.locator(".vjs-progress-holder").boundingBox();
    if (holder === null) throw new Error("seek bar is not visible");
    const title = page.locator(".vv-seek-preview-title");
    for (const [index, expected] of [
      [1, "Chapter 2"],
      [2, "Ending"],
    ] as const) {
      const mark = await marks.nth(index).boundingBox();
      if (mark === null) throw new Error("chapter mark is not visible");
      await page.mouse.move(mark.x + mark.width + 2, holder.y + holder.height / 2, {
        steps: 5,
      });
      await expect(title).toHaveText(expected);
    }

    const button = page.locator(".vjs-control-bar > .vjs-chapters-button");
    await button.hover();
    await button.getByRole("button", { name: "Chapters" }).click();
    await expect(button.getByRole("menuitemradio")).toHaveText([
      /^0:00 Opening/,
      /^0:02 Chapter 2/,
      /^0:04 Ending/,
    ]);
    await button.getByRole("menuitemradio", { name: /^0:04 Ending/ }).click();
    await expect(page.locator(".vjs-current-time-display")).toHaveText("0:04");
    await expect(page.locator(".video-js")).toHaveClass(/vjs-paused/);

    await page.locator(".video-js").focus();
    await page.keyboard.press("Space");
    await expect(page.locator(".video-js")).toHaveClass(/vjs-playing/);
    await expect(page.locator(".vjs-current-time-display")).toHaveText(/^0:0[4-6]$/);

    // 360px では、チャプター・変換の表示・字幕がそろっても操作バーのボタンが枠の中に収まる
    // （ui-design.md「Responsive behaviour」）。隣に字幕があり、mkv は変換して再生する。
    // 6 秒の動画は数秒で終わり、終わると操作バーの中身が変わる。止めてから測り、幅を
    // 変えたあとの並べ直しが落ち着くまで測り直す（1 回だけ測ると、main で 2 回落ちた）。
    await page.keyboard.press("Space");
    await expect(page.locator(".video-js")).toHaveClass(/vjs-paused/);
    await page.setViewportSize({ width: 360, height: 800 });
    await page.locator(".video-js").hover();
    await expect(page.locator(".vjs-control-bar > .vjs-subs-caps-button")).toBeVisible();
    await expect
      .poll(() =>
        page.evaluate(() => {
          const bar = document.querySelector(".vjs-control-bar");
          if (bar === null) return null;
          const frame = bar.getBoundingClientRect();
          return Array.from(bar.querySelectorAll(":scope > .vjs-control, :scope > div"))
            .filter((child) => (child as HTMLElement).offsetWidth > 0)
            .filter((child) => !child.classList.contains("vjs-progress-control"))
            .filter((child) => child.getBoundingClientRect().right > frame.right + 0.5)
            .map((child) => child.className);
        }),
      )
      .toEqual([]);
  });
});

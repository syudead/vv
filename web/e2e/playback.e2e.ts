import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
  type Request,
} from "@playwright/test";

interface Video {
  id: number;
  title: string;
  playable: boolean;
  probeState: string;
  container?: string;
  videoCodec?: string;
  audioCodec?: string;
}

interface MediaFolder {
  id: number;
  version: number;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
const videos = new Map<string, Video>();
let folder: MediaFolder | undefined;
let sourceSnapshot = new Map<string, string>();

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

async function snapshot(directory: string): Promise<Map<string, string>> {
  const result = new Map<string, string>();
  for (const name of (await readdir(directory)).sort()) {
    const content = await readFile(path.join(directory, name));
    result.set(name, createHash("sha256").update(content).digest("hex"));
  }
  return result;
}

async function waitForScan(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/scans/current");
        if (!response.ok()) return "missing";
        return ((await response.json()) as { state: string }).state;
      },
      { timeout: 60_000 },
    )
    .toBe("done");
}

async function waitForVideos(request: APIRequestContext) {
  await expect
    .poll(
      async () => {
        const response = await request.get("/api/videos?limit=100");
        const page = (await response.json()) as { items: Video[] };
        for (const item of page.items) videos.set(item.title, item);
        return [...videos.values()].filter((item) => item.probeState === "done").length;
      },
      { timeout: 60_000 },
    )
    .toBe(12);
}

/**
 * play は動画のページを開き、大きな再生ボタンで再生を始める。
 *
 * 再生を始める前に動画の取得が失敗すると、プレイヤーの上に失敗の層（role="alert"）が
 * 出て、大きな再生ボタンは隠れたままになる（vjs-has-started）。ボタンだけを待つと、
 * テストの期限まで「ボタンが見えない」とだけ言って止まる（#256・#259 のマージ後に、
 * 配信が 404 を返したときがそうだった）。最初の読み込みの成否が決まるまで待ち、
 * 失敗の層が出たら、その文言と配信の応答を添えてすぐに失敗させる。
 */
async function play(page: Page, item: Video) {
  const mediaResponses: string[] = [];
  page.on("response", (response) => {
    const url = new URL(response.url());
    if (/^\/api\/videos\/\d+\/(stream|transcode\.mp4)$/.test(url.pathname)) {
      mediaResponses.push(`${String(response.status())} ${url.pathname}${url.search}`);
    }
  });
  await page.goto(`/videos/${String(item.id)}`);
  // 押す前に、最初の読み込み（preload=metadata）の成否が決まるのを待つ。ボタンが
  // 見えた直後に取得が失敗すると、押そうとしている間にボタンが隠れるためである。
  await page.waitForFunction(
    () => {
      const failed = document.querySelector('[data-player-frame] [role="alert"]');
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      return failed !== null || (element !== null && element.readyState >= 1);
    },
    undefined,
    { timeout: 10_000 },
  );
  const button = page.locator(".vjs-big-play-button");
  const failure = page.locator("[data-player-frame]").getByRole("alert").first();
  if (await failure.isVisible()) {
    throw new Error(
      `${item.title} (id ${String(item.id)}) の再生を始める前に失敗の層が出た: ` +
        `${(await failure.innerText()).replaceAll("\n", " ")} / 配信の応答: ` +
        `${mediaResponses.join(", ") || "なし"}`,
    );
  }
  await button.click();
  await page.waitForFunction(() => {
    const element = document.querySelector("video");
    return (
      element !== null &&
      !element.paused &&
      element.readyState >= 2 &&
      element.currentTime > 0.1
    );
  });
}

function mediaRequests(page: Page): Request[] {
  const requests: Request[] = [];
  page.on("request", (request) => {
    if (/\/api\/videos\/\d+\/(stream|transcode\.mp4)/.test(request.url())) {
      requests.push(request);
    }
  });
  return requests;
}

/**
 * reportedStart は変換の要求に付いた attempt の、実際の開始位置の報告を待って返す
 * （specs/018-live-transcode-seek/contracts/transcode-start-api.md）。
 */
async function reportedStart(page: Page, transcode: Request): Promise<number> {
  const attempt = new URL(transcode.url()).searchParams.get("attempt");
  if (attempt === null) throw new Error(`attempt が無い: ${transcode.url()}`);
  const response = await page.waitForResponse(
    (candidate) =>
      candidate.url().includes("/transcode-start?") &&
      new URL(candidate.url()).searchParams.get("attempt") === attempt,
  );
  expect(response.status()).toBe(200);
  return ((await response.json()) as { startMs: number }).startMs;
}

/**
 * pauseAndRecordProgress は再生中の動画を操作バーで止め、止まったあとの保存の位置を返す。
 * 再生中は 5 秒ごとにも保存を送る（web/src/player/VideoPlayer.tsx の saveIntervalMs）ので、
 * 押す前に待ち始めた最初の PUT が止めたときの保存とは限らない。止まったのを確かめてから、
 * 最後に送った保存の位置を使う。`lastSaved` は、そのあと（再読み込みで離れるときの保存も
 * 含む）に送った最後の位置を返す。
 */
async function pauseAndRecordProgress(page: Page, item: Video) {
  const saves: number[] = [];
  page.on("request", (candidate) => {
    if (
      candidate.url().endsWith(`/api/videos/${String(item.id)}/progress`) &&
      candidate.method() === "PUT"
    ) {
      saves.push((candidate.postDataJSON() as { positionMs: number }).positionMs);
    }
  });
  const before = saves.length;
  await page.locator(".vjs-play-control").click();
  await page.waitForFunction(() => document.querySelector("video")?.paused === true);
  await expect.poll(() => saves.length).toBeGreaterThan(before);
  const lastSaved = () => {
    const last = saves.at(-1);
    if (last === undefined) throw new Error("再生位置の保存が送られていない");
    return last;
  };
  return { saved: lastSaved(), lastSaved };
}

/** reloadAndWaitForTranscode は再読み込みのあとのライブ変換の要求と、その開始位置を返す。 */
async function reloadAndWaitForTranscode(page: Page, item: Video) {
  const resumedPromise = page.waitForRequest((candidate) =>
    candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`),
  );
  await page.reload();
  const resumed = await resumedPromise;
  return {
    resumed,
    startMs: Number(new URL(resumed.url()).searchParams.get("startMs")),
  };
}

/** frameColor は再生中の映像の左上の 1 画素の RGBA を返す。 */
async function frameColor(page: Page): Promise<number[]> {
  return page.evaluate(() => {
    const element = document.querySelector("video");
    if (element === null) throw new Error("video is missing");
    const canvas = document.createElement("canvas");
    canvas.width = 1;
    canvas.height = 1;
    const context = canvas.getContext("2d");
    if (context === null) throw new Error("2d context is missing");
    context.drawImage(element, 0, 0, 1, 1);
    return [...context.getImageData(0, 0, 1, 1).data];
  });
}

/** displayedSeconds は操作バーの現在時刻の表示（m:ss）を秒で返す。 */
async function displayedSeconds(page: Page): Promise<number> {
  const text = await page.locator(".vjs-current-time-display").innerText();
  const match = /(\d+):(\d{2})/.exec(text);
  if (match === null) throw new Error(`現在時刻を読めない: ${text}`);
  return Number(match[1]) * 60 + Number(match[2]);
}

async function saveProgress(request: APIRequestContext, item: Video, positionMs: number) {
  const response = await request.put(`/api/videos/${String(item.id)}/progress`, {
    headers: mutationHeaders,
    data: { positionMs },
  });
  expect(response.ok()).toBe(true);
}

async function throttle(page: Page, bytesPerSecond: number) {
  const session = await page.context().newCDPSession(page);
  await session.send("Network.enable");
  await session.send("Network.emulateNetworkConditions", {
    offline: false,
    latency: 0,
    downloadThroughput: bytesPerSecond,
    uploadThroughput: bytesPerSecond,
    connectionType: "wifi",
  });
}

// serial にしない。1 つの失敗で残りが走らないと、1 回の実行で 1 件ずつしか失敗が
// 分からない。準備（フォルダの登録と取り込み）は beforeAll にあり、失敗のあとに作業者が
// 入れ替わっても入れ直す。
test.describe("live MP4 playback", () => {
  test.beforeAll(async ({ request }) => {
    test.setTimeout(120_000);
    const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
    if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
    sourceSnapshot = await snapshot(mediaDir);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: mediaDir },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request);
    await waitForVideos(request);
  });

  test.afterAll(async ({ request }) => {
    const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
    if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
    expect(await snapshot(mediaDir)).toEqual(sourceSnapshot);
    if (folder !== undefined) {
      const removed = await request.delete(
        `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
        { headers: mutationHeaders },
      );
      expect(removed.status()).toBe(204);
    }
  });

  test("広い画面では左の列の中身が長くても、ページ全体はスクロールしない（1280・1920px）", async ({
    page,
  }) => {
    // 左の列の下の方にある absolute の要素（読み上げ用の sr-only など）が列の外を基準に
    // すると、列はスクロールしてもページが伸び、余計なスクロールバーと余白が出る。
    for (const viewport of [
      { width: 1280, height: 720 },
      { width: 1920, height: 950 },
    ]) {
      await page.setViewportSize(viewport);
      await page.goto(`/videos/${String(video("direct").id)}`);
      await expect(page.locator('[data-slot="detail-page-main"]')).toBeVisible();
      await expect
        .poll(() =>
          page.evaluate(() => document.documentElement.scrollHeight - window.innerHeight),
        )
        .toBe(0);
    }
  });

  test("タッチ端末は映像をタップして再生・一時停止する", async ({ browser }) => {
    const context = await browser.newContext({
      hasTouch: true,
      viewport: { width: 390, height: 800 },
    });
    const page = await context.newPage();
    try {
      await page.goto(`/videos/${String(video("direct").id)}`);
      expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(
        true,
      );
      await page.waitForFunction(
        () =>
          (document.querySelector<HTMLVideoElement>("video.vjs-tech")?.readyState ?? 0) >=
          1,
      );
      await expect(page.locator(".vjs-big-play-button")).toBeHidden();
      await page.locator("div.vjs-poster").tap();
      await expect
        .poll(() =>
          page
            .locator("video.vjs-tech")
            .evaluate((el) => !(el as HTMLVideoElement).paused),
        )
        .toBe(true);

      await page.locator("video.vjs-tech").tap();
      await expect
        .poll(() =>
          page
            .locator("video.vjs-tech")
            .evaluate((el) => (el as HTMLVideoElement).paused),
        )
        .toBe(true);
      await page.locator("video.vjs-tech").tap();
      await expect
        .poll(() =>
          page
            .locator("video.vjs-tech")
            .evaluate((el) => !(el as HTMLVideoElement).paused),
        )
        .toBe(true);
    } finally {
      await context.close();
    }
  });

  test("形式matrixをdirectまたはtranscodeの正しい経路で3秒以内に再生する", async ({
    browser,
  }) => {
    test.setTimeout(90_000);
    const matrix = [
      { title: "direct", direct: true, video: "h264", audio: "aac" },
      { title: "container-only", direct: false, video: "h264", audio: "aac" },
      { title: "container-only-mov", direct: false, video: "h264", audio: "aac" },
      { title: "video-only", direct: false, video: "mpeg4", audio: "aac" },
      { title: "audio-only", direct: false, video: "h264", audio: "flac" },
      { title: "video-audio", direct: false, video: "mpeg4", audio: "pcm_s16le" },
      { title: "silent", direct: false, video: "mpeg4", audio: undefined },
    ];

    for (const expected of matrix) {
      const page = await browser.newPage();
      const requests = mediaRequests(page);
      const item = video(expected.title);
      expect(item.playable).toBe(expected.direct);
      expect(item.videoCodec).toBe(expected.video);
      expect(item.audioCodec).toBe(expected.audio);

      await play(page, item);

      const direct = requests.filter((request) => request.url().endsWith("/stream"));
      const transcode = requests.filter((request) =>
        request.url().includes("/transcode.mp4"),
      );
      if (expected.direct) expect(direct.length).toBeGreaterThan(0);
      else expect(direct).toHaveLength(0);
      expect(transcode.length).toBe(expected.direct ? 0 : 1);
      await page.close();
    }
  });

  test("長いGOPのresumeと未buffer seekで論理時刻と映像内容を揃える", async ({
    page,
    request,
  }) => {
    test.setTimeout(30_000);
    await page.goto("/");
    await throttle(page, 128 * 1024);
    const item = video("long-gop");
    await saveProgress(request, item, 5000);
    const requests = mediaRequests(page);

    const resumed = page.waitForRequest((candidate) =>
      candidate
        .url()
        .includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=5000`),
    );
    await page.goto(`/videos/${String(item.id)}`);
    const initialRequest = await resumed;
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.currentTime > 0.1;
    });

    // 絞った回線では再生が始まるまでに 2 秒を超えることがあり、そのあいだに video.js は
    // 操作バーを隠して押せなくする（vjs-user-inactive）。人と同じく、プレイヤーの上で
    // マウスを動かして操作バーを出してから再生バーを押す。
    const playerBox = await page.locator(".video-js").boundingBox();
    if (playerBox === null) throw new Error("player is not visible");
    await page.mouse.move(
      playerBox.x + playerBox.width * 0.75,
      playerBox.y + playerBox.height / 2,
      { steps: 3 },
    );
    await expect(page.locator(".video-js")).toHaveClass(/vjs-user-active/);
    const seekBar = page.locator(".vjs-progress-control");
    const box = await seekBar.boundingBox();
    if (box === null) throw new Error("seek bar is not visible");
    const seekRequestPromise = page.waitForRequest(
      (candidate) =>
        candidate
          .url()
          .includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`) &&
        !candidate.url().endsWith("startMs=5000"),
    );
    const oldRequestStopped = page.waitForEvent("requestfailed", {
      predicate: (candidate) => candidate === initialRequest,
      timeout: 5000,
    });
    await page.mouse.click(box.x + box.width * 0.75, box.y + box.height / 2);
    const seekRequest = await seekRequestPromise;
    await oldRequestStopped;
    const seekStart = Number(new URL(seekRequest.url()).searchParams.get("startMs"));
    expect(seekStart).toBeGreaterThan(20_000);
    expect(seekStart).toBeLessThan(25_500);
    // 映像はコピーで直前のキーフレーム（20 秒）から始まる。
    const actualStart = await reportedStart(page, seekRequest);
    expect(actualStart).toBeGreaterThan(19_500);
    expect(actualStart).toBeLessThanOrEqual(seekStart);
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return (
        element !== null &&
        !element.paused &&
        element.readyState >= 2 &&
        element.currentTime > 0.1
      );
    });

    const color = await frameColor(page);
    expect(color[2]).toBeGreaterThan(color[0] ?? 255);
    expect(color[2]).toBeGreaterThan(color[1] ?? 255);

    expect(
      requests.filter((candidate) => candidate.url().includes("/stream")),
    ).toHaveLength(0);
  });

  test("コピーで始めた変換は直前のキーフレームの時刻を表示し、再読み込みで同じ場面から再開する", async ({
    page,
  }) => {
    test.setTimeout(45_000);
    const item = video("sparse-keyframes");
    expect(item.playable).toBe(false);
    await play(page, item);

    // 14 秒付近（緑の区間、直前のキーフレームは 8 秒）へシークする。
    const playerBox = await page.locator(".video-js").boundingBox();
    if (playerBox === null) throw new Error("player is not visible");
    await page.mouse.move(
      playerBox.x + playerBox.width * 0.6,
      playerBox.y + playerBox.height / 2,
      { steps: 3 },
    );
    const seekBar = page.locator(".vjs-progress-control");
    const box = await seekBar.boundingBox();
    if (box === null) throw new Error("seek bar is not visible");
    const seekRequestPromise = page.waitForRequest((candidate) =>
      candidate.url().includes(`/api/videos/${String(item.id)}/transcode.mp4?startMs=`),
    );
    await page.mouse.click(box.x + box.width * (14 / 24), box.y + box.height / 2);
    const seekRequest = await seekRequestPromise;
    const seekStart = Number(new URL(seekRequest.url()).searchParams.get("startMs"));
    expect(seekStart).toBeGreaterThan(11_000);
    expect(seekStart).toBeLessThan(16_000);
    const actualStart = await reportedStart(page, seekRequest);
    expect(actualStart).toBeGreaterThan(7_900);
    expect(actualStart).toBeLessThan(8_100);

    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.readyState >= 2;
    });
    const { saved, lastSaved } = await pauseAndRecordProgress(page, item);
    // 表示も保存も、映っている内容の時刻（キーフレーム + 再生した分）になる。
    const elementSeconds = await page
      .locator("video")
      .evaluate((element) => (element as HTMLVideoElement).currentTime);
    expect(saved).toBeGreaterThanOrEqual(actualStart);
    expect(saved).toBeLessThan(seekStart);
    expect(Math.abs(saved - (actualStart + elementSeconds * 1000))).toBeLessThan(1000);
    expect(Math.abs((await displayedSeconds(page)) - saved / 1000)).toBeLessThanOrEqual(
      1,
    );
    const paused = await frameColor(page);
    expect(paused[1]).toBeGreaterThan(paused[0] ?? 255);
    expect(paused[1]).toBeGreaterThan(paused[2] ?? 255);

    // 再読み込みすると保存した位置から始まり、同じキーフレームからの同じ場面が映る。
    const { resumed, startMs } = await reloadAndWaitForTranscode(page, item);
    expect(startMs).toBe(lastSaved());
    expect(Math.abs(startMs - saved)).toBeLessThan(1000);
    expect(await reportedStart(page, resumed)).toBe(actualStart);
    await page.locator(".vjs-big-play-button").click();
    await page.waitForFunction(() => {
      const element = document.querySelector("video");
      return element !== null && !element.paused && element.readyState >= 2;
    });
    const color = await frameColor(page);
    expect(color[1]).toBeGreaterThan(color[0] ?? 255);
    expect(color[1]).toBeGreaterThan(color[2] ?? 255);
    const shown = await displayedSeconds(page);
    expect(shown).toBeGreaterThanOrEqual(Math.floor(actualStart / 1000));
    expect(shown).toBeLessThan(seekStart / 1000);
  });

  test("360px、768px、1280pxでプレイヤーが画面の幅に収まり、ページが横にはみ出さない", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct");
    for (const width of [360, 768, 1280]) {
      await page.setViewportSize({ width, height: 800 });
      await page.goto(`/videos/${String(item.id)}`);
      await expect(page.locator(".video-js")).toBeVisible();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(width);
      const bounds = await page.locator(".video-js").boundingBox();
      expect(bounds?.x ?? -1).toBeGreaterThanOrEqual(0);
      expect((bounds?.x ?? width) + (bounds?.width ?? width + 1)).toBeLessThanOrEqual(
        width,
      );
    }
  });

  test("隣の字幕をメニューで選ぶと、サーバーが変換した字幕を cue の時刻に出す", async ({
    page,
  }) => {
    test.setTimeout(30_000);
    const item = video("direct");
    await page.setViewportSize({ width: 1280, height: 800 });
    // 操作バーは再生を始めるまで出ない（video.js）。再生を始めてから操作バーを出す。
    // cue は 0.5 秒から。
    await play(page, item);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("direct");
    await page.locator(".video-js").hover();

    const button = page.locator(".vjs-control-bar > .vjs-subs-caps-button");
    const display = page.locator(".vjs-text-track-display");
    // 再生が 0.5 秒を過ぎていれば、選んだ字幕の文字が出る。
    await button.hover();
    await page.getByRole("menuitemradio", { name: /^Default/ }).click();
    await expect(display).toContainText("Default subtitle cue");

    await button.hover();
    await page.getByRole("menuitemradio", { name: /^ja/ }).click();
    await expect(display).toContainText("日本語の字幕");
  });
});

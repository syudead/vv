import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  type Request,
  test,
} from "@playwright/test";

// 視聴履歴のうち、本物の再生でしか確かめられないもの（specs/043-watch-history、親 #792、
// 子 #864・#877）を実サーバーと実メディアに通す（docs/design-docs/web-testing.md）。再生が
// 履歴の件を作る・作らない、件から開いた動画が保存した位置から再開する、「Resume」と
// 「Start over」がその位置から再生を始める、である。動画は run-e2e.mjs が
// generateHistoryFixtures で作る 40 秒の2本で、ほかの e2e の動画とは題名で分ける。
// 削除・絞り込み・検索・日付への移動・位置の表示・ゲストの扱いは src/history/HistoryPage と
// src/auth/AuthGate のテスト、Go のストアと httpapi のテストが確かめる。
//
// バックフィル（受け入れ条件 8）と 2 タブ同時の再生はここでは用意できないので、
// specs/043-watch-history/quickstart.md の手動手順で確かめる。

interface MediaFolder {
  id: number;
  version: number;
}

interface Progress {
  positionMs: number;
  completed: boolean;
}

interface Video {
  id: number;
  title: string;
  probeState: string;
  thumbnailState: string;
  progress?: Progress;
}

interface WatchHistoryEntry {
  id: number;
  playedAt: string;
  title: string;
  video?: { id: number };
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の HISTORY_VIDEOS と同じ題名。
const titleA = "履歴の確認A";
const titleB = "履歴の確認B";
const videos = new Map<string, Video>();
// 履歴の行の読み上げ名の日（日の見出しの 2 つの部分、例 "Today, Oct 10"）。時刻は入らない。
// 日付をまたいだ実行でも落ちないよう Today と Yesterday のどちらも受ける。
const playedAt = /played (Today|Yesterday), [A-Z][a-z]{2} \d{1,2}(, \d{4})?$/;
// en の formatTime の形（例 "3:04 PM"）。履歴の一覧のどこにも出ない。
const timeOfDay = /\d{1,2}:\d{2}\s?[AP]M/;

function video(title: string): Video {
  const found = videos.get(title);
  if (found === undefined) throw new Error(`video not indexed: ${title}`);
  return found;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
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

/** historyEntries は API から履歴を全部読む（ほかの e2e の件も含む）。 */
async function historyEntries(request: APIRequestContext): Promise<WatchHistoryEntry[]> {
  const response = await request.get("/api/watch-history?limit=200");
  expect(response.status()).toBe(200);
  return ((await response.json()) as { items: WatchHistoryEntry[] }).items;
}

/**
 * saveProgress は再生 ID の無い保存で item の再生位置を positionMs にする。再生 ID が無いので
 * 履歴は増えない。
 */
async function saveProgress(request: APIRequestContext, item: Video, positionMs: number) {
  const response = await request.put(`/api/videos/${String(item.id)}/progress`, {
    headers: mutationHeaders,
    data: { positionMs },
  });
  expect(response.status()).toBe(200);
  return (await response.json()) as Progress;
}

async function savedProgress(request: APIRequestContext, item: Video) {
  const response = await request.get(`/api/videos/${String(item.id)}`);
  expect(response.status()).toBe(200);
  return ((await response.json()) as Video).progress;
}

/** historyList は履歴の画面の一覧（日のまとまりの全体）である。 */
function historyList(page: Page): Locator {
  return page.getByRole("list", { name: "Watch history" });
}

/** entryLinks は履歴の画面で、title の動画を開く行のリンクである。 */
function entryLinks(page: Page, title: string): Locator {
  return historyList(page).getByRole("link", {
    name: new RegExp(`^${escapeRegExp(title)}, `),
  });
}

/**
 * openHistory はサイドバーから履歴の画面を開き、一覧か空の表示が出るまで待つ。動画の画面には
 * サイドバーが無いので、そこからは見出しの × で戻ってから開く。
 */
async function openHistory(page: Page) {
  if (/\/videos\/\d+$/.test(new URL(page.url()).pathname)) {
    await page.getByRole("button", { name: "Close", exact: true }).click();
    await expect(page).not.toHaveURL(/\/videos\/\d+$/);
  }
  await page
    .getByRole("complementary", { name: "Main navigation" })
    .getByRole("link", { name: "History" })
    .click();
  await expect(page).toHaveURL("/history");
  await expect(
    historyList(page).or(page.getByRole("heading", { name: "No watch history" })),
  ).toBeVisible();
}

interface ProgressSave {
  positionMs: number;
  playbackId?: string;
  /** stored はサーバーが保存を受け付けた応答が返ったこと。 */
  stored: boolean;
}

/**
 * progressSaves は、そのページが item に送った再生位置の保存の本文を集め、成功の応答が
 * 返ったものに stored を立てる。
 */
function progressSaves(page: Page, item: Video) {
  const saves: ProgressSave[] = [];
  page.on("request", (candidate: Request) => {
    if (
      candidate.method() === "PUT" &&
      candidate.url().endsWith(`/api/videos/${String(item.id)}/progress`)
    ) {
      const save: ProgressSave = {
        ...(candidate.postDataJSON() as { positionMs: number; playbackId?: string }),
        stored: false,
      };
      saves.push(save);
      void candidate
        .response()
        .then((response) => {
          save.stored = response?.ok() ?? false;
        })
        .catch(() => undefined);
    }
  });
  return saves;
}

async function waitForPlayerReady(page: Page) {
  await page.waitForFunction(() => {
    const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
    return element !== null && element.readyState >= 1;
  });
}

async function currentTime(page: Page): Promise<number> {
  return page
    .locator("video.vjs-tech")
    .evaluate((element) => (element as HTMLVideoElement).currentTime);
}

/** playFor は再生を始め、そこから seconds 秒ぶん進むまで待つ。 */
async function playFor(page: Page, seconds: number, start: () => Promise<void>) {
  const from = await currentTime(page);
  await start();
  await page.waitForFunction(
    (until) => {
      const element = document.querySelector<HTMLVideoElement>("video.vjs-tech");
      return element !== null && !element.paused && element.currentTime >= until;
    },
    from + seconds,
    { timeout: 20_000 },
  );
}

/**
 * pressPlayControl は操作バーの再生・一時停止を押す。再生が数秒続くと video.js は操作バーを
 * 隠して押せなくする（vjs-user-inactive）ので、人と同じくプレイヤーの上でマウスを動かして
 * 出してから押す。
 */
async function pressPlayControl(page: Page) {
  const box = await page.locator(".video-js").boundingBox();
  if (box === null) throw new Error("player is not visible");
  await page.mouse.move(box.x + box.width * 0.25, box.y + box.height / 2);
  await page.mouse.move(box.x + box.width * 0.75, box.y + box.height / 2, { steps: 3 });
  await page.locator(".vjs-control-bar > .vjs-play-control").click();
}

async function pause(page: Page) {
  await pressPlayControl(page);
  await page.waitForFunction(
    () => document.querySelector<HTMLVideoElement>("video.vjs-tech")?.paused === true,
  );
}

/** watchVideo は動画の画面を開いて seconds 秒見てから止め、送られた保存を返す。 */
async function watchVideo(page: Page, item: Video, seconds: number) {
  const saves = progressSaves(page, item);
  await page.goto(`/videos/${String(item.id)}`);
  await waitForPlayerReady(page);
  await playFor(page, seconds, () => page.locator(".vjs-big-play-button").click());
  await pause(page);
  // 止めたときの保存（ID つき）がサーバーに受け付けられるまで待つ。要求が出ただけで戻ると、
  // 直後に API で読む位置が止める前のままのことがある。
  const pausedMs = Math.round((await currentTime(page)) * 1000);
  await expect
    .poll(() =>
      saves.some(
        (save) =>
          save.playbackId !== undefined &&
          save.stored &&
          Math.abs(save.positionMs - pausedMs) < 1000,
      ),
    )
    .toBe(true);
  return saves;
}

/** entryRows は履歴の画面で、title の動画の行（日のまとまりの 1 件）である。 */
function entryRows(page: Page, title: string): Locator {
  return historyList(page)
    .locator('[data-slot="grouped-list-item"]')
    .filter({
      has: page.getByRole("link", { name: new RegExp(`^${escapeRegExp(title)}, `) }),
    });
}

/** positionText は行の「0:06 / 0:40」の文字である。 */
function positionText(row: Locator): Locator {
  return row.getByText(/^\d+:\d{2} \/ \d+:\d{2}$/);
}

/**
 * positionBar は行の位置の行のバーである。途中の動画はサムネイルの下端にも同じ名前のバーが
 * あり、位置の行はその後ろにある。
 */
function positionBar(row: Locator): Locator {
  return row.getByRole("progressbar", { name: "Watched portion" }).last();
}

/** seconds は「0:06 / 0:40」の前半（今の位置）を秒にする。 */
function seconds(text: string): number {
  const [minutes = "", rest = ""] = text.split(" / ")[0]?.split(":") ?? [];
  return Number(minutes) * 60 + Number(rest);
}

/**
 * recordFirstPlaying は、この後に開く動画が最初に実際に再生し始めた（playing）ときの位置を
 * 覚えさせる。自動再生は play をメタデータより先に呼ぶので、paused が外れた直後の位置は
 * 続きからのシークの前の 0 のことがある。playing はメタデータで位置を当てた後に届く。
 * 画面の移り変わりはクライアント側なので、ここで付けた待ち受けは動画の画面でも生きている。
 */
async function recordFirstPlaying(page: Page) {
  await page.evaluate(() => {
    const record = window as unknown as { firstPlayingAt?: number };
    delete record.firstPlayingAt;
    document.addEventListener(
      "playing",
      (event) => {
        if (
          record.firstPlayingAt === undefined &&
          event.target instanceof HTMLVideoElement
        ) {
          record.firstPlayingAt = event.target.currentTime;
        }
      },
      { capture: true },
    );
  });
}

/** firstPlayingAt は recordFirstPlaying が覚えた位置（秒）を返し、まだ再生していなければ null。 */
async function firstPlayingAt(page: Page): Promise<number | null> {
  return page.evaluate(
    () => (window as unknown as { firstPlayingAt?: number }).firstPlayingAt ?? null,
  );
}

test.describe.serial("watch history", () => {
  let folder: MediaFolder | undefined;

  test.beforeAll(async ({ request }) => {
    test.setTimeout(180_000);
    const root = process.env.MDM_E2E_HISTORY_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_HISTORY_MEDIA_DIR is not configured");

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;
    expect(folder.id).toBeGreaterThan(0);

    const scan = await request.post("/api/scans", { headers: mutationHeaders, data: {} });
    expect(scan.status()).toBe(202);
    await waitForScan(request, ((await scan.json()) as { id: number }).id);

    await expect
      .poll(
        async () => {
          const response = await request.get(
            `/api/videos?limit=200&query=${encodeURIComponent("履歴の確認")}`,
          );
          const page = (await response.json()) as { items: Video[] };
          for (const item of page.items) videos.set(item.title, item);
          return [titleA, titleB].every((title) => {
            const found = videos.get(title);
            return found?.probeState === "done" && found.thumbnailState !== "pending";
          });
        },
        { timeout: 60_000 },
      )
      .toBe(true);

    // ほかの e2e の再生が残した件を消し、空の履歴から始める。
    const cleared = await request.delete("/api/watch-history", {
      headers: { Origin: origin },
    });
    expect(cleared.status()).toBe(204);
  });

  // 登録したフォルダは消す。失敗時の再試行で beforeAll をやり直せるようにし、後に続く e2e の
  // ライブラリにこの 2 本を残さない。
  test.afterAll(async ({ request }) => {
    if (folder === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(folder.id)}?version=${String(folder.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("動画を開いて再生せずに戻ると、履歴は増えない（受け入れ条件 2）", async ({
    page,
    request,
  }) => {
    const item = video(titleA);
    const saves = progressSaves(page, item);
    await page.goto(`/videos/${String(item.id)}`);
    await waitForPlayerReady(page);

    await openHistory(page);
    await expect(page.getByRole("heading", { name: "No watch history" })).toBeVisible();
    expect(saves.filter((save) => save.playbackId !== undefined)).toEqual([]);
    expect(await historyEntries(request)).toEqual([]);
  });

  test("数秒再生すると今日の見出しの下のいちばん上に時刻なしで出て、一時停止と再開を 2 回しても 1 件のまま（受け入れ条件 1、3）", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000);
    await watchVideo(page, video(titleB), 2);

    const item = video(titleA);
    const saves = progressSaves(page, item);
    await page.goto(`/videos/${String(item.id)}`);
    await waitForPlayerReady(page);
    await playFor(page, 2, () => page.locator(".vjs-big-play-button").click());
    for (let round = 0; round < 2; round += 1) {
      await pause(page);
      await playFor(page, 1, () => pressPlayControl(page));
    }
    await pause(page);
    await expect
      .poll(() => saves.filter((save) => save.playbackId).length)
      .toBeGreaterThan(1);
    // この再生の保存はどれも同じ再生 ID を持つ（research.md R-2）。
    const ids = new Set(saves.flatMap((save) => save.playbackId ?? []));
    expect(ids.size).toBe(1);

    await openHistory(page);
    // いちばん上のまとまりは今日の見出しで、その先頭の行が A。行に時刻は無い。
    const today = historyList(page).locator('[data-slot="grouped-list-group"]').first();
    await expect(today.getByRole("heading", { level: 2 })).toHaveText(/^Today · /);
    const firstRow = today.locator('[data-slot="grouped-list-item"]').first();
    await expect(firstRow.getByRole("link").first()).toHaveAccessibleName(
      new RegExp(`^${escapeRegExp(titleA)}, .*${playedAt.source}`),
    );
    await expect(firstRow).not.toContainText(timeOfDay);
    await expect(historyList(page)).not.toContainText(timeOfDay);
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveAccessibleName(playedAt);

    const entries = await historyEntries(request);
    expect(entries.map((entry) => entry.video?.id)).toEqual([
      video(titleA).id,
      video(titleB).id,
    ]);
  });

  test("同じ動画を 2 回目に訪れると 2 件目が増える（受け入れ条件 4）", async ({
    page,
  }) => {
    test.setTimeout(60_000);
    // 次のテストで再開を確かめるため、先頭へ戻さない 5 秒（MinResumeMs）より先まで見る。
    await watchVideo(page, video(titleA), 6);

    await openHistory(page);
    await expect(entryLinks(page, titleA)).toHaveCount(2);
    await expect(entryLinks(page, titleB)).toHaveCount(1);
    await expect(historyList(page).getByRole("link").first()).toHaveAccessibleName(
      new RegExp(`^${escapeRegExp(titleA)}, `),
    );
  });

  test("履歴の件を押すと動画が開き、保存した位置から再開する（受け入れ条件 5）", async ({
    page,
    request,
  }) => {
    const item = video(titleA);
    const saved = (await savedProgress(request, item))?.positionMs ?? 0;
    // 5 秒（internal/domain/progress.go の MinResumeMs）未満なら先頭から始まり、
    // 再開を確かめられない。
    expect(saved).toBeGreaterThanOrEqual(5000);

    await page.goto("/");
    await openHistory(page);
    await entryLinks(page, titleA).first().click();
    await expect(page).toHaveURL(`/videos/${String(item.id)}`);
    await waitForPlayerReady(page);
    await expect
      .poll(async () => Math.abs((await currentTime(page)) * 1000 - saved))
      .toBeLessThan(1000);
  });

  test("「Resume」を押すと動画が開き、出ていた位置から再生が始まる（受け入れ条件 15）", async ({
    page,
    request,
  }) => {
    const item = video(titleB);
    // B は前のテストで 2 秒だけ見たので、先頭へ戻さない 5 秒（MinResumeMs）より先の途中に置く。
    const saved = (await saveProgress(request, item, 8000)).positionMs;
    expect(saved).toBeGreaterThanOrEqual(5000);

    await page.goto("/");
    await openHistory(page);
    const row = entryRows(page, titleB).first();
    const shown = seconds((await positionText(row).textContent()) ?? "");
    expect(shown).toBe(Math.floor(saved / 1000));
    await recordFirstPlaying(page);
    await row.getByRole("link", { name: `Resume ${titleB}` }).click();
    await expect(page).toHaveURL(`/videos/${String(item.id)}`);

    // 押しただけで再生が始まり、その最初の位置は行に出ていた位置である。
    await expect.poll(() => firstPlayingAt(page), { timeout: 15_000 }).not.toBeNull();
    const startedAt = (await firstPlayingAt(page)) ?? 0;
    expect(startedAt).toBeGreaterThanOrEqual(shown);
    expect(startedAt).toBeLessThan(shown + 2);
    await pause(page);
  });

  test("見終わった件は「Start over」を出し、押すと 0 から再生が始まる（受け入れ条件 16）", async ({
    page,
    request,
  }) => {
    const item = video(titleA);
    // A を視聴済みにする。40 秒の 30 秒目は末尾 15 秒（CompletionTailMs）の内側である。
    expect((await saveProgress(request, item, 30_000)).completed).toBe(true);
    await page.goto("/");
    await openHistory(page);
    // A は 2 回見たので 2 行あり、どちらも今の位置（見終わった）を出す。
    const rows = entryRows(page, titleA);
    await expect(rows).toHaveCount(2);
    const row = rows.first();
    await expect(row.getByRole("link", { name: `Resume ${titleA}` })).toHaveCount(0);
    // 視聴済みの行のバーは満ちている。
    const bar = positionBar(row);
    await expect(bar).toHaveAttribute(
      "aria-valuenow",
      (await bar.getAttribute("aria-valuemax")) ?? "",
    );
    await recordFirstPlaying(page);
    await row.getByRole("link", { name: `Start ${titleA} over` }).click();
    await expect(page).toHaveURL(`/videos/${String(item.id)}`);

    await expect.poll(() => firstPlayingAt(page), { timeout: 15_000 }).not.toBeNull();
    expect((await firstPlayingAt(page)) ?? 99).toBeLessThan(2);
    await pause(page);
  });
});

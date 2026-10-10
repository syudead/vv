import {
  type APIRequestContext,
  type Browser,
  expect,
  type Locator,
  type Page,
  type Request,
  test,
} from "@playwright/test";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";

// 視聴履歴の流れ（specs/043-watch-history、親 #792、子 #864）を、実サーバーと実メディアに
// 通す。動画は run-e2e.mjs が generateHistoryFixtures で作る 40 秒の2本で、ほかの e2e の
// 動画とは題名で分ける。ページテストやロジックテストで済む規則（日の見出し、404 の読み直し、
// 削除の失敗など）はここで重ねない（docs/design-docs/web-testing.md の Test levels）。
//
// バックフィル（受け入れ条件 8）と 2 タブ同時の再生はここでは用意できないので、
// specs/043-watch-history/quickstart.md の手動手順で確かめる。
//
// 改訂（子 #877）の絞り込み・検索・日付への移動・位置と「Resume」「Start over」（受け入れ
// 条件 10〜16）も同じ 2 本で確かめる。実時間では数分の内の件しか作れないので、日付の一覧
// （13）の確認ではサーバーの DB の 1 件の再生時刻だけを 3 日前へずらす（backdateEntry）。
// 月の項目と別の時間帯のブラウザは quickstart.md の手動手順に残す。

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

/**
 * libraryCards はライブラリで 2 本を「最後に再生した順」に並べ、各カードの題名と進捗バーの
 * 値を並びのまま返す。履歴を消してもこれが変わらないことを確かめる（受け入れ条件 6）。
 */
async function libraryCards(page: Page) {
  await page.goto(`/?q=${encodeURIComponent("履歴の確認")}&sort=playedDesc`);
  const cards = page.locator("article[data-video-id]");
  await expect(cards).toHaveCount(2);
  return cards.evaluateAll((elements) =>
    elements.map((element) => ({
      title: element.querySelector("h3")?.textContent ?? "",
      progress:
        element.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow") ??
        null,
    })),
  );
}

/**
 * watchedCards はライブラリの「Watched」の絞り込みで出る 2 本のうちの題名を返す。格子のカードは
 * 視聴済みを題名の色でしか示さないので、視聴状態は絞り込みの結果で見る（受け入れ条件 6）。
 */
async function watchedCards(page: Page, expected: number) {
  await page.goto(
    `/?q=${encodeURIComponent("履歴の確認")}&sort=playedDesc&watch=watched`,
  );
  const cards = page.locator("article[data-video-id]");
  await expect(cards).toHaveCount(expected);
  return cards.locator("h3").allTextContents();
}

/**
 * playToEnd は動画の画面で再生を始め、末尾の数秒前へ送って最後まで再生する。終わった後の保存で
 * 視聴済みになるまで待つ（受け入れ条件 10、16 の「最後まで見た動画」）。
 */
async function playToEnd(page: Page, request: APIRequestContext, item: Video) {
  const saves = progressSaves(page, item);
  await page.goto(`/videos/${String(item.id)}`);
  await waitForPlayerReady(page);
  await playFor(page, 1, () => page.locator(".vjs-big-play-button").click());
  await page.locator("video.vjs-tech").evaluate((element) => {
    const video = element as HTMLVideoElement;
    video.currentTime = video.duration - 4;
  });
  await page.waitForFunction(
    () => document.querySelector<HTMLVideoElement>("video.vjs-tech")?.ended === true,
    undefined,
    { timeout: 20_000 },
  );
  expect(saves.filter((save) => save.playbackId).length).toBeGreaterThan(0);
  await expect
    .poll(async () => (await savedProgress(request, item))?.completed)
    .toBe(true);
}

/** watchFilter は履歴の画面の状態の切り替え（「All」「In progress」「Watched」）の 1 つである。 */
function watchFilter(page: Page, name: "All" | "In progress" | "Watched"): Locator {
  return page
    .getByRole("radiogroup", { name: "Watch status" })
    .getByRole("radio", { name, exact: true });
}

function searchField(page: Page): Locator {
  return page.getByRole("searchbox", { name: "Search titles" });
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

/** jumpItems は「Jump to date」の項目（lg からは横の欄の日と月）である。 */
function jumpItems(page: Page): Locator {
  return page.getByRole("region", { name: "Jump to date" }).getByRole("radio");
}

/** localDays は時刻（ISO 文字列）をブラウザの暦の YYYY-MM-DD にし、重複を除いて返す。 */
async function localDays(page: Page, times: string[]): Promise<string[]> {
  return page.evaluate((values) => {
    const pad = (n: number) => String(n).padStart(2, "0");
    const days = values.map((value) => {
      const date = new Date(value);
      return `${String(date.getFullYear())}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
    });
    return [...new Set(days)];
  }, times);
}

/**
 * backdateEntry はサーバーの DB で、視聴履歴の 1 件の再生時刻を playedAtMs（Unix ミリ秒）に
 * 書き換える。何日も前の視聴は e2e の実時間では作れないので、日付の一覧（受け入れ条件 13）の
 * 確認のためだけに使う。サーバーは WAL で開いているので、別の接続から書いてよい。
 */
function backdateEntry(id: number, playedAtMs: number) {
  const runRoot = process.env.MDM_E2E_RUN_ROOT;
  if (runRoot === undefined) throw new Error("MDM_E2E_RUN_ROOT is not configured");
  // playwright.config.ts の MDM_DATA_DIR と internal/store/sqlite.go の DatabaseFileName。
  const db = new DatabaseSync(path.join(runRoot, "data", "mdm.db"));
  try {
    db.exec("pragma busy_timeout = 5000");
    const result = db
      .prepare("update watch_history set played_at = ? where id = ?")
      .run(playedAtMs, id);
    expect(Number(result.changes)).toBe(1);
  } finally {
    db.close();
  }
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

async function guestPage(browser: Browser) {
  const context = await browser.newContext({
    storageState: { cookies: [], origins: [] },
  });
  return { context, page: await context.newPage() };
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

  test("1 件消しても他の件は残り、カードの進捗バー・視聴状態・最後に再生した順は変わらない（受け入れ条件 6）", async ({
    page,
    request,
  }) => {
    const item = video(titleA);
    // 消す件の動画（A）を視聴済みにして、印が残ることも確かめる。再生 ID の無い保存なので
    // 履歴は増えない。40 秒の 30 秒目は末尾 15 秒（CompletionTailMs）の内側である。
    const finished = await request.put(`/api/videos/${String(item.id)}/progress`, {
      headers: mutationHeaders,
      data: { positionMs: 30_000 },
    });
    expect(finished.status()).toBe(200);
    expect(((await finished.json()) as Progress).completed).toBe(true);
    expect((await historyEntries(request)).length).toBe(3);

    const progressBefore = await savedProgress(request, item);
    const cardsBefore = await libraryCards(page);
    expect(cardsBefore.map((card) => card.title)).toEqual([titleA, titleB]);
    // B は途中まで見たので進捗バーがあり、A は視聴済みなので一覧の「Watched」に出る。
    expect(cardsBefore[1]?.progress).not.toBeNull();
    expect(await watchedCards(page, 1)).toEqual([titleA]);

    await openHistory(page);
    // いちばん上（A の 2 回目の視聴）の行の × を押す。
    const newest = historyList(page).locator('[data-slot="grouped-list-item"]').first();
    await expect(newest.getByRole("link").first()).toHaveAccessibleName(
      new RegExp(`^${escapeRegExp(titleA)}, `),
    );
    await newest
      .getByRole("button", {
        name: new RegExp(`^Remove "${escapeRegExp(titleA)}" played `),
      })
      .click();
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(1);
    expect((await historyEntries(request)).length).toBe(2);

    expect(await savedProgress(request, item)).toEqual(progressBefore);
    expect(await libraryCards(page)).toEqual(cardsBefore);
    expect(await watchedCards(page, 1)).toEqual([titleA]);
  });

  test("全件削除は確認を出し、取り消せば何も消えず、確定すれば空の表示になる（受け入れ条件 7）", async ({
    page,
    request,
  }) => {
    await page.goto("/");
    await openHistory(page);
    // lg から全件の削除は横の欄の最後にある（lg 未満の More の奥はページテストで確かめる）。
    const openClear = async () => {
      await page
        .getByRole("region", { name: "Jump to date" })
        .getByRole("button", { name: "Clear history…" })
        .click();
      return page.getByRole("alertdialog", { name: "Clear watch history?" });
    };

    let dialog = await openClear();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toBeHidden();
    await expect(historyList(page).getByRole("link", { name: /, played / })).toHaveCount(
      2,
    );

    dialog = await openClear();
    await dialog.getByRole("button", { name: "Clear", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByRole("heading", { name: "No watch history" })).toBeVisible();
    await expect(historyList(page)).toHaveCount(0);
    expect(await historyEntries(request)).toEqual([]);
  });

  // ここから改訂（子 #877）。前のテストで履歴は空になり、A は視聴済み、B は先頭近くまで
  // 見た状態である。

  test("途中の B と最後まで見た A で、「In progress」は B だけ、「Watched」は A だけ、既定は両方を出す（受け入れ条件 10）", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    const cleared = await request.delete("/api/watch-history", {
      headers: { Origin: origin },
    });
    expect(cleared.status()).toBe(204);

    // B を 2 回（2 回目は 5 秒の MinResumeMs より先まで）、その間に A を最後まで見る。
    // 日付の一覧のテストは、最も古い B の 1 回目を前の日へずらす。
    await watchVideo(page, video(titleB), 2);
    await playToEnd(page, request, video(titleA));
    await watchVideo(page, video(titleB), 6);
    const progressB = await savedProgress(request, video(titleB));
    expect(progressB?.completed).toBe(false);
    expect(progressB?.positionMs ?? 0).toBeGreaterThanOrEqual(5000);
    expect((await historyEntries(request)).map((entry) => entry.video?.id)).toEqual([
      video(titleB).id,
      video(titleA).id,
      video(titleB).id,
    ]);

    await openHistory(page);
    await expect(watchFilter(page, "All")).toHaveAttribute("aria-checked", "true");
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(2);

    await watchFilter(page, "In progress").click();
    await expect(page).toHaveURL("/history?watch=inProgress");
    await expect(entryLinks(page, titleB)).toHaveCount(2);
    await expect(entryLinks(page, titleA)).toHaveCount(0);

    await watchFilter(page, "Watched").click();
    await expect(page).toHaveURL("/history?watch=watched");
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(0);

    await watchFilter(page, "All").click();
    await expect(page).toHaveURL("/history");
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(2);

    // ライブラリで視聴済みのカードは、履歴の「Watched」に出る A と同じである。
    expect(await watchedCards(page, 1)).toEqual([titleA]);
  });

  test("狭い画面で / を押すと検索欄が開いてフォーカスが入り、題名の一部で絞り、消すと戻る（受け入れ条件 11）", async ({
    page,
  }) => {
    // lg（1024 px）未満では検索欄は畳まれ、見出しの検索のボタンで開く（ui-design.md「Header row」）。
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/history");
    await expect(entryLinks(page, titleB)).toHaveCount(2);
    await expect(searchField(page)).toBeHidden();
    await expect(page.getByRole("button", { name: "Search titles" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );

    // HistoryPage と SearchBox のどちらも / を受ける。欄が開いてフォーカスが入り、/ は入らない。
    await page.locator("body").press("/");
    await expect(searchField(page)).toBeVisible();
    await expect(searchField(page)).toBeFocused();
    await expect(searchField(page)).toHaveValue("");

    await page.keyboard.type("確認A");
    await expect(page).toHaveURL(`/history?q=${encodeURIComponent("確認A")}`);
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(0);

    // Esc は欄を空にしてフォーカスを外し、空でフォーカスの無い欄は畳まれる
    // （ui-design.md「Header row」）。
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL("/history");
    await expect(searchField(page)).toBeHidden();
    await expect(page.getByRole("button", { name: "Search titles" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(2);

    // 開き直した欄は空のまま。
    await page.getByRole("button", { name: "Search titles" }).click();
    await expect(searchField(page)).toBeFocused();
    await expect(searchField(page)).toHaveValue("");
  });

  test("「In progress」とタイトルの検索を合わせると、両方を満たす件だけが残る（受け入れ条件 12）", async ({
    page,
  }) => {
    await page.goto("/");
    await openHistory(page);
    await watchFilter(page, "In progress").click();
    await expect(entryLinks(page, titleA)).toHaveCount(0);

    // lg からは欄がいつも出ていて、/ でフォーカスが入る。
    await page.locator("body").press("/");
    await expect(searchField(page)).toBeFocused();
    await page.keyboard.type("確認B");
    await expect(page).toHaveURL(
      `/history?watch=inProgress&q=${encodeURIComponent("確認B")}`,
    );
    await expect(entryLinks(page, titleB)).toHaveCount(2);
    await expect(entryLinks(page, titleA)).toHaveCount(0);

    // 題名は合っても途中ではない A は出ず、該当なしの表示になる。
    await searchField(page).fill("確認A");
    await expect(page).toHaveURL(
      `/history?watch=inProgress&q=${encodeURIComponent("確認A")}`,
    );
    await expect(
      page.getByRole("heading", { name: "No history matches these conditions" }),
    ).toBeVisible();
    await expect(historyList(page)).toHaveCount(0);

    await watchFilter(page, "All").click();
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(0);
  });

  test("日付の一覧には件のある日だけが並び、1 つ選ぶとその日からの履歴が出る（受け入れ条件 13）", async ({
    page,
    request,
  }) => {
    const entries = await historyEntries(request);
    const oldest = entries.at(-1);
    expect(oldest?.video?.id).toBe(video(titleB).id);
    if (oldest === undefined) return;
    backdateEntry(oldest.id, Date.parse(oldest.playedAt) - 3 * 24 * 60 * 60 * 1000);
    const moved = await historyEntries(request);

    await page.goto("/");
    await openHistory(page);
    const days = await localDays(
      page,
      moved.map((entry) => entry.playedAt),
    );
    expect(days.length).toBeGreaterThanOrEqual(2);
    const items = jumpItems(page);
    await expect(items).toHaveCount(days.length);
    expect(
      await items.evaluateAll((elements) =>
        elements.map((element) => element.getAttribute("data-value")),
      ),
    ).toEqual(days);

    // 3 日前の日を選ぶと、その日の件から始まり、それより新しい件は出ない。
    const day = days.at(-1) ?? "";
    await items.last().click();
    await expect(page).toHaveURL(`/history?date=${day}`);
    await expect(items.last()).toHaveAttribute("aria-checked", "true");
    await expect(entryLinks(page, titleB)).toHaveCount(1);
    await expect(entryLinks(page, titleA)).toHaveCount(0);

    // 最初の項目は一覧の先頭なので、date を外して全件に戻る。
    await items.first().click();
    await expect(page).toHaveURL("/history");
    await expect(entryLinks(page, titleA)).toHaveCount(1);
    await expect(entryLinks(page, titleB)).toHaveCount(2);
  });

  test("途中の件は位置と長さを出し、別のタブで見進めてから開き直すと表示が進む（受け入れ条件 14）", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000);
    await page.goto("/");
    await openHistory(page);
    const item = video(titleB);
    const rows = entryRows(page, titleB);
    await expect(rows).toHaveCount(2);
    const saved = (await savedProgress(request, item))?.positionMs ?? 0;
    // 同じ動画の行は、どれも今の位置を出す。
    for (const row of await rows.all()) {
      await expect(positionText(row)).toHaveText(
        new RegExp(`^0:${String(Math.floor(saved / 1000)).padStart(2, "0")} / 0:40$`),
      );
      await expect(positionBar(row)).toBeVisible();
    }
    const before = seconds((await positionText(rows.first()).textContent()) ?? "");

    const other = await page.context().newPage();
    await watchVideo(other, item, 3);
    await other.close();
    const after = (await savedProgress(request, item))?.positionMs ?? 0;
    expect(Math.floor(after / 1000)).toBeGreaterThan(before);

    await page.reload();
    await expect(entryRows(page, titleB)).toHaveCount(3);
    for (const row of await entryRows(page, titleB).all()) {
      await expect(positionText(row)).toHaveText(
        new RegExp(`^0:${String(Math.floor(after / 1000)).padStart(2, "0")} / 0:40$`),
      );
    }
  });

  test("「Resume」を押すと動画が開き、出ていた位置から再生が始まる（受け入れ条件 15）", async ({
    page,
    request,
  }) => {
    const item = video(titleB);
    const saved = (await savedProgress(request, item))?.positionMs ?? 0;
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
  }) => {
    const item = video(titleA);
    await page.goto("/");
    await openHistory(page);
    const row = entryRows(page, titleA);
    await expect(row).toHaveCount(1);
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

  test("ゲストのサイドバーに履歴は無く、/history を開くとログイン画面になる（受け入れ条件 9）", async ({
    browser,
  }) => {
    const { context, page } = await guestPage(browser);
    await page.goto("/");
    const sidebar = page.getByRole("complementary", { name: "Main navigation" });
    await expect(sidebar.getByRole("link", { name: "Sign in" })).toBeVisible();
    await expect(sidebar.getByRole("link", { name: "History" })).toHaveCount(0);

    await page.goto("/history");
    await expect(page).toHaveURL("/login?next=%2Fhistory");
    await expect(page.getByRole("heading", { level: 1, name: "Sign in" })).toBeVisible();
    await context.close();
  });
});

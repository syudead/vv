import {
  type APIRequestContext,
  type Browser,
  expect,
  type Locator,
  type Page,
  type Request,
  test,
} from "@playwright/test";

// 視聴履歴の流れ（specs/043-watch-history、親 #792、子 #864）を、実サーバーと実メディアに
// 通す。動画は run-e2e.mjs が generateHistoryFixtures で作る 40 秒の2本で、ほかの e2e の
// 動画とは題名で分ける。ページテストやロジックテストで済む規則（日の見出し、404 の読み直し、
// 削除の失敗など）はここで重ねない（docs/design-docs/web-testing.md の Test levels）。
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
// 履歴の行の日と時刻（日の見出しの 2 行と en の formatTime、例 "Today, Oct 10 at 3:04 PM"）。
// 日付をまたいだ実行でも落ちないよう Today と Yesterday のどちらも受ける。
const playedAt =
  /played (Today|Yesterday), [A-Z][a-z]{2} \d{1,2}(, \d{4})? at \d{1,2}:\d{2}\s?[AP]M$/;

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

/** progressSaves は、そのページが item に送った再生位置の保存の本文を集める。 */
function progressSaves(page: Page, item: Video) {
  const saves: { positionMs: number; playbackId?: string }[] = [];
  page.on("request", (candidate: Request) => {
    if (
      candidate.method() === "PUT" &&
      candidate.url().endsWith(`/api/videos/${String(item.id)}/progress`)
    ) {
      saves.push(candidate.postDataJSON() as { positionMs: number; playbackId?: string });
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
  // 止めたときの保存（ID つき）が届くまで待つ。
  await expect
    .poll(() => saves.filter((save) => save.playbackId).length)
    .toBeGreaterThan(0);
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

  test("数秒再生すると時刻つきでいちばん上に出て、一時停止と再開を 2 回しても 1 件のまま（受け入れ条件 1、3）", async ({
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
    const first = historyList(page).getByRole("link").first();
    await expect(first).toHaveAccessibleName(
      new RegExp(`^${escapeRegExp(titleA)}, .*${playedAt.source}`),
    );
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
    const newest = historyList(page).locator('[data-slot="timeline-item"]').first();
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
    const openClear = async () => {
      await page.getByRole("button", { name: "More" }).click();
      await page.getByRole("menuitem", { name: "Clear history…" }).click();
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

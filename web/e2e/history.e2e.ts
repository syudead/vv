import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

// 視聴履歴のうち、本物の再生でしか確かめられないものを実サーバーと実メディアに通す（docs/design-docs/web-testing.md）。履歴の
// 「Resume」を押すと、そのまま再生できるファイルを Chromium が保存した位置から再生し始める。
// 動画は run-e2e.mjs が generateHistoryFixtures で作る 40 秒の2本で、ほかの e2e の動画とは
// 題名で分ける。履歴の件の作り方・削除・絞り込み・検索・位置の表示・「Start over」・ゲストの
// 扱いは src/history/HistoryPage、src/player/useProgressSaving、src/auth/AuthGate のテストと、
// Go のストアと httpapi のテストが確かめる。

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
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };
// media-fixtures.mjs の HISTORY_VIDEOS と同じ題名。
const titleA = "履歴の確認A";
const titleB = "履歴の確認B";
const videos = new Map<string, Video>();

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

/**
 * saveProgress は item の再生位置を positionMs にする。再生 ID を付けると、その ID の最初の
 * 保存が履歴の件を 1 件足す。
 */
async function saveProgress(
  request: APIRequestContext,
  item: Video,
  positionMs: number,
  playbackId?: string,
) {
  const response = await request.put(`/api/videos/${String(item.id)}/progress`, {
    headers: mutationHeaders,
    data: playbackId === undefined ? { positionMs } : { positionMs, playbackId },
  });
  expect(response.status()).toBe(200);
  return (await response.json()) as Progress;
}

/** historyList は履歴の画面の一覧（日のまとまりの全体）である。 */
function historyList(page: Page): Locator {
  return page.getByRole("list", { name: "Watch history" });
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

test.describe("watch history", () => {
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
            return found?.probeState === "done";
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

  test("「Resume」を押すと動画が開き、出ていた位置から再生が始まる（受け入れ条件 15）", async ({
    page,
    request,
  }) => {
    const item = video(titleB);
    // 再生 ID を付けた保存が履歴の件を作る。先頭へ戻さない 5 秒（MinResumeMs）より先の途中に置く。
    const saved = (await saveProgress(request, item, 8000, crypto.randomUUID()))
      .positionMs;
    expect(saved).toBeGreaterThanOrEqual(5000);

    await page.goto("/library");
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
});

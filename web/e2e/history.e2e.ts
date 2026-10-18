import {
  type APIRequestContext,
  expect,
  type Locator,
  type Page,
  test,
} from "@playwright/test";

// Verify that Resume starts real Chromium playback at the saved position. Lower-level suites cover history behavior; see docs/design-docs/web-testing.md.

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
// Keep titles aligned with HISTORY_VIDEOS in media-fixtures.mjs.
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

/** A playback ID creates a history entry on its first progress save. */
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

function historyList(page: Page): Locator {
  return page.getByRole("list", { name: "Watch history" });
}

/** Leave the player through its close control before using the sidebar. */
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

/** Move over the player to reveal controls hidden by video.js inactivity handling. */
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

function entryRows(page: Page, title: string): Locator {
  return historyList(page)
    .locator('[data-slot="grouped-list-item"]')
    .filter({
      has: page.getByRole("link", { name: new RegExp(`^${escapeRegExp(title)}, `) }),
    });
}

function positionText(row: Locator): Locator {
  return row.getByText(/^\d+:\d{2} \/ \d+:\d{2}$/);
}

function seconds(text: string): number {
  const [minutes = "", rest = ""] = text.split(" / ")[0]?.split(":") ?? [];
  return Number(minutes) * 60 + Number(rest);
}

/** Record playing rather than paused=false: autoplay may clear paused before the metadata-driven resume seek. The listener survives client-side navigation. */
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

/** Return null until the first playing event. */
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

    const cleared = await request.delete("/api/watch-history", {
      headers: { Origin: origin },
    });
    expect(cleared.status()).toBe(204);
  });

  // Unregister fixtures so retries can repeat setup and later suites do not inherit these videos.
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
    // Use a playback ID to create history and a position beyond MinResumeMs (5 seconds).
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

    await expect.poll(() => firstPlayingAt(page), { timeout: 15_000 }).not.toBeNull();
    const startedAt = (await firstPlayingAt(page)) ?? 0;
    expect(startedAt).toBeGreaterThanOrEqual(shown);
    expect(startedAt).toBeLessThan(shown + 2);
    await pause(page);
  });
});

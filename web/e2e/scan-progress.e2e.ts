import {
  type APIRequestContext,
  type Browser,
  expect,
  type Page,
  test,
} from "@playwright/test";

/** generateScanProgressFixtures（media-fixtures.mjs）が作る動画の本数。 */
const SCAN_PROGRESS_VIDEOS = 10;

/**
 * stubServerEvents は変化の知らせの接続を、何も送らない1本に差し替える。
 *
 * この検査は取り込みの状態を route で決めるので、実際のサーバーが送る状態
 * （先に走った検査の取り込み）を混ぜない。retry を長くして、切れたあとに
 * つなぎ直さないようにする。
 */
async function stubServerEvents(page: Page) {
  await page.route("**/api/events", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: "retry: 3600000\n\n",
    }),
  );
}

/**
 * failLogout はログアウトの要求を失敗させる。押すたびに「ログアウトできませんでした」の
 * トーストが出るので、トーストの置き場所を確かめるきっかけに使う。
 */
async function failLogout(page: Page) {
  await page.route("**/api/auth/logout", (route) =>
    route.fulfill({ status: 500, contentType: "application/json", body: "{}" }),
  );
}

const logoutFailed = "Couldn't sign out";

/** scanBody は `GET /api/scans/current` の応答（contracts/scan-api.md §2）を作る。 */
function scanBody(values: Record<string, unknown>) {
  return {
    status: "running",
    issues: { failed: 0, substituted: 0, revision: 0 },
    state: "running",
    ...values,
  };
}

test("scan progress remains one indicator across library, settings, and playback", async ({
  page,
}) => {
  let currentCalls = 0;
  await page.route("**/api/media-folders", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([{ id: 1, path: "/media", version: 1 }]),
    }),
  );
  await stubServerEvents(page);
  await page.route("**/api/scans/current", async (route) => {
    currentCalls += 1;
    const body = scanBody({ id: 17, videos: { total: 100, settled: currentCalls } });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });

  await page.goto("/#scan-progress");
  const indicator = page.getByRole("button", { name: /Open the scan status/ });
  await expect(indicator).toBeVisible();
  await indicator.hover();
  await expect(page.getByRole("dialog")).toContainText(" of 100 videos done");
  await indicator.click();
  await expect(page).toHaveURL(/\/settings#scan-status$/);
  await expect(page.getByRole("heading", { name: "Scan status" })).toBeFocused();

  const settingsIndicator = page.getByRole("button", { name: /Open the scan status/ });
  await expect(settingsIndicator).toHaveText(/Scanning/);
  await page.getByRole("link", { name: "VVMDM home" }).click();
  await expect(page).toHaveURL(/\/$/);
  const routePersistentIndicator = page.getByRole("button", {
    name: /Open the scan status/,
  });
  await expect(routePersistentIndicator).toHaveCount(1);

  await page.evaluate(() => {
    window.history.pushState({}, "", "/videos/1");
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
  await expect(page).toHaveURL(/\/videos\/1$/);
  await expect(routePersistentIndicator).toHaveCount(1);
  const callsAfterPlaybackNavigation = currentCalls;
  // 状態は一定間隔では取り直さない。ウィンドウへ戻ったときの取り直しが、再生画面でも
  // 同じインジケーターへ反映されることを確かめる。
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect
    .poll(async () => {
      const label = await routePersistentIndicator.textContent();
      return Number(label?.match(/(\d+) \/ 100/)?.[1] ?? 0);
    })
    .toBeGreaterThan(callsAfterPlaybackNavigation);
});

test("an idle page recovers from a transient current-scan failure on the next refresh", async ({
  page,
}) => {
  let refreshRequested = false;
  let failedOnce = false;
  await page.route("**/api/media-folders", async (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "[]" }),
  );
  await stubServerEvents(page);
  await page.route("**/api/scans/current", async (route) => {
    if (!refreshRequested) {
      await route.fulfill({ status: 404, body: "{}" });
      return;
    }
    if (!failedOnce) {
      failedOnce = true;
      await route.fulfill({ status: 500, body: "{}" });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(scanBody({ id: 18, status: "finding" })),
    });
  });

  await page.goto("/");
  await expect(page.getByRole("button", { name: /Open the scan status/ })).toHaveCount(0);
  refreshRequested = true;
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  // 一時的な失敗のあとは、一定間隔では取り直さない。次にウィンドウへ戻ったとき
  // （または変化の知らせの接続をつなぎ直したとき）に取り直して回復する。
  await expect.poll(() => failedOnce).toBe(true);
  // failedOnce は 500 を返す前に立つ。その取得（とフォルダの取得）がまだ途中のうちに
  // 戻ると、ScanProvider の loadUnlessLoading は途中の取得に任せて取り直さない。そこで、
  // 表示が出るまで「ウィンドウへ戻る」を繰り返す（一定間隔での取り直しはしないので、
  // 回復は戻ったことによる）。
  const indicator = page.getByRole("button", { name: /^Scanning\./ });
  await expect(async () => {
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(indicator).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 10_000 });
  await indicator.hover();
  const progress = page.getByRole("progressbar", {
    name: "Progress of the videos in this scan",
  });
  await expect(progress).toBeVisible();
  await expect(progress).not.toHaveAttribute("aria-valuenow");
  await expect(page.getByRole("dialog")).toContainText("Looking for files…");
});

test("opening failed scan details acknowledges the notice across reloads", async ({
  page,
}) => {
  let state = "running";
  await page.route("**/api/media-folders", async (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "[]" }),
  );
  await stubServerEvents(page);
  await page.route("**/api/scans/current", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(
        scanBody({
          id: 19,
          status: state,
          state,
          videos: { total: 2, settled: 1 },
          errorCode: state === "failed" ? "internal" : undefined,
        }),
      ),
    }),
  );

  await page.goto("/");
  await expect(
    page.getByRole("button", { name: /^Scanning 1 of 2 videos done\./ }),
  ).toBeVisible();
  state = "failed";
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await page
    .getByRole("button", { name: /^Scan failed .*Open the scan status$/ })
    .click();
  await expect(page).toHaveURL(/\/settings#scan-status$/);
  await expect(page.getByRole("button", { name: /Open the scan status/ })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("button", { name: /Open the scan status/ })).toHaveCount(0);
});

for (const width of [360, 640, 768]) {
  test(`toast stays clear of the scan summary at ${String(width)}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 800 });
    await page.route("**/api/media-folders", async (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: "[]" }),
    );
    await stubServerEvents(page);
    await page.route("**/api/scans/current", async (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(scanBody({ id: 20, videos: { total: 10, settled: 4 } })),
      }),
    );

    await failLogout(page);

    await page.goto("/");
    const indicator = page.getByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
    await expect(indicator).toBeVisible();
    await page.getByRole("button", { name: "Menu" }).click();
    await page.getByRole("button", { name: "Sign out" }).click();
    if (width < 640) {
      // 狭い幅のサイドバーは Sheet のドロワーで、Esc で閉じる。
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }
    const toast = page.getByText(logoutFailed);
    await expect(toast).toBeVisible();
    const sidebar = page.getByRole("complementary", { name: "Main navigation" });
    const expandedSidebarBox = width >= 640 ? await sidebar.boundingBox() : null;
    if (width >= 640) {
      await page.getByRole("button", { name: "Menu" }).click();
      // 畳むとアイコンのレールになる（shadcn/ui の Sidebar の collapsed）。
      await expect(page.locator('[data-slot="sidebar"]')).toHaveAttribute(
        "data-state",
        "collapsed",
      );
    }
    await indicator.hover();
    const summary = page.getByRole("dialog");
    await expect(summary).toBeVisible();
    const [toastBox, summaryBox, indicatorBox, railSidebarBox] = await Promise.all([
      toast.boundingBox(),
      summary.boundingBox(),
      indicator.boundingBox(),
      width >= 640 ? sidebar.boundingBox() : null,
    ]);
    expect(toastBox).not.toBeNull();
    expect(summaryBox).not.toBeNull();
    expect(indicatorBox).not.toBeNull();
    const toastAndSummaryAreSeparate =
      toastBox!.x + toastBox!.width <= summaryBox!.x ||
      summaryBox!.x + summaryBox!.width <= toastBox!.x ||
      toastBox!.y + toastBox!.height <= summaryBox!.y ||
      summaryBox!.y + summaryBox!.height <= toastBox!.y;
    expect(toastAndSummaryAreSeparate).toBe(true);
    expect(toastBox!.y + toastBox!.height).toBeLessThanOrEqual(indicatorBox!.y);
    for (const sidebarBox of [expandedSidebarBox, railSidebarBox]) {
      if (sidebarBox !== null) {
        expect(toastBox!.x).toBeGreaterThanOrEqual(sidebarBox.x + sidebarBox.width);
      }
    }
  });
}

for (const { width, height } of [
  { width: 360, height: 800 },
  { width: 640, height: 500 },
  { width: 768, height: 800 },
  { width: 1280, height: 800 },
]) {
  test(`toast stays clear of playback at ${String(width)}x${String(height)}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height });
    await page.route("**/api/media-folders", async (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: "[]" }),
    );
    await stubServerEvents(page);
    await page.route("**/api/scans/current", async (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(scanBody({ id: 21, videos: { total: 10, settled: 4 } })),
      }),
    );

    await failLogout(page);

    await page.goto("/");
    await page.getByRole("button", { name: "Menu" }).click();
    // ログアウトは失敗するまで押せないので、トーストが出るのを待ってから次を押す。
    const logoutButton = page.getByRole("button", { name: "Sign out" });
    for (let count = 1; count <= 3; count += 1) {
      await logoutButton.click();
      await expect(page.getByText(logoutFailed)).toHaveCount(count);
    }
    if (width < 640) {
      // 狭い幅のサイドバーは Sheet のドロワーで、Esc で閉じる。
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }

    await page.evaluate(() => {
      window.history.pushState({}, "", "/videos/1");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    await expect(page).toHaveURL(/\/videos\/1$/);
    // 再生画面では、溜まったトーストを一度に 1 つだけ出す。
    await expect(page.getByText(logoutFailed)).toHaveCount(1);
    const toast = page.getByText(logoutFailed);
    await expect(toast).toBeVisible();
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));

    const indicator = page.getByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
    await indicator.hover();
    const summary = page.getByRole("dialog");
    await expect(summary).toBeVisible();
    const [toastBox, summaryBox, indicatorBox] = await Promise.all([
      toast.boundingBox(),
      summary.boundingBox(),
      indicator.boundingBox(),
    ]);
    expect(toastBox).not.toBeNull();
    expect(summaryBox).not.toBeNull();
    expect(indicatorBox).not.toBeNull();
    // 通知は上の帯の中（右下の表示と吹き出しより上）に出る。
    expect(toastBox!.y + toastBox!.height).toBeLessThanOrEqual(indicatorBox!.y);
    expect(toastBox!.y + toastBox!.height).toBeLessThanOrEqual(summaryBox!.y);
    // 閉じる × は見出しの帯に 1 つだけあり、見えていることも確かめる。
    const closeButtons = page.getByRole("button", { name: "Close" });
    const closeBoxes = [];
    for (let index = 0; index < (await closeButtons.count()); index += 1) {
      const box = await closeButtons.nth(index).boundingBox();
      if (box !== null && box.width > 0) closeBoxes.push(box);
    }
    expect(closeBoxes).toHaveLength(1);
    const closeBox = closeBoxes[0]!;
    for (const box of [toastBox!, indicatorBox!]) {
      const clearOfClose =
        box.x + box.width <= closeBox.x ||
        closeBox.x + closeBox.width <= box.x ||
        box.y + box.height <= closeBox.y ||
        closeBox.y + closeBox.height <= box.y;
      expect(clearOfClose).toBe(true);
    }
    for (const other of [summaryBox!, indicatorBox!]) {
      const separate =
        toastBox!.x + toastBox!.width <= other.x ||
        other.x + other.width <= toastBox!.x ||
        toastBox!.y + toastBox!.height <= other.y ||
        other.y + other.height <= toastBox!.y;
      expect(separate).toBe(true);
    }

    // 概要が開いているときの Esc は概要に任せ、再生画面は閉じない。
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(/\/videos\/1$/);
  });
}

test("準備が残るあいだは完了と表示せず、再読み込みのあとも同じ状態と進み具合を出す", async ({
  page,
}) => {
  await page.route("**/api/media-folders", async (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "[]" }),
  );
  await stubServerEvents(page);
  // 走査は閉じ（state = done）、準備だけが残っている。
  await page.route("**/api/scans/current", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(
        scanBody({
          id: 22,
          state: "done",
          videos: { total: 10, settled: 6 },
          activity: { kind: "seekThumbnail", fileName: "clip.mp4" },
        }),
      ),
    }),
  );

  const expectSameState = async () => {
    const indicator = page.getByRole("button", {
      name: "Scanning 6 of 10 videos done. Open the scan status",
    });
    await expect(indicator).toBeVisible();
    await expect(indicator).toHaveText("Scanning6 / 10");
    const section = page.locator("#scan-status");
    await expect(section).toContainText("6 of 10 videos done");
    await expect(section).toContainText("Creating seek thumbnails · clip.mp4");
    await expect(section).not.toContainText("Done");
    await expect(section).not.toContainText("Finished");
  };
  await page.goto("/settings#scan-status");
  await expectSameState();
  await page.reload();
  await expectSameState();
});

/** guestContext は Cookie を持たないブラウザを開く。 */
function guestContext(browser: Browser) {
  return browser.newContext({ storageState: { cookies: [], origins: [] } });
}

test("ゲストには右下の表示を出さず、取り込みの状態も問い合わせない", async ({
  browser,
}) => {
  const context = await guestContext(browser);
  const page = await context.newPage();
  const requests: string[] = [];
  page.on("request", (request) => requests.push(new URL(request.url()).pathname));
  // 所有者の取り込みが走っていても、ゲストには見せない。
  await page.route("**/api/scans/current", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(scanBody({ id: 23, videos: { total: 10, settled: 4 } })),
    }),
  );
  await page.goto("/");
  await expect(page.getByRole("link", { name: "Sign in" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Open the scan status/ })).toHaveCount(0);
  expect(requests.filter((path) => /^\/api\/(scans|events)/.test(path))).toEqual([]);
  await context.close();
});

interface MediaFolder {
  id: number;
  version: number;
  path: string;
}

interface ApiScan {
  id: number;
  status: string;
  videos?: { total: number; settled: number };
  settledAt?: string;
}

interface ApiVideo {
  title: string;
  probeState: string;
  thumbnailState: string;
  seekThumbnailState?: string;
  previewState: string;
}

const origin = "http://127.0.0.1:15173";
const mutationHeaders = { Origin: origin, "Content-Type": "application/json" };

async function currentScan(request: APIRequestContext): Promise<ApiScan | null> {
  const response = await request.get("/api/scans/current");
  if (response.status() === 404) return null;
  return (await response.json()) as ApiScan;
}

test.describe.serial("import progress with the real server", () => {
  let folder: MediaFolder | null = null;

  test.afterAll(async ({ request }) => {
    if (folder === null) return;
    const response = await request.get("/api/media-folders");
    const current = ((await response.json()) as MediaFolder[]).find(
      (item) => item.id === folder?.id,
    );
    if (current === undefined) return;
    const removed = await request.delete(
      `/api/media-folders/${String(current.id)}?version=${String(current.version)}`,
      { headers: mutationHeaders },
    );
    expect(removed.status()).toBe(204);
  });

  test("10本の取り込みで、進み具合が単位を変えずに増え、準備の終わりで完了になる", async ({
    page,
    request,
  }) => {
    test.setTimeout(360_000);
    const root = process.env.MDM_E2E_SCAN_PROGRESS_MEDIA_DIR;
    if (root === undefined)
      throw new Error("MDM_E2E_SCAN_PROGRESS_MEDIA_DIR is not configured");

    // 先に走った検査の取り込みが残っていると、その動画が分母に持ち越される。
    // 終わるのを待ってから始める。
    await expect
      .poll(async () => (await currentScan(request))?.status ?? "none", {
        timeout: 300_000,
      })
      .not.toMatch(/^(finding|running)$/);

    const created = await request.post("/api/media-folders", {
      headers: mutationHeaders,
      data: { path: root },
    });
    expect(created.status()).toBe(201);
    folder = (await created.json()) as MediaFolder;

    await page.goto("/settings#scan-status");
    await expect(page.getByRole("button", { name: "Scan library" })).toBeEnabled();
    // 右下の本体の文言を、変わるたびに時刻とともに記録する（SSE の知らせは速いので、
    // 取りこぼさないようページの中で見る）。
    await page.evaluate(() => {
      const log: { at: number; label: string }[] = [];
      (window as unknown as { scanLabels: typeof log }).scanLabels = log;
      const record = () => {
        const button = Array.from(document.querySelectorAll("button")).find((node) =>
          node.getAttribute("aria-label")?.endsWith("Open the scan status"),
        );
        const label = button?.textContent ?? "";
        if (label !== "" && log.at(-1)?.label !== label)
          log.push({ at: Date.now(), label });
      };
      new MutationObserver(record).observe(document.body, {
        subtree: true,
        childList: true,
        characterData: true,
        attributes: true,
      });
    });
    await page.getByRole("button", { name: "Scan library" }).click();

    // 分母には、ほかの登録フォルダで毎回読めないファイル（登録できなかった件）も入る
    // ことがあるので、単位は 10 本以上の一定の値として確かめる。
    const terminal = /^(Done|Some failed)$/;
    const section = page.locator("#scan-status");
    await expect(section.getByText(terminal)).toBeVisible({ timeout: 300_000 });

    const scan = await currentScan(request);
    expect(["done", "partial"]).toContain(scan?.status);
    const total = scan!.videos!.total;
    expect(total).toBeGreaterThanOrEqual(SCAN_PROGRESS_VIDEOS);
    expect(scan?.videos?.settled).toBe(total);
    const fullText = `${String(total)} of ${String(total)} videos done`;
    await expect(section).toContainText(fullText);
    // 完了と示した時点で、この取り込みの 10 本のどれにも準備が残っていない。
    const videos = (
      (await (await request.get("/api/videos?limit=200")).json()) as { items: ApiVideo[] }
    ).items.filter((video) => video.title.startsWith("scan-progress-"));
    expect(videos).toHaveLength(SCAN_PROGRESS_VIDEOS);
    for (const video of videos) {
      expect([
        video.probeState,
        video.thumbnailState,
        video.seekThumbnailState,
        video.previewState,
      ]).not.toContain("pending");
    }

    const log = await page.evaluate(
      () =>
        (window as unknown as { scanLabels: { at: number; label: string }[] }).scanLabels,
    );
    const counted = log
      .map((entry) => ({ ...entry, match: /(\d+) \/ (\d+)/.exec(entry.label) }))
      .filter((entry) => entry.match !== null);
    // 単位は最初から最後まで動画の本数（「M / N」）のまま、済みの本数は減らない。割合や
    // 仕事の件数に切り替わらない。
    expect(counted.length).toBeGreaterThan(0);
    for (const entry of log) {
      expect(entry.label, JSON.stringify(log)).toMatch(
        /^(Starting|Scanning|(Scanning|Done|Some failed)\d+ \/ \d+.*)$/,
      );
    }
    let previous = 0;
    for (const entry of counted) {
      const settled = Number(entry.match![1]);
      expect(settled, JSON.stringify(log)).toBeGreaterThanOrEqual(previous);
      previous = settled;
    }
    expect(Number(counted.at(-1)!.match![2])).toBe(total);
    // 途中の進み具合が見え、完了は満ちたときだけ、完了のあとに Scanning へ戻らない。
    expect(
      counted.some(
        (entry) => entry.label.startsWith("Scanning") && Number(entry.match![1]) < total,
      ),
    ).toBe(true);
    const isTerminal = (label: string) =>
      label.startsWith("Done") || label.startsWith("Some failed");
    const firstTerminal = log.findIndex((entry) => isTerminal(entry.label));
    expect(firstTerminal).toBeGreaterThan(0);
    for (const entry of log.slice(firstTerminal)) {
      expect(entry.label).not.toMatch(/^Scanning/);
    }
    for (const entry of log.filter((item) => item.label.startsWith("Done"))) {
      expect(entry.label).toContain(`${String(total)} / ${String(total)}`);
    }

    // 完了の時刻は準備の終わりを示す: 取り込み中と示していた最後の時点より前ではなく、
    // 画面はその時刻を「Finished 〈日時〉」と書く。
    expect(scan?.settledAt).toBeDefined();
    const settledAt = Date.parse(scan!.settledAt!);
    const lastScanning = log.filter((entry) => entry.label.startsWith("Scanning")).at(-1);
    expect(lastScanning).toBeDefined();
    expect(settledAt).toBeGreaterThanOrEqual(lastScanning!.at - 1_000);
    const finished = await page.evaluate(
      (value) =>
        `Finished ${new Intl.DateTimeFormat("en-US", {
          dateStyle: "medium",
          timeStyle: "short",
        }).format(new Date(value))}`,
      scan!.settledAt!,
    );
    await expect(section.getByTestId("scan-detail")).toHaveText(finished);

    // 再読み込みのあとも、同じ状態と進み具合が出る。
    const statusWord = scan?.status === "done" ? "Done" : "Some failed";
    await page.reload();
    await expect(section.getByText(statusWord, { exact: true })).toBeVisible();
    await expect(section).toContainText(fullText);
    await expect(section.getByTestId("scan-detail")).toHaveText(finished);
  });
});

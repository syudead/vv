import { expect, type Page, test } from "@playwright/test";

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
    const body = {
      id: 17,
      state: "running",
      total: 100,
      completed: currentCalls,
      failed: 0,
    };
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });

  await page.goto("/#scan-progress");
  const indicator = page.getByRole("button", { name: /取り込み状況を開く/ });
  await expect(indicator).toBeVisible();
  await indicator.hover();
  await expect(page.getByRole("dialog")).toContainText("件");
  await indicator.click();
  await expect(page).toHaveURL(/\/settings#scan-status$/);
  await expect(page.getByRole("heading", { name: "取り込み状況" })).toBeFocused();

  const settingsIndicator = page.getByRole("button", { name: /取り込み状況を開く/ });
  await expect(settingsIndicator).toHaveText(/取り込み中/);
  await page.getByRole("link", { name: "vv" }).click();
  await expect(page).toHaveURL(/\/$/);
  const routePersistentIndicator = page.getByRole("button", {
    name: /取り込み状況を開く/,
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
      return Number(label?.match(/(\d+)%/)?.[1] ?? 0);
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
      body: JSON.stringify({
        id: 18,
        state: "running",
        total: 0,
        completed: 0,
        failed: 0,
      }),
    });
  });

  await page.goto("/");
  await expect(page.getByRole("button", { name: /取り込み状況を開く/ })).toHaveCount(0);
  refreshRequested = true;
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  // 一時的な失敗のあとは、一定間隔では取り直さない。次にウィンドウへ戻ったとき
  // （または変化の知らせの接続をつなぎ直したとき）に取り直して回復する。
  await expect.poll(() => failedOnce).toBe(true);
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));

  const indicator = page.getByRole("button", { name: /^取り込み中。/ });
  await expect(indicator).toBeVisible({ timeout: 5000 });
  await indicator.hover();
  const progress = page.getByRole("progressbar", { name: "取り込み対象を確認中" });
  await expect(progress).toBeVisible();
  await expect(progress).not.toHaveAttribute("aria-valuenow");
  await expect(page.getByRole("dialog")).toContainText("0 / 確認中 件");
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
      body: JSON.stringify({
        id: 19,
        state,
        total: 2,
        completed: state === "running" ? 1 : 2,
        failed: state === "failed" ? 1 : 0,
        error: state === "failed" ? "disk" : undefined,
      }),
    }),
  );

  await page.goto("/");
  await expect(page.getByRole("button", { name: /取り込み中 50%/ })).toBeVisible();
  state = "failed";
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await page
    .getByRole("button", { name: /取り込みに失敗しました。取り込み状況を開く/ })
    .click();
  await expect(page).toHaveURL(/\/settings#scan-status$/);
  await expect(page.getByRole("button", { name: /取り込み状況を開く/ })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("button", { name: /取り込み状況を開く/ })).toHaveCount(0);
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
        body: JSON.stringify({
          id: 20,
          state: "running",
          total: 10,
          completed: 4,
          failed: 0,
        }),
      }),
    );

    await page.goto("/");
    const indicator = page.getByRole("button", { name: /取り込み中 40%/ });
    await expect(indicator).toBeVisible();
    await page.getByRole("button", { name: "メニュー" }).click();
    await page.getByRole("button", { name: "最近追加" }).click();
    if (width < 640) {
      await page
        .getByRole("button", { name: "メニューを閉じる" })
        .evaluate((button: HTMLButtonElement) => button.click());
    }
    const toast = page.getByText("「最近追加」は準備中です");
    await expect(toast).toBeVisible();
    const sidebar = page.getByRole("complementary", { name: "メインナビゲーション" });
    const expandedSidebarBox = width >= 640 ? await sidebar.boundingBox() : null;
    if (width >= 640) {
      await page.getByRole("button", { name: "メニュー" }).click();
      await expect(sidebar).toHaveClass(/w-sidebar-rail/);
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
        body: JSON.stringify({
          id: 21,
          state: "running",
          total: 10,
          completed: 4,
          failed: 0,
        }),
      }),
    );

    await page.goto("/");
    await page.getByRole("button", { name: "メニュー" }).click();
    await page.getByRole("button", { name: "最近追加" }).click();
    await page.getByRole("button", { name: "最近追加" }).click();
    await page.getByRole("button", { name: "視聴途中" }).click();
    if (width < 640) {
      await page
        .getByRole("button", { name: "メニューを閉じる" })
        .evaluate((button: HTMLButtonElement) => button.click());
    }
    await expect(page.getByText("「最近追加」は準備中です")).toHaveCount(2);
    await expect(page.getByText("「視聴途中」は準備中です")).toBeVisible();

    await page.evaluate(() => {
      window.history.pushState({}, "", "/videos/1");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    await expect(page).toHaveURL(/\/videos\/1$/);
    await expect(page.getByText("「最近追加」は準備中です")).toHaveCount(1);
    await expect(page.getByText("「視聴途中」は準備中です")).toHaveCount(0);
    const toast = page.getByText("「最近追加」は準備中です");
    await expect(toast).toBeVisible();
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));

    const indicator = page.getByRole("button", { name: /取り込み中 40%/ });
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
    expect(toastBox!.y + toastBox!.height).toBeLessThanOrEqual(52);
    // 閉じる × は見出しの帯に 1 つだけあり、見えていることも確かめる。
    const closeButtons = page.getByRole("button", { name: "閉じる" });
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

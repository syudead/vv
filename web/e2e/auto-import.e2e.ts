import { expect, type Page, test } from "@playwright/test";

/**
 * 自動の取り込み（specs/042-folder-watch-import）の画面を確かめる。
 *
 * 自動の取り込みの `Scan` は、実際のサーバーの監視では作れない（監視は実ファイルの変化で動く）
 * ので、取り込みの状態・問題の一覧・ライブラリの一覧は route で決める。設定の保存だけは
 * 実際のサーバーに通す。
 */

interface StubScan {
  id: number;
  origin: "manual" | "watch";
  status: "running" | "done" | "partial" | "failed";
  failed?: number;
}

function scanBody(scan: StubScan) {
  return {
    id: scan.id,
    origin: scan.origin,
    status: scan.status,
    state: scan.status === "running" ? "running" : "done",
    videos: { total: 2, settled: scan.status === "running" ? 0 : 2 },
    issues: { failed: scan.failed ?? 0, substituted: 0, revision: scan.id },
    settledAt: scan.status === "running" ? undefined : "2026-10-07T10:00:00Z",
  };
}

/**
 * stubScan は取り込みの状態を route で決める。変化の知らせの接続は何も送らない1本に差し替え、
 * 状態はウィンドウのフォーカスで取り直させる（`setScan` のあとに `refocus` を呼ぶ）。
 */
async function stubScan(page: Page, initial: StubScan, issues: string[] = []) {
  let current = initial;
  await page.route("**/api/events", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: "retry: 3600000\n\n",
    }),
  );
  await page.route("**/api/media-folders", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([{ id: 1, path: "/media", version: 1 }]),
    }),
  );
  await page.route("**/api/scans/current", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(scanBody(current)),
    }),
  );
  await page.route("**/api/scans/current/issues?*", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        scanId: current.id,
        items: issues.map((fileName) => ({
          severity: "failed",
          kinds: ["probe_failed"],
          fileName,
          folder: { rootId: 1, path: "", rootName: "media" },
        })),
      }),
    }),
  );
  return {
    async set(next: StubScan) {
      current = next;
      // ウィンドウへ戻ったときと同じに、状態を取り直させる。
      await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    },
  };
}

test("a running watch scan shows nothing; one that ends partial is announced and its issues are listed", async ({
  page,
}) => {
  const stub = await stubScan(page, { id: 1, origin: "manual", status: "done" }, [
    "broken-a.mp4",
    "broken-b.mp4",
  ]);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

  const indicator = page.getByRole("button", { name: /Open the scan status/ });

  await stub.set({ id: 2, origin: "watch", status: "running" });
  // 走っているあいだは右下に何も出さない（進み具合の表示にも、状態の言葉にも）。
  await page.waitForTimeout(500);
  await expect(indicator).toHaveCount(0);
  await expect(page.getByRole("progressbar")).toHaveCount(0);

  await stub.set({ id: 2, origin: "watch", status: "partial", failed: 2 });
  await expect(indicator).toBeVisible();
  await expect(indicator).toContainText("Auto-import: some failed");
  await expect(
    page.getByRole("status").filter({ hasText: "Auto-import finished" }),
  ).toHaveText("Auto-import finished with some failures. 2 videos may not be usable.");

  await indicator.click();
  await expect(page).toHaveURL(/\/settings#scan-status$/);
  const status = page.locator("#scan-status");
  await expect(status.getByText("Auto-import: some failed")).toBeVisible();
  await expect(status.getByText("broken-a.mp4")).toBeVisible();
  await expect(status.getByText("broken-b.mp4")).toBeVisible();
  // 自動の取り込みは Scan library を止めず、メディアフォルダも固めない。
  await expect(status.getByRole("button", { name: "Scan library" })).toBeEnabled();
});

test("a watch scan that ends done announces nothing", async ({ page }) => {
  const stub = await stubScan(page, { id: 1, origin: "manual", status: "done" });
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

  await stub.set({ id: 2, origin: "watch", status: "running" });
  await stub.set({ id: 2, origin: "watch", status: "done" });
  await page.waitForTimeout(800);
  await expect(page.getByRole("button", { name: /Open the scan status/ })).toHaveCount(0);
});

test("the auto-import switch saves the choice and keeps it after a reload", async ({
  page,
}) => {
  const name = "Pick up changes as they happen";
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Auto-import" })).toBeVisible();
  const toggle = page.getByRole("switch", { name });
  await expect(toggle).toBeVisible();
  const before = await toggle.getAttribute("aria-checked");
  const next = before === "true" ? "false" : "true";

  try {
    const saved = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/settings/auto-import") &&
        response.request().method() === "PUT",
    );
    await toggle.click();
    expect((await saved).status()).toBe(200);
    await expect(toggle).toHaveAttribute("aria-checked", next);

    // 押しても、バーも通知も出ない。
    await expect(page.getByRole("progressbar")).toHaveCount(0);
    await expect(page.getByRole("button", { name: /Open the scan status/ })).toHaveCount(
      0,
    );

    await page.reload();
    await expect(page.getByRole("switch", { name })).toHaveAttribute(
      "aria-checked",
      next,
    );
  } finally {
    // 後の検査に持ち越さない。
    const restore = await page.request.put("/api/settings/auto-import", {
      headers: {
        Origin: "http://127.0.0.1:15173",
        "Content-Type": "application/json",
      },
      data: { enabled: before === "true" },
    });
    expect(restore.status()).toBe(200);
  }
});

/** libraryVideo はライブラリの一覧の動画の項目である。 */
function libraryVideo(id: number) {
  return {
    kind: "video",
    video: {
      id,
      title: `video ${String(id)}`,
      public: false,
      sizeBytes: 1024,
      addedAt: "2026-10-07T00:00:00Z",
      updatedAt: "2026-10-07T00:00:00Z",
      fileCreatedAt: "2026-10-07T00:00:00Z",
      playable: true,
      probeState: "done",
      thumbnailState: "pending",
      previewState: "pending",
      tags: [],
    },
  };
}

test("a finished watch scan adds new videos to the open library without losing scroll or selection", async ({
  page,
}) => {
  const stub = await stubScan(page, { id: 1, origin: "manual", status: "done" });
  let ids = Array.from({ length: 60 }, (_, index) => 1000 - index);
  let libraryCalls = 0;
  await page.route("**/api/library?*", (route) => {
    libraryCalls += 1;
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: ids.map(libraryVideo), total: ids.length }),
    });
  });
  await page.route("**/api/videos/*/thumbnail*", (route) =>
    route.fulfill({ status: 404, body: "" }),
  );

  await page.goto("/");
  const target = page.locator('[data-video-id="960"]');
  await target.scrollIntoViewIfNeeded();
  await page.getByRole("checkbox", { name: 'Select "video 960"' }).click({ force: true });
  await expect(page.getByRole("checkbox", { name: 'Select "video 960"' })).toBeChecked();
  await page.evaluate(() => window.scrollTo(0, 600));
  const scrolled = await page.evaluate(() => window.scrollY);
  expect(scrolled).toBeGreaterThan(0);
  const callsBefore = libraryCalls;
  // 今のカードの要素に印を付け、一覧が一度でも空になったかを見張る。消して読み直すと、
  // 要素は作り直されて印が消え、空になる瞬間が記録される。
  await page.evaluate(() => {
    document.querySelector('[data-video-id="960"]')?.setAttribute("data-kept", "yes");
    const win = window as unknown as { __minCards: number };
    win.__minCards = document.querySelectorAll("[data-video-id]").length;
    new MutationObserver(() => {
      win.__minCards = Math.min(
        win.__minCards,
        document.querySelectorAll("[data-video-id]").length,
      );
    }).observe(document.body, { childList: true, subtree: true });
  });

  ids = [1001, ...ids];
  await stub.set({ id: 2, origin: "watch", status: "done" });

  // 先頭に新しい動画が入る。一覧は消えず、読み込み中の表示も出ない。
  await expect(page.locator('[data-video-id="1001"]')).toBeAttached();
  expect(libraryCalls).toBeGreaterThan(callsBefore);
  await expect(page.getByRole("status", { name: /Loading/ })).toHaveCount(0);
  await expect(page.locator('[data-video-id="960"][data-kept="yes"]')).toHaveCount(1);
  expect(
    await page.evaluate(() => (window as unknown as { __minCards: number }).__minCards),
  ).toBeGreaterThanOrEqual(60);
  // 先頭へ戻らず、選択も残る。
  const after = await page.evaluate(() => window.scrollY);
  expect(Math.abs(after - scrolled)).toBeLessThan(400);
  await expect(page.getByRole("checkbox", { name: 'Select "video 960"' })).toBeChecked();
});

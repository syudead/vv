import { expect, test } from "@playwright/test";

/**
 * 自動の取り込み（specs/042-folder-watch-import）の設定の保存を、実際のサーバーに通す。
 *
 * 自動の取り込みの `Scan` は実際のサーバーの監視では作れない（監視は実ファイルの変化で動く）
 * ので、取り込みの状態・問題の一覧・開いている一覧の取り直しは Vitest が確かめる
 * （ScanProgressIndicator.test.tsx・ScanStatusSection.test.tsx・LibraryPage.watchScan.test.tsx）。
 */

test("the auto-import switch saves the choice and keeps it after a reload", async ({
  page,
}) => {
  const name = "Pick up changes as they happen";
  // 切り替えは「Library」のページにある（specs/058-settings-pages/ui-design.md「Library page」）。
  await page.goto("/settings/library");
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

import { expect, test } from "@playwright/test";

test("scan progress remains available in the shell and settings", async ({ page }) => {
  let currentCalls = 0;
  await page.route("**/api/media-folders", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([{ id: 1, path: "/media", version: 1 }]),
    }),
  );
  await page.route("**/api/scans/current", async (route) => {
    currentCalls += 1;
    const body = {
      id: 17,
      state: "running",
      total: 10,
      completed: Math.min(4, currentCalls),
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
  await expect(page.getByRole("dialog").getByRole("status")).toContainText("件");
  await indicator.click();
  await expect(page).toHaveURL(/\/settings#scan-status$/);
  await expect(page.getByRole("heading", { name: "取り込み状況" })).toBeFocused();
});

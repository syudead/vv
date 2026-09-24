import { expect, test } from "@playwright/test";

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
  await expect(page.getByRole("dialog").getByRole("status")).toContainText("件");
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
  await expect
    .poll(async () => {
      const label = await routePersistentIndicator.textContent();
      return Number(label?.match(/(\d+)%/)?.[1] ?? 0);
    })
    .toBeGreaterThan(callsAfterPlaybackNavigation);
});

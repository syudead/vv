import { expect, type Page, test } from "@playwright/test";

/** Silence real SSE updates so prior scans cannot interfere with routed scan state; a long retry interval prevents reconnects. */
async function stubServerEvents(page: Page) {
  await page.route("**/api/events", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: "retry: 3600000\n\n",
    }),
  );
}

/** Fail logout to produce a repeatable toast for layout checks. */
async function failLogout(page: Page) {
  await page.route("**/api/auth/logout", (route) =>
    route.fulfill({ status: 500, contentType: "application/json", body: "{}" }),
  );
}

const logoutFailed = "Couldn't sign out";

function scanBody(values: Record<string, unknown>) {
  return {
    origin: "manual",
    status: "running",
    issues: { failed: 0, substituted: 0, revision: 0 },
    state: "running",
    ...values,
  };
}

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

    await page.goto("/library");
    const indicator = page.getByRole("button", {
      name: /^Scanning 4 of 10 videos done\./,
    });
    await expect(indicator).toBeVisible();
    // Below 640 px the sidebar is a Sheet drawer; wider screens use a rail.
    if (width < 640) await page.getByRole("button", { name: "Menu" }).click();
    await page.getByRole("button", { name: "Sign out" }).click();
    if (width < 640) {
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }
    const toast = page.getByText(logoutFailed);
    await expect(toast).toBeVisible();
    const sidebar = page.getByRole("complementary", { name: "Main navigation" });
    if (width >= 640) {
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
    if (railSidebarBox !== null) {
      expect(toastBox!.x).toBeGreaterThanOrEqual(railSidebarBox.x + railSidebarBox.width);
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

    await page.goto("/library");
    if (width < 640) await page.getByRole("button", { name: "Menu" }).click();
    // Wait for the failure toast before clicking logout again because the pending request disables it.
    const logoutButton = page.getByRole("button", { name: "Sign out" });
    for (let count = 1; count <= 3; count += 1) {
      await logoutButton.click();
      await expect(page.getByText(logoutFailed)).toHaveCount(count);
    }
    if (width < 640) {
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }

    await page.evaluate(() => {
      window.history.pushState({}, "", "/videos/1");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    await expect(page).toHaveURL(/\/videos\/1$/);
    // Wait until the player shows only one queued toast before measuring its position.
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
    // Notifications belong in the top band above bottom-right controls and popovers.
    expect(toastBox!.y + toastBox!.height).toBeLessThanOrEqual(indicatorBox!.y);
    expect(toastBox!.y + toastBox!.height).toBeLessThanOrEqual(summaryBox!.y);

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
  });
}

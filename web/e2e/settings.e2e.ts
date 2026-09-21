import { mkdir, rm } from "node:fs/promises";
import path from "node:path";

import { expect, test } from "@playwright/test";

test("設定画面からVite proxy越しにメディアフォルダを追加できる", async ({ page }) => {
  const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
  if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
  await mkdir(mediaDir, { recursive: true });

  try {
    await page.route("**/api/directories*", async (route) => {
      const requested = new URL(route.request().url()).searchParams.get("path");
      const body =
        requested === mediaDir
          ? { currentPath: mediaDir, parentPath: path.dirname(mediaDir), directories: [] }
          : { directories: [{ name: path.basename(mediaDir), path: mediaDir }] };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    });

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "設定" })).toBeVisible();
    await expect(page.getByText("メディアフォルダが設定されていません")).toBeVisible();

    await page.getByRole("button", { name: "フォルダを追加" }).click();
    const dialog = page.getByRole("dialog", { name: "メディアフォルダを追加" });
    await dialog.getByRole("button", { name: path.basename(mediaDir) }).click();

    const mutation = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/media-folders") &&
        response.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "このフォルダを追加" }).click();
    expect((await mutation).status()).toBe(201);

    await expect(page.getByText(mediaDir)).toBeVisible();
    await expect(page.getByText("same-originの操作だけを受け付けます")).toHaveCount(0);
  } finally {
    await rm(mediaDir, { recursive: true, force: true });
  }
});

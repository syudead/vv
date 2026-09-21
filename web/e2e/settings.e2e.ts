import { mkdir, rm } from "node:fs/promises";
import path from "node:path";

import { expect, test } from "@playwright/test";

test("filesystem rootをメディアフォルダとして登録できる", async ({ page }) => {
  const mediaDir = process.env.MDM_E2E_MEDIA_DIR;
  if (mediaDir === undefined) throw new Error("MDM_E2E_MEDIA_DIR is not configured");
  const filesystemRoot = path.parse(mediaDir).root;

  await page.route("**/api/directories*", async (route) => {
    const requested = new URL(route.request().url()).searchParams.get("path");
    const body =
      requested === filesystemRoot
        ? { currentPath: filesystemRoot, parentPath: null, directories: [] }
        : {
            parentPath: null,
            directories: [{ name: filesystemRoot, path: filesystemRoot }],
          };
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });

  await page.goto("/settings");
  await page.getByRole("button", { name: "フォルダを追加" }).click();
  const picker = page.getByRole("dialog", { name: "メディアフォルダを追加" });
  await picker.getByRole("button", { name: filesystemRoot, exact: true }).click();
  const add = picker.getByRole("button", { name: "このフォルダを追加" });
  await expect(add).toBeEnabled();

  const create = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/media-folders") &&
      response.request().method() === "POST",
  );
  await add.click();
  expect((await create).status()).toBe(201);
  await expect(page.locator("code", { hasText: filesystemRoot })).toBeVisible();

  await page.getByRole("button", { name: "フォルダを削除" }).click();
  const confirmation = page.getByRole("dialog", { name: "フォルダの削除を確認" });
  const remove = page.waitForResponse(
    (response) =>
      response.url().includes("/api/media-folders/") &&
      response.request().method() === "DELETE",
  );
  await confirmation.getByRole("button", { name: "削除する" }).click();
  expect((await remove).status()).toBe(204);
  await expect(page.locator("code", { hasText: filesystemRoot })).toHaveCount(0);
});

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

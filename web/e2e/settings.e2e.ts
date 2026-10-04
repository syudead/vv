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
  await page.getByRole("button", { name: "Add folder" }).click();
  const picker = page.getByRole("dialog", { name: "Add media folder" });
  await picker.getByRole("button", { name: filesystemRoot, exact: true }).click();
  const add = picker.getByRole("button", { name: "Add this folder" });
  await expect(add).toBeEnabled();

  const create = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/media-folders") &&
      response.request().method() === "POST",
  );
  await add.click();
  expect((await create).status()).toBe(201);
  await expect(page.locator("code", { hasText: filesystemRoot })).toBeVisible();

  await page.getByRole("button", { name: "Remove folder" }).click();
  const confirmation = page.getByRole("alertdialog", { name: "Remove this folder?" });
  const remove = page.waitForResponse(
    (response) =>
      response.url().includes("/api/media-folders/") &&
      response.request().method() === "DELETE",
  );
  await confirmation.getByRole("button", { name: "Remove" }).click();
  expect((await remove).status()).toBe(204);
  await expect(page.locator("code", { hasText: filesystemRoot })).toHaveCount(0);
});

test("設定画面からVite proxy越しにメディアフォルダを追加できる", async ({ page }) => {
  const mediaDir = process.env.MDM_E2E_SETTINGS_MEDIA_DIR;
  if (mediaDir === undefined) {
    throw new Error("MDM_E2E_SETTINGS_MEDIA_DIR is not configured");
  }
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
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.getByText("No media folders yet")).toBeVisible();

    await page.getByRole("button", { name: "Add folder" }).click();
    const dialog = page.getByRole("dialog", { name: "Add media folder" });
    await dialog.getByRole("button", { name: path.basename(mediaDir) }).click();

    const mutation = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/media-folders") &&
        response.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "Add this folder" }).click();
    expect((await mutation).status()).toBe(201);

    await expect(page.getByText(mediaDir)).toBeVisible();
    await expect(page.getByText("This change must be made from vv itself.")).toHaveCount(
      0,
    );

    await page.getByRole("button", { name: "Remove folder" }).click();
    const confirmation = page.getByRole("alertdialog", { name: "Remove this folder?" });
    const remove = page.waitForResponse(
      (response) =>
        response.url().includes("/api/media-folders/") &&
        response.request().method() === "DELETE",
    );
    await confirmation.getByRole("button", { name: "Remove" }).click();
    expect((await remove).status()).toBe(204);
  } finally {
    await rm(mediaDir, { recursive: true, force: true });
  }
});

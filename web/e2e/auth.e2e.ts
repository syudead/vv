import { expect, test } from "@playwright/test";

import { ownerAccount } from "./owner-account";

// Verify browser cookie login through the Vite proxy with an independent session. See docs/design-docs/web-testing.md for lower-level auth coverage.
test.use({ storageState: { cookies: [], origins: [] } });

test("ゲストで設定画面を開くとログイン画面になり、ログインすると設定画面に戻る", async ({
  page,
}) => {
  await page.goto("/settings");
  await expect(page).toHaveURL("/login?next=%2Fsettings");

  await page.getByLabel("Username").fill(ownerAccount.username);
  await page.getByLabel("Password").fill(ownerAccount.password);
  await page.keyboard.press("Enter");

  await expect(page).toHaveURL("/settings");
  // The settings Scan page is visible from the sm breakpoint.
  await expect(page.getByRole("heading", { level: 1, name: "Scan" })).toBeVisible();
});

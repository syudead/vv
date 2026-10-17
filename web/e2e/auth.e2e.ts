import { expect, test } from "@playwright/test";

import { ownerAccount } from "./owner-account";

// 本物のブラウザが Vite の proxy 越しにログインの Cookie（HttpOnly、SameSite=Strict）を受けて
// 持ち、その Cookie で所有者の画面を開けること。入力欄の属性・警告・フォーカス・送信中の扱い・ログアウトは
// src/auth のテストと Go の httpapi のテストが確かめる（docs/design-docs/web-testing.md）。
//
// Cookie の無いブラウザで始め、自分でログインする。既存の e2e が使う所有者のセッション
// （auth.setup.ts）には触れない。
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
  // `/settings` は sm から「Scan」のページを出す。
  await expect(page.getByRole("heading", { level: 1, name: "Scan" })).toBeVisible();
});

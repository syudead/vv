import { expect, test } from "@playwright/test";

import { ownerAccount } from "./owner-account";

// アカウントを作る前のサーバーでは、どの URL も初回設定を求める
// （specs/016-single-account-auth/contracts/auth-api.md §1・§4・§5）。このプロジェクトは
// auth.setup.ts より先に、まだ誰もアカウントを作っていないサーバーで走る。
test("未設定のサーバーはどの要求にも初回設定を求める", async ({ request }) => {
  const session = await request.get("/api/auth/session");
  expect(session.status()).toBe(200);
  expect(await session.json()).toEqual({ state: "setupRequired" });

  for (const target of [
    "/api/videos",
    "/api/videos/1",
    "/api/videos/1/thumbnail",
    "/api/folders",
    "/api/media-folders",
    "/api/scans/current",
    "/api/events",
    "/api/tags",
    "/api/nope",
  ]) {
    const response = await request.get(target);
    expect(response.status(), target).toBe(401);
    expect(response.headers()["x-vv-audience"], target).toBe("guest");
    expect(await response.json(), target).toEqual({
      code: "unauthenticated",
      message: "Sign-in required.",
    });
  }

  const nextSession = await request.get("/api/auth/session?next=%2Fsettings");
  expect(await nextSession.json()).toEqual({ state: "setupRequired" });

  // 稼働確認は未設定でも同じ形で返る。
  const health = await request.get("/api/health");
  expect(health.status()).toBe(200);
  expect(((await health.json()) as { status: string }).status).toBe("ok");
});

// 画面でも、未設定のサーバーではどの URL も初回設定画面になり、キーボードだけで
// 設定まで進める。ここで作ったアカウントを、auth.setup.ts はログインで使う。
test("未設定のサーバーはどの URL も初回設定画面にし、設定するとログイン済みの一覧になる", async ({
  page,
}) => {
  for (const target of ["/settings", "/login?next=%2Ftags", "/folders"]) {
    await page.goto(target);
    await expect(page).toHaveURL("/setup");
    await expect(
      page.getByRole("heading", { level: 1, name: "Create an account" }),
    ).toBeVisible();
  }

  await page.goto("/videos/1");
  await expect(page).toHaveURL("/setup");
  const username = page.getByLabel("Username");
  const password = page.getByLabel("Password", { exact: true });
  const confirm = page.getByLabel("Confirm password");
  await expect(username).toBeFocused();
  await expect(username).toHaveAttribute("autocomplete", "username");
  await expect(password).toHaveAttribute("autocomplete", "new-password");
  await expect(confirm).toHaveAttribute("autocomplete", "new-password");
  // e2e は HTTP で配るので、主操作の下に警告が出る。
  await expect(page.getByText(/This connection isn't encrypted/)).toBeVisible();
  await expect(username).toHaveAttribute("aria-describedby", "connection-warning");

  // キーボードだけで入力する。確認を違えると送らず、確認の欄へ戻る。
  const requests: string[] = [];
  page.on("request", (request) => {
    if (request.url().endsWith("/api/auth/setup")) requests.push(request.method());
  });
  await page.keyboard.type(ownerAccount.username);
  await page.keyboard.press("Tab");
  await page.keyboard.type(ownerAccount.password);
  await page.keyboard.press("Tab");
  await page.keyboard.type(`${ownerAccount.password}-typo`);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("alert")).toHaveText("The passwords don't match.");
  await expect(confirm).toBeFocused();
  await expect(confirm).toHaveValue("");
  expect(requests).toEqual([]);

  await page.keyboard.type(ownerAccount.password);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL("/");
  await expect(
    page.getByRole("heading", { level: 1, name: "ライブラリ" }),
  ).toBeAttached();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  expect(requests).toEqual(["POST"]);

  // 設定済みのサーバーでは、初回設定画面を二度と出さない。
  await page.goto("/setup");
  await expect(page).toHaveURL("/");
});

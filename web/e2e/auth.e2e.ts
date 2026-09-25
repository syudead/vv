import { expect, type Page, test } from "@playwright/test";

import { ownerAccount } from "./owner-account";

// 初回設定の後のログイン画面・ログアウト・ゲートの振り分け
// （specs/016-single-account-auth/ui-design.md「Gate」「Credential screens」「Sidebar」）。
// 未設定のサーバーでの初回設定画面は unconfigured.setup.ts が確かめる。
//
// どのテストも Cookie の無いブラウザで始め、自分でログインする。既存の e2e が使う
// 所有者のセッション（auth.setup.ts）をログアウトで終わらせないためである。
// ログインの失敗は同じ送信元で 5 分に 5 回までなので、ここでの失敗は 1 回に収める
// （owner-account.ts）。
test.use({ storageState: { cookies: [], origins: [] } });

async function expectCredentialFields(page: Page) {
  await expect(page.getByLabel("ユーザー名")).toHaveAttribute("autocomplete", "username");
  await expect(page.getByLabel("パスワード")).toHaveAttribute(
    "autocomplete",
    "current-password",
  );
}

test("ゲストで設定画面を開くとログイン画面になり、キーボードだけでログインすると設定画面に戻る", async ({
  page,
}) => {
  await page.goto("/settings");
  await expect(page).toHaveURL("/login?next=%2Fsettings");
  await expect(page.getByRole("heading", { level: 1, name: "ログイン" })).toBeVisible();
  await expectCredentialFields(page);
  // e2e は HTTP で配るので、主操作の下に警告が出て、ユーザー名と主操作から指される。
  await expect(page.getByText(/この接続は暗号化されていません/)).toBeVisible();
  await expect(page.getByLabel("ユーザー名")).toHaveAttribute(
    "aria-describedby",
    "connection-warning",
  );
  await expect(page.getByRole("button", { name: "ログイン" })).toHaveAttribute(
    "aria-describedby",
    "connection-warning",
  );

  await expect(page.getByLabel("ユーザー名")).toBeFocused();
  await page.keyboard.type(ownerAccount.username);
  await page.keyboard.press("Tab");
  await expect(page.getByLabel("パスワード")).toBeFocused();
  await page.keyboard.type(ownerAccount.password);
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "ログイン" })).toBeFocused();
  await page.keyboard.press("Enter");

  await expect(page).toHaveURL("/settings");
  await expect(page.getByRole("heading", { level: 1, name: "設定" })).toBeVisible();
});

test("ログインの送信中は再送信できず、失敗するとパスワードだけが空になる", async ({
  page,
}) => {
  await page.goto("/login");
  await page.getByLabel("ユーザー名").fill(ownerAccount.username);
  await page.getByLabel("パスワード").fill("wrong-password");

  // 応答を止めて、送信中の状態を確かめる。
  let release: () => void = () => undefined;
  const held = new Promise<void>((resolve) => (release = resolve));
  let attempts = 0;
  await page.route("**/api/auth/login", async (route) => {
    attempts += 1;
    await held;
    await route.continue();
  });

  await page.getByLabel("パスワード").press("Enter");
  const submit = page.getByRole("button", { name: "ログイン" });
  await expect(submit).toBeDisabled();
  await page.getByLabel("パスワード").press("Enter");
  await submit.click({ force: true });
  expect(attempts).toBe(1);

  release();
  await expect(page.getByRole("alert")).toHaveText(
    "ユーザー名またはパスワードが違います",
  );
  await expect(page.getByLabel("ユーザー名")).toHaveValue(ownerAccount.username);
  await expect(page.getByLabel("パスワード")).toHaveValue("");
  await expect(page.getByLabel("パスワード")).toBeFocused();
  expect(attempts).toBe(1);

  // 正しいパスワードで入れる。成功で、この送信元の失敗の記録は消える。
  await page.unroute("**/api/auth/login");
  await page.getByLabel("パスワード").fill(ownerAccount.password);
  await page.getByLabel("パスワード").press("Enter");
  await expect(page).toHaveURL("/");
});

test("ログアウトするとゲストの画面になり、前の Cookie を付け直しても所有者に戻らない", async ({
  page,
  context,
}) => {
  await page.goto("/login");
  await page.getByLabel("ユーザー名").fill(ownerAccount.username);
  await page.getByLabel("パスワード").fill(ownerAccount.password);
  await page.getByRole("button", { name: "ログイン" }).click();
  await expect(page).toHaveURL("/");
  const logout = page.getByRole("button", { name: "ログアウト" });
  await expect(logout).toBeVisible();

  const cookies = await context.cookies();
  const session = cookies.find((cookie) => cookie.name === "vv_session");
  expect(session).toBeDefined();
  expect(session?.httpOnly).toBe(true);

  // パスワードとセッション ID をブラウザの保存領域に置かない。
  const stored = await page.evaluate(() =>
    JSON.stringify([{ ...window.localStorage }, { ...window.sessionStorage }]),
  );
  expect(stored).not.toContain(ownerAccount.password);
  expect(stored).not.toContain(session?.value ?? "");

  await page.goto("/folders?query=x");
  await page.getByRole("button", { name: "ログアウト" }).click();

  // 同じ URL がゲストの画面として読み直される。
  await expect(page).toHaveURL("/folders?query=x");
  const login = page
    .getByRole("navigation", { name: "アカウントと設定" })
    .getByRole("link", { name: "ログイン" });
  await expect(login).toBeVisible();
  await expect(login).toHaveAttribute("href", "/login?next=%2Ffolders%3Fquery%3Dx");
  await expect(page.getByRole("button", { name: "ログアウト" })).toHaveCount(0);
  expect(await context.cookies()).toEqual([]);

  // ログアウト前の Cookie を付け直しても、所有者として扱われない。
  await context.addCookies([session!]);
  const state = await page.request.get("/api/auth/session");
  expect(await state.json()).toEqual({ state: "guest" });
  await page.goto("/settings");
  await expect(page).toHaveURL("/login?next=%2Fsettings");
});

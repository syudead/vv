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
  await expect(page.getByLabel("Username")).toHaveAttribute("autocomplete", "username");
  await expect(page.getByLabel("Password")).toHaveAttribute(
    "autocomplete",
    "current-password",
  );
}

test("ゲストで設定画面を開くとログイン画面になり、キーボードだけでログインすると設定画面に戻る", async ({
  page,
}) => {
  await page.goto("/settings");
  await expect(page).toHaveURL("/login?next=%2Fsettings");
  await expect(page.getByRole("heading", { level: 1, name: "Sign in" })).toBeVisible();
  await expectCredentialFields(page);
  // e2e は HTTP で配るので、主操作の下に警告が出て、ユーザー名と主操作から指される。
  await expect(page.getByText(/This connection isn't encrypted/)).toBeVisible();
  await expect(page.getByLabel("Username")).toHaveAttribute(
    "aria-describedby",
    "connection-warning",
  );
  await expect(page.getByRole("button", { name: "Sign in" })).toHaveAttribute(
    "aria-describedby",
    "connection-warning",
  );
  for (const width of [360, 768, 1280]) {
    await page.setViewportSize({ width, height: 800 });
    await expect(page.getByRole("img", { name: "VVMDM" })).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(width);
  }

  await expect(page.getByLabel("Username")).toBeFocused();
  await page.keyboard.type(ownerAccount.username);
  await page.keyboard.press("Tab");
  await expect(page.getByLabel("Password")).toBeFocused();
  await page.keyboard.type(ownerAccount.password);
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Sign in" })).toBeFocused();
  await page.keyboard.press("Enter");

  await expect(page).toHaveURL("/settings");
  await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
});

test("ログインの送信中は再送信できず、失敗するとパスワードだけが空になる", async ({
  page,
}) => {
  await page.goto("/login");
  await page.getByLabel("Username").fill(ownerAccount.username);
  await page.getByLabel("Password").fill("wrong-password");

  // 応答を止めて、送信中の状態を確かめる。
  let release: () => void = () => undefined;
  const held = new Promise<void>((resolve) => (release = resolve));
  let attempts = 0;
  await page.route("**/api/auth/login", async (route) => {
    attempts += 1;
    await held;
    await route.continue();
  });

  await page.getByLabel("Password").press("Enter");
  // 送信中はボタンの名前が「Signing in…」に変わるので、どちらの名前でも同じボタンを指す。
  const submit = page.getByRole("button", { name: /^Sign(ing)? in/ });
  await expect(submit).toBeDisabled();
  await expect(submit).toHaveText("Signing in…");
  await expect(submit).toHaveAttribute("aria-busy", "true");
  await page.getByLabel("Password").press("Enter");
  await submit.click({ force: true });
  expect(attempts).toBe(1);

  release();
  await expect(page.getByRole("alert")).toHaveText(
    "The username or password is incorrect.",
  );
  await expect(page.getByLabel("Username")).toHaveValue(ownerAccount.username);
  await expect(page.getByLabel("Password")).toHaveValue("");
  await expect(page.getByLabel("Password")).toBeFocused();
  expect(attempts).toBe(1);

  // 正しいパスワードで入れる。成功で、この送信元の失敗の記録は消える。
  await page.unroute("**/api/auth/login");
  await page.getByLabel("Password").fill(ownerAccount.password);
  await page.getByLabel("Password").press("Enter");
  await expect(page).toHaveURL("/");
});

test("ログアウトするとゲストの画面になり、前の Cookie を付け直しても所有者に戻らない", async ({
  page,
  context,
}) => {
  await page.goto("/login");
  await page.getByLabel("Username").fill(ownerAccount.username);
  await page.getByLabel("Password").fill(ownerAccount.password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL("/");
  const logout = page.getByRole("button", { name: "Sign out" });
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
  await page.getByRole("button", { name: "Sign out" }).click();

  // 同じ URL がゲストの画面として読み直される。
  await expect(page).toHaveURL("/folders?query=x");
  const login = page
    .getByRole("navigation", { name: "Account and settings" })
    .getByRole("link", { name: "Sign in" });
  await expect(login).toBeVisible();
  await expect(login).toHaveAttribute("href", "/login?next=%2Ffolders%3Fquery%3Dx");
  await expect(page.getByRole("button", { name: "Sign out" })).toHaveCount(0);
  expect(await context.cookies()).toEqual([]);

  // ログアウト前の Cookie を付け直しても、所有者として扱われない。
  await context.addCookies([session!]);
  const state = await page.request.get("/api/auth/session");
  expect(await state.json()).toEqual({ state: "guest" });
  await page.goto("/settings");
  await expect(page).toHaveURL("/login?next=%2Fsettings");
});

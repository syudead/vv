import { expect, test } from "@playwright/test";

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
      message: "ログインが必要です",
    });
  }

  const nextSession = await request.get("/api/auth/session?next=%2Fsettings");
  expect(await nextSession.json()).toEqual({ state: "setupRequired" });

  // 稼働確認は未設定でも同じ形で返る。
  const health = await request.get("/api/health");
  expect(health.status()).toBe(200);
  expect(((await health.json()) as { status: string }).status).toBe("ok");
});

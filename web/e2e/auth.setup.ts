import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test as setup } from "@playwright/test";

import { ownerAccount, ownerStorageState } from "./owner-account";

// 所有者のアカウントを作ってログインし、その Cookie を既存の e2e すべてに渡す。
// CI の再実行で、アカウントが前の実行で作られていたら（409）ログインに切り替える。
setup("所有者のアカウントを作ってログインする", async ({ request }) => {
  const created = await request.post("/api/auth/setup", { data: ownerAccount });
  if (created.status() === 409) {
    const login = await request.post("/api/auth/login", { data: ownerAccount });
    expect(login.status()).toBe(200);
  } else {
    expect(created.status()).toBe(200);
    expect(await created.json()).toEqual({ redirectTo: "/" });
  }

  const session = await request.get("/api/auth/session");
  expect(await session.json()).toEqual({ state: "owner" });
  const videos = await request.get("/api/videos");
  expect(videos.status()).toBe(200);
  expect(videos.headers()["x-vv-audience"]).toBe("owner");

  await mkdir(path.dirname(ownerStorageState), { recursive: true });
  await request.storageState({ path: ownerStorageState });
});

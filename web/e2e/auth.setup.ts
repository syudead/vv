import { mkdir } from "node:fs/promises";
import path from "node:path";

import { expect, test as setup } from "@playwright/test";

import { ownerAccount, ownerStorageState } from "./owner-account";

// 所有者のアカウントを作ってログインし、オンボードを終えた段階にして、その Cookie を
// e2e すべてに渡す。CI の再試行でアカウントがもうあるときは 409 を受けてログインに切り替える。
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

  // 既存の e2e は自分でメディアフォルダを足すので、オンボードを終えた所有者として走る。
  // 終えていなければ、フォルダの無い間はどの画面も /welcome へ送られる
  // （docs/design-docs/home.md・R-4）。
  const onboarding = await request.put("/api/onboarding", { data: { step: "done" } });
  expect(onboarding.status()).toBe(200);

  await mkdir(path.dirname(ownerStorageState), { recursive: true });
  await request.storageState({ path: ownerStorageState });
});

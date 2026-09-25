import path from "node:path";

// e2e の所有者のアカウント。auth.setup.ts が作る。同じ送信元のログインの失敗は
// 5 分に 5 回で制限されるので、e2e 全体の失敗の試行はそれより少なく収める
// （specs/016-single-account-auth/contracts/auth-api.md §3）。
export const ownerAccount = {
  username: "e2e-owner",
  password: "e2e-owner-password",
} as const;

const runRoot = process.env.MDM_E2E_RUN_ROOT;
if (runRoot === undefined) throw new Error("MDM_E2E_RUN_ROOT is not configured");

// 所有者としてログインした状態（Cookie）の置き場。auth.setup.ts が書き、既存の
// e2e すべてが playwright.config.ts の storageState として読む。
export const ownerStorageState = path.join(runRoot, "auth", "owner.json");

import path from "node:path";

// Keep failed login attempts across the suite below the limit of five per source in five minutes.
export const ownerAccount = {
  username: "e2e-owner",
  password: "e2e-owner-password",
} as const;

const runRoot = process.env.MDM_E2E_RUN_ROOT;
if (runRoot === undefined) throw new Error("MDM_E2E_RUN_ROOT is not configured");

// auth.setup.ts writes this owner session; playwright.config.ts shares it with e2e tests.
export const ownerStorageState = path.join(runRoot, "auth", "owner.json");

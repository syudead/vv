// @vitest-environment node
import { readdirSync } from "node:fs";
import { join } from "node:path";

import { expect, it } from "vitest";

it("keeps extensionless source module paths unambiguous on case-insensitive filesystems", () => {
  const modules = new Map<string, string[]>();
  // Vite tries extensions in order; Foo.tsx and foo.ts can resolve to the same file on Windows.
  for (const file of readdirSync(join(__dirname, ".."), {
    recursive: true,
    encoding: "utf8",
  })) {
    if (!/\.[cm]?[jt]sx?$/.test(file) || /\.d\.[cm]?ts$/.test(file)) continue;
    const key = file.replace(/\.[cm]?[jt]sx?$/, "").toLowerCase();
    const matches = modules.get(key) ?? [];
    matches.push(file);
    modules.set(key, matches);
  }
  expect([...modules.values()].filter((files) => files.length > 1)).toEqual([]);
});

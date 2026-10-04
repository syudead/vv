// @vitest-environment node
import { existsSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import designExceptions, { type DesignException } from "../../design-exceptions.js";

/**
 * デザインシステムの検査の例外一覧（web/design-exceptions.js）を検査する
 * （specs/038-design-system/contracts/registry.md の Exception list）。消えたファイルを
 * 指す項目と、理由の無い special の項目で落ちる。
 */

const src = join(__dirname, "..");
const rules = new Set<string>([
  "no-restricted-syntax",
  "better-tailwindcss/no-restricted-classes",
  "better-tailwindcss/no-unknown-classes",
]);

function problems(
  entries: readonly DesignException[],
  exists: (file: string) => boolean,
): string[] {
  return entries.flatMap((entry) => {
    const found: string[] = [];
    if (!exists(entry.file)) {
      found.push(`${entry.file}: names a missing file`);
    }
    if (entry.kind !== "migration" && entry.kind !== "special") {
      found.push(`${entry.file}: kind must be migration or special`);
    }
    if (entry.kind === "special" && !entry.reason?.trim()) {
      found.push(`${entry.file}: a special entry needs a reason`);
    }
    if (entry.rules.length === 0) {
      found.push(`${entry.file}: names no rule`);
    }
    for (const rule of entry.rules) {
      if (!rules.has(rule)) {
        found.push(`${entry.file}: unknown rule ${rule}`);
      }
    }
    return found;
  });
}

describe("design-system exception list", () => {
  it("names existing files, known rules and a reason for every special entry", () => {
    expect(problems(designExceptions, (file) => existsSync(join(src, file)))).toEqual([]);
  });

  it("lists each file once", () => {
    const files = designExceptions.map((entry) => entry.file);
    expect(files.filter((file, index) => files.indexOf(file) !== index)).toEqual([]);
  });

  it("reports a missing file and a special entry without a reason", () => {
    const entries: DesignException[] = [
      { file: "gone/Missing.tsx", rules: ["no-restricted-syntax"], kind: "migration" },
      {
        file: "designSystem/DesignSystemPage.tsx",
        rules: ["better-tailwindcss/no-restricted-classes"],
        kind: "special",
      },
    ];
    expect(problems(entries, (file) => existsSync(join(src, file)))).toEqual([
      "gone/Missing.tsx: names a missing file",
      "designSystem/DesignSystemPage.tsx: a special entry needs a reason",
    ]);
  });
});

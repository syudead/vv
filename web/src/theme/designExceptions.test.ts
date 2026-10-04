// @vitest-environment node
import { existsSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import designExceptions, { type DesignException } from "../../design-exceptions.js";

/**
 * デザインシステムの検査の例外一覧（web/design-exceptions.js）を検査する
 * （specs/038-design-system/contracts/registry.md の Exception list）。消えたファイルを
 * 指す項目、理由の無い special の項目と、残った migration の項目で落ちる。テーマの
 * 名前空間を空にした後は、画面はすべてデザインシステムに移っており、残る例外は
 * デザインシステムで表せない見た目（special）だけである（research.md R-9）。
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
    if (entry.kind === "migration") {
      found.push(
        `${entry.file}: migration entries are no longer allowed; move the file onto the design system or make it special with a reason`,
      );
    } else if (entry.kind !== "special") {
      found.push(`${entry.file}: kind must be special`);
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

  it("reports a missing file, a migration entry and a special entry without a reason", () => {
    const entries: DesignException[] = [
      {
        file: "gone/Missing.tsx",
        rules: ["no-restricted-syntax"],
        kind: "special",
        reason: "Gone.",
      },
      {
        file: "designSystem/DesignSystemPage.tsx",
        rules: ["better-tailwindcss/no-restricted-classes"],
        kind: "migration",
      },
      {
        file: "designSystem/Foundations.tsx",
        rules: ["better-tailwindcss/no-restricted-classes"],
        kind: "special",
      },
    ];
    expect(problems(entries, (file) => existsSync(join(src, file)))).toEqual([
      "gone/Missing.tsx: names a missing file",
      "designSystem/DesignSystemPage.tsx: migration entries are no longer allowed; move the file onto the design system or make it special with a reason",
      "designSystem/Foundations.tsx: a special entry needs a reason",
    ]);
  });
});

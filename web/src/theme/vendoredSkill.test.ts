// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 取り込んだ shadcn スキルは、リポジトリが固定した CLI だけを呼ぶ
 * （specs/038-design-system/research.md R-7）。上流のスキルは npx shadcn@latest を使うので、
 * 更新で取り込み直したときに置き換えを忘れるとここで落ちる。
 */

const skill = join(__dirname, "..", "..", "..", ".agents", "skills", "shadcn");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? walk(path) : [path];
  });
}

describe("vendored shadcn skill", () => {
  // VENDORED.md は取り込みの記録で、置き換えた元の書き方を説明するために shadcn@ を書く。
  const files = walk(skill).filter(
    (path) => /\.(md|ya?ml|json)$/.test(path) && !path.endsWith("VENDORED.md"),
  );

  it("has files to check", () => {
    expect(files.length).toBeGreaterThan(0);
  });

  it("runs the CLI pinned in web/package.json, never shadcn@<version>", () => {
    const offenders = files.filter((path) =>
      readFileSync(path, "utf8").includes("shadcn@"),
    );
    expect(offenders.map((path) => path.slice(skill.length))).toEqual([]);
  });
});

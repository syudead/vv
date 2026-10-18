// @vitest-environment node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/** Vendored skills must use the pinned CLI; catch reintroduced shadcn@latest during upstream updates. */

const skill = join(__dirname, "..", "..", "..", ".agents", "skills", "shadcn");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? walk(path) : [path];
  });
}

describe("vendored shadcn skill", () => {
  // VENDORED.md may mention the original command as provenance.
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

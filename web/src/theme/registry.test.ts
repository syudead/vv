// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import { basename, join, relative, sep } from "node:path";

import { describe, expect, it } from "vitest";

// Every component file in web/src/ui is a registry item and has its section in components.md (web/registry/rules/components.md).

const web = join(__dirname, "..", "..");

interface Registry {
  items: { files?: { path: string }[] }[];
}

function componentFiles(): string[] {
  return readdirSync(join(web, "src", "ui"), { recursive: true, encoding: "utf8" })
    .filter((file) => file.endsWith(".tsx") && !file.endsWith(".test.tsx"))
    .map((file) =>
      relative(web, join(web, "src", "ui", file))
        .split(sep)
        .join("/"),
    )
    .sort();
}

describe("design-system registry", () => {
  it("publishes every component file in web/src/ui as a registry item's file", () => {
    const registry = JSON.parse(
      readFileSync(join(web, "registry.json"), "utf8"),
    ) as Registry;
    const published = new Set(
      registry.items.flatMap((item) => (item.files ?? []).map((file) => file.path)),
    );
    expect(componentFiles().filter((file) => !published.has(file))).toEqual([]);
  });

  it("gives every PascalCase component its own section in components.md", () => {
    const rules = readFileSync(join(web, "registry", "rules", "components.md"), "utf8");
    const headings = new Set(
      [...rules.matchAll(/^### (.+)$/gm)].map((match) => match[1]?.trim()),
    );
    const names = componentFiles()
      .map((file) => basename(file, ".tsx"))
      .filter((name) => /^[A-Z]/.test(name));
    expect(names.filter((name) => !headings.has(name))).toEqual([]);
  });
});

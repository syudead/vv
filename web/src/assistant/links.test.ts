import { describe, expect, it } from "vitest";

import { atDestination, criteriaPath, destinationPath, showsCriteria } from "./links";
import { libraryRule } from "./screenContext";

describe("criteriaPath", () => {
  it("ライブラリが読み戻すと同じ条件になる URL を作る", () => {
    const rule = {
      q: "夏",
      favorite: true,
      tags: [9, 3],
      tagKind: 2,
      person: 5,
      organization: 6,
      minDurationMs: 600_000,
      maxDurationMs: 1_800_000,
      addedFrom: "2025-01-01",
      addedTo: "2025-12-31",
      watchState: "unwatched" as const,
    };
    const path = criteriaPath({ rule, labels: [] });

    const [pathname, search = ""] = path.split("?");
    expect(pathname).toBe("/");
    expect(libraryRule(`?${search}`)).toEqual({ ...rule, tags: [3, 9] });
    expect(showsCriteria("/", `?${search}&sort=addedDesc`, { rule, labels: [] })).toBe(
      true,
    );
    expect(showsCriteria("/", "?tag=3", { rule, labels: [] })).toBe(false);
  });
});

describe("destinationPath", () => {
  it("名前の付いた物はその画面、一覧と設定はそのルートへ", () => {
    expect(destinationPath({ kind: "person", id: 4 })).toBe("/people/4");
    expect(destinationPath({ kind: "organization", id: 7 })).toBe("/?organization=7");
    expect(destinationPath({ kind: "tag", id: 2 })).toBe("/?tag=2");
    expect(destinationPath({ kind: "history" })).toBe("/history");
    expect(destinationPath({ kind: "settings", settingsPage: "connections" })).toBe(
      "/settings/connections",
    );
  });
});

describe("atDestination", () => {
  it("移った先に居続ける間だけ真で、ページが足した URL の値は無視する", () => {
    expect(atDestination("/", "?organization=7&sort=addedDesc", "/?organization=7")).toBe(
      true,
    );
    expect(atDestination("/", "", "/?organization=7")).toBe(false);
    expect(atDestination("/people/4", "", "/people/4")).toBe(true);
    expect(atDestination("/people/5", "", "/people/4")).toBe(false);
  });
});

import { describe, expect, it } from "vitest";

import type { Credit } from "../api/credits";
import { parentOf, reparentRows } from "./creditRows";

function ref(id: number, name: string) {
  return { id, name, provisional: false };
}

function organization(id: number, name: string, parent?: ReturnType<typeof ref>) {
  const row: Credit = {
    id,
    name,
    aliases: [],
    externalIds: [],
    videoCount: 0,
    createdAt: "2026-01-01T00:00:00Z",
  };
  return parent === undefined ? row : { ...row, parent };
}

describe("reparentRows", () => {
  const studio = organization(10, "Studio");
  const label = organization(11, "Label", ref(10, "Studio"));
  const other = organization(12, "Other", ref(13, "Elsewhere"));
  const rows = [studio, label, other];

  it("改名した団体を親に持つ子の親の名前を書き換え、ほかの行はそのまま", () => {
    const next = reparentRows(rows, 10, ref(10, "New Studio"));
    expect(next.map(parentOf)).toEqual([
      undefined,
      ref(10, "New Studio"),
      ref(13, "Elsewhere"),
    ]);
    expect(next[0]).toBe(studio);
    expect(next[2]).toBe(other);
  });

  it("統合では統合元の子を統合先の下へ付け替える", () => {
    const next = reparentRows(rows, 10, ref(12, "Other"));
    expect(parentOf(next[1]!)).toEqual(ref(12, "Other"));
  });

  it("削除では子の親を外す", () => {
    const next = reparentRows(rows, 10, undefined);
    expect(parentOf(next[1]!)).toBeUndefined();
    expect("parent" in next[1]!).toBe(false);
  });

  it("子が無ければ同じ配列を返す", () => {
    expect(reparentRows(rows, 99, undefined)).toBe(rows);
  });
});

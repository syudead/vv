import { describe, expect, it } from "vitest";

import type { Credit } from "../api/credits";
import { parentOf, reparentRows } from "./creditRows";

function organization(id: number, name: string, parent?: { id: number; name: string }) {
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
  const label = organization(11, "Label", { id: 10, name: "Studio" });
  const other = organization(12, "Other", { id: 13, name: "Elsewhere" });
  const rows = [studio, label, other];

  it("改名した団体を親に持つ子の親の名前を書き換え、ほかの行はそのまま", () => {
    const next = reparentRows(rows, 10, { id: 10, name: "New Studio" });
    expect(next.map(parentOf)).toEqual([
      undefined,
      { id: 10, name: "New Studio" },
      { id: 13, name: "Elsewhere" },
    ]);
    expect(next[0]).toBe(studio);
    expect(next[2]).toBe(other);
  });

  it("統合では統合元の子を統合先の下へ付け替える", () => {
    const next = reparentRows(rows, 10, { id: 12, name: "Other" });
    expect(parentOf(next[1]!)).toEqual({ id: 12, name: "Other" });
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

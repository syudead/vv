import { describe, expect, it } from "vitest";

import type { HomeShelf } from "../api/home";
import {
  folderShelfIds,
  shelfMoves,
  withShelfAt,
  withShelfMoved,
  withShelfRemoved,
} from "./shelfOrder";

// ホームのフォルダの棚の並びの規則（specs/057-home/ui-design.md「Home」、research.md R-7）。

const folder = (id: number): HomeShelf => ({
  key: `folder:${String(id)}`,
  kind: "folder",
  name: `Folder ${String(id)}`,
  folderId: id,
});

describe("folderShelfIds", () => {
  it("フォルダの棚の id だけを上から順に取り出す", () => {
    const shelves: HomeShelf[] = [
      folder(4),
      folder(2),
      { key: "continue", kind: "continue", name: "" },
      { key: "recent", kind: "recent", name: "" },
    ];
    expect(folderShelfIds(shelves)).toEqual([4, 2]);
  });
});

describe("withShelfMoved", () => {
  it("1 つ上か下の棚と入れ替える", () => {
    expect(withShelfMoved([1, 2, 3], 2, -1)).toEqual([2, 1, 3]);
    expect(withShelfMoved([1, 2, 3], 2, 1)).toEqual([1, 3, 2]);
  });

  it("端より先と、無い id では並びを変えない", () => {
    expect(withShelfMoved([1, 2, 3], 1, -1)).toEqual([1, 2, 3]);
    expect(withShelfMoved([1, 2, 3], 3, 1)).toEqual([1, 2, 3]);
    expect(withShelfMoved([1, 2, 3], 9, 1)).toEqual([1, 2, 3]);
  });
});

describe("withShelfRemoved と withShelfAt", () => {
  it("外した棚を、外す前の位置に戻せる", () => {
    const removed = withShelfRemoved([1, 2, 3], 2);
    expect(removed).toEqual({ ids: [1, 3], index: 1 });
    expect(withShelfAt(removed.ids, 2, removed.index)).toEqual([1, 2, 3]);
  });

  it("並びより先か負の位置なら最後に置き、もうある id は動かさない", () => {
    expect(withShelfAt([1, 3], 2, 5)).toEqual([1, 3, 2]);
    expect(withShelfAt([1, 3], 2, -1)).toEqual([1, 3, 2]);
    expect(withShelfAt([1, 3], 3, 0)).toEqual([1, 3]);
  });
});

describe("shelfMoves", () => {
  it("最初の棚に Move up、最後の棚に Move down を出さない", () => {
    expect(shelfMoves([1, 2, 3], 1)).toEqual({ up: false, down: true });
    expect(shelfMoves([1, 2, 3], 2)).toEqual({ up: true, down: true });
    expect(shelfMoves([1, 2, 3], 3)).toEqual({ up: true, down: false });
    expect(shelfMoves([1], 1)).toEqual({ up: false, down: false });
  });
});

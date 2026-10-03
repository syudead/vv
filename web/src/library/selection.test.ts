import { describe, expect, it } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import {
  emptySelection,
  favoriteTargets,
  fromSelectAll,
  reconcileGroups,
  sameSelection,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "./selection";

/** 選択の持ち方（specs/035-favorites/research.md R-7）。 */
const series = { folder: { rootId: 3, path: "series" }, videoIds: [101, 102, 103] };

function videoItem(id: number): LibraryItem {
  return { kind: "video", video: { id } as Video };
}

function groupItem(videoIds: number[]): LibraryItem {
  const group = { folder: { rootId: 3, path: "series" }, videoIds } as LibraryGroup;
  return { kind: "group", group };
}

describe("LibrarySelection", () => {
  it("グループのチェックはメンバーを入れてグループとして覚え、お気に入りではグループだけを送る", () => {
    const selection = setGroup(setVideo(emptySelection, 1, true), series, true);
    expect(Array.from(selection.ids)).toEqual([1, 101, 102, 103]);
    expect(favoriteTargets(selection)).toEqual({
      videoIds: [1],
      folders: [{ rootId: 3, path: "series" }],
    });
  });

  it("メンバーを 1 本外すと、グループではなく残ったメンバーを動画として送り、戻してもグループに戻らない", () => {
    const grouped = setGroup(emptySelection, series, true);
    const removed = setVideo(grouped, 102, false);
    expect(favoriteTargets(removed)).toEqual({ videoIds: [101, 103], folders: [] });
    const back = toggleVideo(removed, 102);
    expect(back.ids.size).toBe(3);
    expect(back.groups.size).toBe(0);
    expect(sameSelection(back, grouped)).toBe(false);
  });

  it("タグの行の切り替えは、全メンバーが入っていれば外し、そうでなければグループとして選ぶ", () => {
    const partly = setVideo(emptySelection, 101, true);
    const grouped = toggleGroup(partly, series);
    expect(grouped.groups.size).toBe(1);
    const cleared = toggleGroup(grouped, series);
    expect(cleared.ids.size).toBe(0);
    expect(cleared.groups.size).toBe(0);
  });

  it("「すべて選択」の応答は ids と groups の両方が同じときだけ同じ選択とみなす", () => {
    const all = fromSelectAll([1, 101, 102, 103], [series]);
    expect(
      sameSelection(all, setGroup(setVideo(emptySelection, 1, true), series, true)),
    ).toBe(true);
    expect(sameSelection(all, fromSelectAll([1, 101, 102, 103], []))).toBe(false);
  });

  it("取り直した一覧でメンバーが動画の項目なら、グループから外してメンバーを動画として送る", () => {
    const selection = setGroup(setVideo(emptySelection, 1, true), series, true);
    const reconciled = reconcileGroups(selection, [
      videoItem(1),
      videoItem(101),
      videoItem(102),
    ]);
    expect(Array.from(reconciled.ids)).toEqual([1, 101, 102, 103]);
    expect(favoriteTargets(reconciled)).toEqual({
      videoIds: [1, 101, 102, 103],
      folders: [],
    });
  });

  it("取り直したグループのメンバーが減っても全員選択中ならグループのまま覚え直し、増えたら外す", () => {
    const selection = setGroup(emptySelection, series, true);
    const shrunk = reconcileGroups(selection, [groupItem([101, 102])]);
    expect(shrunk.groups.get("3\0series")?.videoIds).toEqual([101, 102]);
    expect(favoriteTargets(shrunk)).toEqual({
      videoIds: [103],
      folders: [{ rootId: 3, path: "series" }],
    });
    const grown = reconcileGroups(selection, [groupItem([101, 102, 103, 104])]);
    expect(grown.groups.size).toBe(0);
    expect(favoriteTargets(grown).videoIds).toEqual([101, 102, 103]);
  });

  it("変化が無ければ同じ選択を返す", () => {
    const selection = setGroup(setVideo(emptySelection, 1, true), series, true);
    expect(reconcileGroups(selection, [videoItem(1), groupItem([101, 102, 103])])).toBe(
      selection,
    );
    expect(reconcileGroups(selection, [])).toBe(selection);
  });
});

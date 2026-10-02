import { describe, expect, it } from "vitest";

import {
  emptySelection,
  favoriteTargets,
  fromSelectAll,
  sameSelection,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "./selection";

/** 選択の持ち方（specs/035-favorites/research.md R-7）。 */
const series = { folder: { rootId: 3, path: "series" }, videoIds: [101, 102, 103] };

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
});

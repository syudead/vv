import { describe, expect, it } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import {
  emptySelection,
  favoriteTargets,
  fromSelectAll,
  isGroupSelected,
  keepMembers,
  reconcileGroups,
  sameSelection,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "./selection";

/** 選択の持ち方。 */
const series = {
  id: 5,
  videoIds: [101, 102, 103],
};

function videoItem(id: number): LibraryItem {
  return { kind: "video", video: { id } as Video };
}

function groupItem(videoIds: number[]): LibraryItem {
  const group = {
    id: 5,
    folder: { rootId: 3, path: "series" },
    videoIds,
  } as LibraryGroup;
  return { kind: "group", group };
}

describe("LibrarySelection", () => {
  it("グループのチェックはメンバーを入れてグループとして覚え、お気に入りではグループだけを送る", () => {
    const selection = setGroup(setVideo(emptySelection, 1, true), series, true);
    expect(Array.from(selection.ids)).toEqual([1, 101, 102, 103]);
    expect(favoriteTargets(selection)).toEqual({
      videoIds: [1],
      groupIds: [5],
    });
  });

  it("メンバーを 1 本外すと、グループではなく残ったメンバーを動画として送り、戻してもグループに戻らない", () => {
    const grouped = setGroup(emptySelection, series, true);
    const removed = setVideo(grouped, 102, false);
    expect(favoriteTargets(removed)).toEqual({
      videoIds: [101, 103],
      groupIds: [],
    });
    const back = toggleVideo(removed, 102);
    expect(back.ids.size).toBe(3);
    expect(back.groups.size).toBe(0);
    expect(sameSelection(back, grouped)).toBe(false);
  });

  it("タグの行の切り替えは、グループとして選んでいれば外し、そうでなければグループとして選ぶ", () => {
    const partly = setVideo(emptySelection, 101, true);
    const grouped = toggleGroup(partly, series);
    expect(grouped.groups.size).toBe(1);
    const cleared = toggleGroup(grouped, series);
    expect(cleared.ids.size).toBe(0);
    expect(cleared.groups.size).toBe(0);
  });

  it("メンバーを共有する 2 つのグループの片方を外すと、もう片方のメンバーとグループは残る", () => {
    const g1 = { id: 1, videoIds: [11, 12] };
    const g2 = { id: 2, videoIds: [12, 13] };
    const both = setGroup(setGroup(emptySelection, g1, true), g2, true);
    const deselected = setGroup(both, g1, false);
    expect(Array.from(deselected.ids).sort()).toEqual([12, 13]);
    expect(Array.from(deselected.groups.keys())).toEqual([2]);
    expect(favoriteTargets(deselected)).toEqual({ videoIds: [], groupIds: [2] });
    const toggled = toggleGroup(both, g1);
    expect(sameSelection(toggled, deselected)).toBe(true);
  });

  it("共有するメンバーしか持たないグループを外しても、グループだけが外れて動画は残る", () => {
    const inner = { id: 1, videoIds: [12] };
    const outer = { id: 2, videoIds: [11, 12] };
    const both = setGroup(setGroup(emptySelection, outer, true), inner, true);
    const deselected = setGroup(both, inner, false);
    expect(Array.from(deselected.ids).sort()).toEqual([11, 12]);
    expect(Array.from(deselected.groups.keys())).toEqual([2]);
  });

  it("全メンバーを共有するグループを外すと、動画は残ってもカードは外れて見え、もう一度選べる", () => {
    // G1={B}・G2={B,C}（B=12、C=13）。
    const g1 = { id: 1, videoIds: [12] };
    const g2 = { id: 2, videoIds: [12, 13] };
    const both = setGroup(setGroup(emptySelection, g1, true), g2, true);
    expect(isGroupSelected(both, 1)).toBe(true);
    expect(isGroupSelected(both, 2)).toBe(true);

    const deselected = setGroup(both, g1, false);
    expect(Array.from(deselected.ids).sort()).toEqual([12, 13]);
    expect(isGroupSelected(deselected, 1)).toBe(false);
    expect(isGroupSelected(deselected, 2)).toBe(true);
    expect(favoriteTargets(deselected)).toEqual({ videoIds: [], groupIds: [2] });

    const toggledOff = toggleGroup(both, g1);
    expect(sameSelection(toggledOff, deselected)).toBe(true);
    const reselected = toggleGroup(toggledOff, g1);
    expect(isGroupSelected(reselected, 1)).toBe(true);
    expect(sameSelection(reselected, both)).toBe(true);
  });

  it("メンバーを動画のチェックで全部選んでも、グループのカードは選んだと見えない", () => {
    const videos = series.videoIds.reduce(
      (selection, id) => setVideo(selection, id, true),
      emptySelection,
    );
    expect(isGroupSelected(videos, series.id)).toBe(false);
    const grouped = toggleGroup(videos, series);
    expect(isGroupSelected(grouped, series.id)).toBe(true);
    expect(grouped.ids.size).toBe(3);
  });

  it("「すべて選択」の応答は ids と groups の両方が同じときだけ同じ選択とみなす", () => {
    const all = fromSelectAll([1, 101, 102, 103], [{ id: 5, videoIds: series.videoIds }]);
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
      groupIds: [],
    });
  });

  it("取り直したグループのメンバーが減っても全員選択中ならグループのまま覚え直し、増えたら外す", () => {
    const selection = setGroup(emptySelection, series, true);
    const shrunk = reconcileGroups(selection, [groupItem([101, 102])]);
    expect(shrunk.groups.get(5)?.videoIds).toEqual([101, 102]);
    expect(favoriteTargets(shrunk)).toEqual({
      videoIds: [103],
      groupIds: [5],
    });
    const grown = reconcileGroups(selection, [groupItem([101, 102, 103, 104])]);
    expect(grown.groups.size).toBe(0);
    expect(favoriteTargets(grown).videoIds).toEqual([101, 102, 103]);
  });

  it("「すべて選択」の groups は、読み込んでいないグループも id で送る", () => {
    const all = fromSelectAll(
      [101, 102, 103, 201],
      [
        { id: 5, videoIds: [101, 102, 103] },
        { id: 6, videoIds: [201] },
      ],
    );
    // 選んだグループのメンバーは動画として送らない。
    expect(favoriteTargets(all)).toEqual({ videoIds: [], groupIds: [5, 6] });
  });

  it("変化が無ければ同じ選択を返す", () => {
    const selection = setGroup(setVideo(emptySelection, 1, true), series, true);
    expect(reconcileGroups(selection, [videoItem(1), groupItem([101, 102, 103])])).toBe(
      selection,
    );
    expect(reconcileGroups(selection, [])).toBe(selection);
  });
});

describe("keepMembers", () => {
  it("並ばなくなった動画を外し、その動画をメンバーに持つグループも外す", () => {
    const selection = fromSelectAll([1, 2, 3], [{ id: 7, videoIds: [2, 3] }]);
    const kept = keepMembers(selection, new Set([1, 2]));
    expect([...kept.ids]).toEqual([1, 2]);
    expect(kept.groups.size).toBe(0);
  });

  it("どれも並んでいれば同じ選択を返す", () => {
    const selection = fromSelectAll([1, 2], [{ id: 7, videoIds: [1, 2] }]);
    expect(keepMembers(selection, new Set([1, 2, 3]))).toBe(selection);
  });
});

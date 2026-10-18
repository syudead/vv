import { describe, expect, it } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { GroupItem } from "../api/groups";
import { emptySelection, setGroup, setVideo } from "../library/selection";
import { appendItems, reconcileGroupSelection, selectedItemIds } from "./groupItems";

function videoItem(id: number, videoId: number): GroupItem {
  return { id, kind: "video", video: { id: videoId } as Video };
}

function groupItem(id: number, groupId: number, videoIds: number[]): GroupItem {
  return { id, kind: "group", group: { id: groupId, videoIds } as LibraryGroup };
}

describe("appendItems", () => {
  it("続きのページを後ろへ足し、前のページにある項目は足さない", () => {
    const first = [videoItem(1, 10), videoItem(2, 11)];
    const page = [videoItem(2, 11), videoItem(3, 12)];
    expect(appendItems(first, page).map((item) => item.id)).toEqual([1, 2, 3]);
  });
});

describe("reconcileGroupSelection", () => {
  it("もう並んでいない子グループを選んだグループから外し、そのメンバーは動画として残す", () => {
    let selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    selection = setGroup(selection, { id: 21, videoIds: [11] }, true);
    const kept = reconcileGroupSelection(selection, [10, 11], [groupItem(1, 20, [10])]);
    expect([...kept.groups.keys()]).toEqual([20]);
    expect([...kept.ids].sort()).toEqual([10, 11]);
  });

  it("グループの videoIds に無い動画を外し、読んでいないページの動画は残す", () => {
    let selection = setVideo(emptySelection, 10, true);
    selection = setVideo(selection, 99, true);
    selection = setVideo(selection, 12, true);
    const kept = reconcileGroupSelection(selection, [10, 11, 12], [videoItem(1, 10)]);
    expect([...kept.ids].sort()).toEqual([10, 12]);
  });

  it("メンバーが増えた子グループは、新しいメンバーが選ばれていなければ選んだグループから外す", () => {
    const selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    const kept = reconcileGroupSelection(
      selection,
      [10, 11],
      [groupItem(1, 20, [10, 11])],
    );
    expect(kept.groups.size).toBe(0);
    expect([...kept.ids]).toEqual([10]);
  });

  it("全メンバーが選ばれていれば今のメンバーで覚え直す", () => {
    let selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    selection = setVideo(selection, 11, true);
    const kept = reconcileGroupSelection(
      selection,
      [10, 11],
      [groupItem(1, 20, [11, 10])],
    );
    expect(kept.groups.get(20)?.videoIds).toEqual([11, 10]);
  });

  it("子グループのメンバーが直接の動画の項目として並んでも、子グループの選択を残す", () => {
    const selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    const items = [videoItem(1, 10), groupItem(2, 20, [10])];
    expect(reconcileGroupSelection(selection, [10], items)).toBe(selection);
  });

  it("変わらなければ同じ選択を返す", () => {
    const selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    expect(reconcileGroupSelection(selection, [10], [groupItem(1, 20, [10])])).toBe(
      selection,
    );
  });
});

describe("selectedItemIds", () => {
  it("選んだグループの項目と、選んだグループのメンバーでない選んだ動画の項目を返す", () => {
    // Video 10 is both a direct item and a member of child group 20.
    const items = [videoItem(1, 10), groupItem(2, 20, [10]), videoItem(3, 11)];
    const onlyVideo = setVideo(emptySelection, 10, true);
    expect(selectedItemIds(onlyVideo, items)).toEqual([1]);

    const withGroup = setVideo(
      setGroup(emptySelection, { id: 20, videoIds: [10] }, true),
      11,
      true,
    );
    expect(selectedItemIds(withGroup, items)).toEqual([2, 3]);
  });
});

import { describe, expect, it } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { GroupItem } from "../api/groups";
import { emptySelection, setGroup, setVideo } from "../library/selection";
import { appendItems, keepListedGroups, selectAllInGroup } from "./groupItems";

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

describe("selectAllInGroup", () => {
  it("動画はグループの videoIds、選んだグループは並んだ子グループにする", () => {
    const items = [groupItem(1, 20, [10, 11]), videoItem(2, 10)];
    const selection = selectAllInGroup([10, 11, 12], items);
    expect([...selection.ids].sort()).toEqual([10, 11, 12]);
    expect([...selection.groups.keys()]).toEqual([20]);
    expect(selection.groups.get(20)?.videoIds).toEqual([10, 11]);
  });
});

describe("keepListedGroups", () => {
  it("もう並んでいない子グループを選んだグループから外し、選んだ動画は残す", () => {
    let selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    selection = setGroup(selection, { id: 21, videoIds: [11] }, true);
    selection = setVideo(selection, 99, true);
    const kept = keepListedGroups(selection, [groupItem(1, 20, [10])]);
    expect([...kept.groups.keys()]).toEqual([20]);
    expect([...kept.ids].sort()).toEqual([10, 11, 99]);
  });

  it("変わらなければ同じ選択を返す", () => {
    const selection = setGroup(emptySelection, { id: 20, videoIds: [10] }, true);
    expect(keepListedGroups(selection, [groupItem(1, 20, [10])])).toBe(selection);
  });
});

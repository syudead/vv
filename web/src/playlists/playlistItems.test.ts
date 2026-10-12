import { describe, expect, it } from "vitest";

import type { LibraryGroup, Video } from "../api/client";
import type { PlaylistItem } from "../api/playlists";
import {
  dropItem,
  focusAfterRemove,
  itemTitle,
  moveItem,
  playlistDuration,
  playTarget,
} from "./playlistItems";

function videoItem(id: number, videoId: number, durationMs?: number): PlaylistItem {
  return {
    id,
    kind: "video",
    video: { id: videoId, title: `Video ${String(videoId)}`, durationMs } as Video,
  };
}

function groupItem(id: number, openVideoId: number, durationMs?: number): PlaylistItem {
  return {
    id,
    kind: "group",
    group: { name: "Series", openVideoId, durationMs } as LibraryGroup,
  };
}

const rows = [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }];

describe("playlistDuration", () => {
  it("見せる項目の長さを足して「1:32:10」の形にする", () => {
    expect(
      playlistDuration([videoItem(1, 10, 3_600_000), groupItem(2, 20, 1_930_000)]),
    ).toBe("1:32:10");
  });

  it("長さの分からない項目があるか、項目が無ければ空にする", () => {
    expect(playlistDuration([videoItem(1, 10, 60_000), videoItem(2, 11)])).toBe("");
    expect(playlistDuration([])).toBe("");
  });
});

describe("playTarget", () => {
  it("動画はその動画を、グループは openVideoId を、プレイリストと項目を付けて開く", () => {
    expect(playTarget(7, videoItem(3, 42))).toEqual({
      to: "/videos/42?playlist=7&item=3",
      state: { from: "/playlists/7", autoplay: true },
    });
    expect(playTarget(7, groupItem(4, 99))?.to).toBe("/videos/99?playlist=7&item=4");
  });

  it("グループの項目の名前はグループ名である", () => {
    expect(itemTitle(groupItem(4, 99))).toBe("Series");
    expect(itemTitle(videoItem(3, 42))).toBe("Video 42");
  });
});

describe("dropItem", () => {
  it("下へ置くと、新しい位置の上にある項目を afterItemId にする", () => {
    const drop = dropItem(rows, 1, 3);
    expect(drop?.items.map((row) => row.id)).toEqual([2, 3, 1, 4]);
    expect(drop?.afterItemId).toBe(3);
  });

  it("先頭へ置くと afterItemId は null", () => {
    const drop = dropItem(rows, 3, 1);
    expect(drop?.items.map((row) => row.id)).toEqual([3, 1, 2, 4]);
    expect(drop?.afterItemId).toBeNull();
  });

  it("同じ位置に置いたら何も送らない", () => {
    expect(dropItem(rows, 2, 2)).toBeNull();
  });
});

describe("moveItem", () => {
  it("サーバーの move と同じく、afterItemId のすぐ後ろへ移す", () => {
    expect(moveItem(rows, 1, 3).map((row) => row.id)).toEqual([2, 3, 1, 4]);
    expect(moveItem(rows, 4, null).map((row) => row.id)).toEqual([4, 1, 2, 3]);
  });

  it("並びに無い項目なら元の並びのまま", () => {
    expect(moveItem(rows, 9, 1)).toBe(rows);
    expect(moveItem(rows, 1, 9)).toBe(rows);
  });
});

describe("focusAfterRemove", () => {
  it("次の行、無ければ前の行、どちらも無ければ null", () => {
    expect(focusAfterRemove(rows, 2)).toBe(3);
    expect(focusAfterRemove(rows, 4)).toBe(3);
    expect(focusAfterRemove([{ id: 1 }], 1)).toBeNull();
  });
});

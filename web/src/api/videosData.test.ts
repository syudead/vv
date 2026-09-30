import { describe, expect, it } from "vitest";

import type { LibraryItem, Video } from "./client";
import { videoItem } from "./libraryItems";
import { type VideosData, videosDataReducer } from "./videosData";

/** video は一覧の 1 件を作る。 */
function video(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
  };
}

function items(ids: number[]): LibraryItem[] {
  return ids.map((id) => videoItem(video(id)));
}

function state(ids: number[], total: number, cursor?: string): VideosData {
  return {
    items: items(ids),
    total,
    cursor,
    hasMore: cursor !== undefined,
    inconsistent: false,
    missingTagIds: [],
  };
}

function ids(data: VideosData): number[] {
  return data.items.map((item) => (item.kind === "video" ? item.video.id : -1));
}

describe("videosDataReducer", () => {
  it("続きのページは既に出ている項目を捨てて足す", () => {
    const next = videosDataReducer(state([1, 2], 10, "c1"), {
      type: "page",
      page: { items: items([2, 3]), total: 10, nextCursor: "c2" },
      replace: false,
    });
    expect(ids(next)).toEqual([1, 2, 3]);
    expect(next.cursor).toBe("c2");
    expect(next.hasMore).toBe(true);
  });

  it("結合した件数が total を超えたら古い状態を保って不整合にする", () => {
    const before = state([1, 2], 3, "c1");
    const next = videosDataReducer(before, {
      type: "page",
      page: { items: items([3, 4]), total: 3 },
      replace: false,
    });
    expect(next.items).toBe(before.items);
    expect(next.hasMore).toBe(false);
    expect(next.inconsistent).toBe(true);
  });

  it("当たる動画が無い変更は同じ状態を返す", () => {
    const before = state([1], 1);
    expect(
      videosDataReducer(before, {
        type: "visibility",
        videoIds: [2],
        isPublic: true,
      }),
    ).toBe(before);
    expect(videosDataReducer(before, { type: "remove", videoId: 2 })).toBe(before);
  });

  it("動画を外すと total を1減らす", () => {
    const next = videosDataReducer(state([1, 2], 5), { type: "remove", videoId: 1 });
    expect(ids(next)).toEqual([2]);
    expect(next.total).toBe(4);
  });

  it("取り直した1件は一覧側の題名と大きさを残す", () => {
    const refreshed = { ...video(1), title: "別の所在", sizeBytes: 9, public: true };
    const next = videosDataReducer(state([1], 1), {
      type: "refresh",
      videoId: 1,
      video: refreshed,
    });
    expect(next.items).toEqual([
      videoItem({ ...refreshed, title: "動画 1", sizeBytes: 1024 }),
    ]);
  });

  it("代表でなくなった動画は、代表が一覧に出ていれば外して total を1減らす", () => {
    const next = videosDataReducer(state([1, 2, 3], 10), {
      type: "replaceWithRepresentative",
      videoId: 2,
      representative: video(1),
    });
    expect(ids(next)).toEqual([1, 3]);
    expect(next.total).toBe(9);
  });

  it("代表でなくなった動画は、代表が一覧に無ければ同じ位置で代表に置き換える", () => {
    const representative = { ...video(7), title: "代表の題名" };
    const next = videosDataReducer(state([1, 2, 3], 10), {
      type: "replaceWithRepresentative",
      videoId: 2,
      representative,
    });
    expect(ids(next)).toEqual([1, 7, 3]);
    expect(next.items[1]).toEqual(videoItem(representative));
    expect(next.total).toBe(10);
  });

  it("一覧に無い動画の置き換えは同じ状態を返す", () => {
    const before = state([1], 1);
    expect(
      videosDataReducer(before, {
        type: "replaceWithRepresentative",
        videoId: 2,
        representative: video(3),
      }),
    ).toBe(before);
  });
});

import { describe, expect, it } from "vitest";

import type { Playlist, PlaylistItem, PlaylistPlayback } from "../api/playlists";
import {
  choosePair,
  itemStartVideoId,
  pairAgrees,
  panelScrollTop,
  parsePlaylistQuery,
  stepPath,
} from "./playlistPlayback";

function pair(playbackAt: string, playlistAt: string, tag: string) {
  const summary = { id: 5, name: tag, itemCount: 0, updatedAt: playbackAt };
  return {
    playback: { playlist: summary, itemId: 1, position: 1, count: 1 } as PlaylistPlayback,
    playlist: { ...summary, updatedAt: playlistAt, items: [] } as Playlist,
  };
}

describe("parsePlaylistQuery", () => {
  it("playlist と item が正の整数のときだけ文脈を返す", () => {
    expect(parsePlaylistQuery("?playlist=5&item=22")).toEqual({
      playlistId: 5,
      itemId: 22,
    });
    expect(parsePlaylistQuery("")).toBeNull();
    expect(parsePlaylistQuery("?playlist=5")).toBeNull();
    expect(parsePlaylistQuery("?item=22")).toBeNull();
    expect(parsePlaylistQuery("?playlist=0&item=22")).toBeNull();
    expect(parsePlaylistQuery("?playlist=-1&item=22")).toBeNull();
    expect(parsePlaylistQuery("?playlist=1.5&item=22")).toBeNull();
    expect(parsePlaylistQuery("?playlist=abc&item=22")).toBeNull();
  });
});

describe("stepPath", () => {
  it("playlist を保ち、item をステップの項目にした動画ページの場所を返す", () => {
    expect(stepPath(5, 13, 23)).toBe("/videos/13?playlist=5&item=23");
  });
});

describe("itemStartVideoId", () => {
  it("動画はその動画、グループは openVideoId から始める", () => {
    expect(
      itemStartVideoId({ id: 1, kind: "video", video: { id: 7 } } as PlaylistItem),
    ).toBe(7);
    expect(
      itemStartVideoId({
        id: 2,
        kind: "group",
        group: { openVideoId: 9 },
      } as PlaylistItem),
    ).toBe(9);
    expect(itemStartVideoId({ id: 3, kind: "video" } as PlaylistItem)).toBeUndefined();
  });
});

describe("One order", () => {
  const early = "2026-10-01T00:00:00Z";
  const late = "2026-10-01T00:00:05Z";

  it("updatedAt が同じ時刻のときだけ組がそろう（書き方の違いは同じ時刻）", () => {
    expect(pairAgrees(pair(early, early, "a"))).toBe(true);
    expect(pairAgrees(pair(early, "2026-10-01T09:00:00+09:00", "a"))).toBe(true);
    expect(pairAgrees(pair(early, late, "a"))).toBe(false);
  });

  it("そろった組があればそれを、無ければ updatedAt の遅い組を選ぶ", () => {
    expect(choosePair([])).toBeNull();
    expect(
      choosePair([pair(early, late, "a"), pair(late, late, "b")])?.playlist.name,
    ).toBe("b");
    const latest = choosePair([
      pair(early, late, "a"),
      pair("2026-10-01T00:00:09Z", late, "b"),
      pair(late, early, "c"),
    ]);
    expect(latest?.playlist.name).toBe("b");
  });
});

describe("panelScrollTop", () => {
  it("上の行が無ければ 0、あれば上の行の下 3 分の 1 を残す位置", () => {
    expect(panelScrollTop(null)).toBe(0);
    expect(panelScrollTop({ top: 6, height: 90 })).toBe(66);
    expect(panelScrollTop({ top: 210, height: 90 })).toBe(270);
  });
});

import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import EndedOverlay from "./EndedOverlay";
import { endLayer, type EndLayerArgs, thumbnailCapture } from "./playerLayers";
import type { VideoPlaylist } from "./useVideoPlaylist";
import { initialPlayerStatus } from "./VideoPlayer";

const ended = { ...initialPlayerStatus, ended: true, positioned: true };

function args(playlist: Partial<VideoPlaylist>): EndLayerArgs {
  return {
    status: ended,
    pageKey: "12:22",
    videoId: 12,
    playlist: {
      reading: false,
      inPlaylist: true,
      dropped: false,
      next: undefined,
      playback: undefined,
      ...playlist,
    } as VideoPlaylist,
    group: undefined,
    nextVideo: undefined,
    autoplayPhase: "notice",
    linkState: { from: "/", trail: [] },
    takeFocus: false,
    owner: true,
    controls: null,
    onCancel: vi.fn(),
    onPlayNext: vi.fn(),
    onGroupGone: vi.fn(),
    onPlaylistGone: vi.fn(),
    videoTo: (id) => `/videos/${String(id)}`,
    nextTo: () => undefined,
  };
}

describe("endLayer", () => {
  it("読めたプレイリストに次も再生の応答も無ければ、次の無い再生終了の層を出す", () => {
    const { layer, noticeShown } = endLayer(
      args({ context: { status: "ready" } as VideoPlaylist["context"] }),
    );
    const element = layer as ReactElement<{ next: unknown }>;
    expect(element.type).toBe(EndedOverlay);
    expect(element.props.next).toBeUndefined();
    expect(noticeShown).toBe(false);
  });
});

describe("thumbnailCapture", () => {
  it("プレイヤーの操作が無い間は位置を null にする", () => {
    const capture = thumbnailCapture({
      shown: true,
      controls: null,
      status: ended,
      statusShown: false,
    });
    expect(capture?.enabled).toBe(false);
    expect(capture?.positionMs()).toBeNull();
  });
});

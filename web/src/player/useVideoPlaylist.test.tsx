import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import type { PlaylistItem, PlaylistPlayback } from "../api/playlists";
import type { PlaylistContext } from "../playlists/usePlaylistContext";
import {
  panelCurrentItemId,
  useUnplayableSkip,
  useVideoPlaylist,
  type VideoPlaylist,
} from "./useVideoPlaylist";

const navigate = vi.fn();
const toast = vi.fn();
const context = vi.hoisted(() => ({ value: undefined as PlaylistContext | undefined }));

vi.mock("react-router", () => ({ useNavigate: () => navigate }));
vi.mock("../ui/Toast", () => ({ useToast: () => toast }));
vi.mock("../playlists/usePlaylistContext", () => ({
  usePlaylistContext: () => context.value,
}));

const video = { id: 13, title: "third" } as Video;
const linkState = { from: "/playlists/5", trail: [] };

const lastPlayback: PlaylistPlayback = {
  playlist: { id: 5, name: "Tonight", itemCount: 3, updatedAt: "2026-10-01T00:00:00Z" },
  itemId: 23,
  position: 3,
  count: 3,
  playableCount: 3,
};

function readyContext(playback: PlaylistPlayback | undefined): PlaylistContext {
  return {
    status: "ready",
    playback,
    panel: {
      kind: "ready",
      playlist: {
        id: 5,
        name: "Tonight",
        itemCount: 1,
        updatedAt: "2026-10-01T00:00:00Z",
        items: [{ id: 23, kind: "video", video } as PlaylistItem],
      },
    },
    settledKey: 0,
    retry: () => {},
  } as PlaylistContext;
}

function renderPlaylist(playback: PlaylistPlayback | undefined) {
  context.value = readyContext(playback);
  return renderHook(() =>
    useVideoPlaylist({
      query: { playlistId: 5, itemId: 23 },
      videoId: 13,
      owner: true,
      readKey: 0,
      ended: false,
      keepPlaying: false,
      linkState,
      smartFolder: undefined,
      adoptedSmartFolder: undefined,
    }),
  ).result.current;
}

beforeEach(() => {
  navigate.mockReset();
  toast.mockReset();
});

describe("useVideoPlaylist", () => {
  it("次のステップが無ければ playNext は移らない", () => {
    const playlist = renderPlaylist(lastPlayback);
    playlist.playNext();
    expect(navigate).not.toHaveBeenCalled();
  });
});

describe("panelCurrentItemId", () => {
  it("再生の応答の項目を、無ければ URL の項目を、位置が 0 なら無しを返す", () => {
    expect(panelCurrentItemId(lastPlayback, 21)).toBe(23);
    expect(panelCurrentItemId(undefined, 21)).toBe(21);
    expect(panelCurrentItemId({ ...lastPlayback, position: 0 }, 21)).toBeUndefined();
  });
});

describe("useUnplayableSkip", () => {
  function renderSkip(playlist: VideoPlaylist, videoTitle: string | undefined) {
    return renderHook(
      ({ title }: { title: string | undefined }) =>
        useUnplayableSkip({
          playlist,
          autoplay: true,
          unplayable: true,
          videoTitle: title,
          pageKey: "13:23",
          linkState,
        }),
      { initialProps: { title: videoTitle } },
    );
  }

  it("最後の項目では進まず、項目の名前で終わりを伝える", () => {
    renderSkip(renderPlaylist(lastPlayback), undefined);
    expect(toast).toHaveBeenCalledExactlyOnceWith(
      'Can\'t play "third". End of the playlist.',
    );
    expect(navigate).not.toHaveBeenCalled();
  });

  it("同じ訪問の間は、値が変わって読み直しても二度は伝えない", () => {
    const { rerender } = renderSkip(renderPlaylist(lastPlayback), undefined);
    rerender({ title: "renamed" });
    expect(toast).toHaveBeenCalledOnce();
  });
});

import { useLocation, useNavigate } from "react-router";

import {
  groupContextOf,
  searchWithoutGroup,
  videoInGroupPath,
} from "../groups/groupPaths";
import {
  smartFolderContextOf,
  videoInSmartFolderPath,
} from "../smartFolders/smartFolderPaths";
import type { MemberTarget } from "./RelatedVideos";

export type VideoGroupContext = {
  /** Omitted when absent or overridden by playlist context. */
  group: number | undefined;
  /** Group wins over smartFolder; playlists still read folder context for member navigation. */
  smartFolder: number | undefined;
  videoTo: (videoId: number) => string;
  nextTo: (next: { id: number } | undefined) => string | undefined;
  /** Omitted for ordinary navigation or playlist-controlled member links. */
  memberTarget: (linkState: unknown) => MemberTarget | undefined;
  /** Drop group context and refresh after membership changes without stopping playback. */
  readWithoutGroup: (reread: () => void) => void;
};

function contextVideoPath(
  videoId: number,
  group: number | undefined,
  smartFolder: number | undefined,
): string {
  if (group !== undefined) return videoInGroupPath(videoId, group);
  if (smartFolder !== undefined) return videoInSmartFolderPath(videoId, smartFolder);
  return `/videos/${String(videoId)}`;
}

/** Playlist context overrides group navigation; smart-folder reads still apply while playlist steps own destinations. */
export function useVideoGroupContext(inPlaylist: boolean): VideoGroupContext {
  const location = useLocation();
  const navigate = useNavigate();
  const group = inPlaylist ? undefined : groupContextOf(location.search);
  const smartFolder =
    group === undefined ? smartFolderContextOf(location.search) : undefined;
  const videoTo = (videoId: number) =>
    contextVideoPath(videoId, group, inPlaylist ? undefined : smartFolder);
  const opened = !inPlaylist && (group !== undefined || smartFolder !== undefined);
  return {
    group,
    smartFolder,
    videoTo,
    nextTo: (next) => (next === undefined ? undefined : videoTo(next.id)),
    memberTarget: (linkState) =>
      opened ? (memberId) => ({ to: videoTo(memberId), state: linkState }) : undefined,
    readWithoutGroup: (reread) => {
      if (group === undefined) {
        reread();
        return;
      }
      void navigate(
        { pathname: location.pathname, search: searchWithoutGroup(location.search) },
        { replace: true, state: location.state as unknown },
      );
    },
  };
}

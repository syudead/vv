import { useEffect, useRef } from "react";
import { useNavigate } from "react-router";

import type { PlaylistPlayback, PlaylistStep } from "../api/playlists";
import { t } from "../i18n";
import { itemTitle } from "../playlists/playlistItems";
import {
  type PlaylistQuery,
  playlistStepPath,
  stepPath,
} from "../playlists/playlistPlayback";
import { usePlaylistContext } from "../playlists/usePlaylistContext";
import { useToast } from "../ui/Toast";
import type { Neighbor } from "./NeighborArrows";
import type { VideoLinkState } from "./pageDecisions";
import type { MemberTarget } from "./RelatedVideos";

/** Owner-only playlist state controls navigation and autoplay. After a 404, restore related arrows but stop at the current video's end. */
export function useVideoPlaylist({
  query,
  videoId,
  owner,
  readKey,
  ended,
  keepPlaying,
  linkState,
  smartFolder,
  adoptedSmartFolder,
}: {
  query: PlaylistQuery | null;
  videoId: number;
  owner: boolean;
  readKey: number;
  ended: boolean;
  keepPlaying: boolean;
  linkState: VideoLinkState;
  smartFolder: number | undefined;
  /** The server omits smartFolder when it no longer contains this video. */
  adoptedSmartFolder: number | undefined;
}) {
  const navigate = useNavigate();
  const context = usePlaylistContext({ query, videoId, owner, readKey });
  const playlistId = query?.playlistId ?? 0;
  const itemId = query?.itemId ?? 0;
  const playback = context.playback;
  const inPlaylist =
    context.status === "reading" ||
    context.status === "ready" ||
    context.status === "failed";
  // Read step folder IDs from playback data, not the panel response.
  // docs/design-docs/catalog-organization.md「Video page in a playlist」）。
  const goStep = (step: PlaylistStep, autoplay: boolean) =>
    void navigate(playlistStepPath(playlistId, step), {
      state: { ...linkState, autoplay },
    });
  const next = playback?.next;
  const stepNeighbor = (step: PlaylistStep | undefined): Neighbor | undefined =>
    step === undefined
      ? undefined
      : { title: step.video.title, go: () => goStep(step, keepPlaying) };
  const currentItem =
    context.panel.kind === "ready"
      ? context.panel.playlist.items.find((item) => item.id === itemId)
      : undefined;
  // Preserve playlist context within group/folder items; keep smartFolder only when the video response accepted it.
  const folderItem = currentItem?.kind === "smartFolder" || smartFolder !== undefined;
  const inGroupItem =
    playback !== undefined &&
    (folderItem
      ? smartFolder !== undefined && adoptedSmartFolder === smartFolder
      : currentItem?.kind === "group" || playback.next?.itemId === playback.itemId);
  const memberTarget: MemberTarget | undefined =
    context.status === "ready" && playback !== undefined && inGroupItem
      ? (memberId) => ({
          to: stepPath(playlistId, memberId, playback.itemId, smartFolder),
          state: { ...linkState, autoplay: keepPlaying },
        })
      : undefined;

  return {
    context,
    playlistId,
    itemId,
    playback,
    inPlaylist,
    dropped: context.status === "dropped",
    /** Suppress autoplay and ended overlays while awaiting the post-ended refresh. */
    reading: inPlaylist && ended && context.settledKey !== readKey,
    next,
    playNext: () => {
      if (next !== undefined) goStep(next, true);
    },
    previousArrow: stepNeighbor(playback?.previousItem),
    nextArrow: stepNeighbor(playback?.nextItem),
    currentItem,
    memberTarget,
  };
}

/** Use the URL item until playback loads; position zero means no current item. */
export function panelCurrentItemId(
  playback: PlaylistPlayback | undefined,
  itemId: number,
): number | undefined {
  if (playback === undefined) return itemId;
  return playback.position > 0 ? playback.itemId : undefined;
}

export type VideoPlaylist = ReturnType<typeof useVideoPlaylist>;

/** Unplayable items never end, so autoplay skips them once per page visit. Returning on a new visit permits another skip. */
export function useUnplayableSkip({
  playlist,
  autoplay,
  unplayable,
  videoTitle,
  pageKey,
  linkState,
}: {
  playlist: VideoPlaylist;
  autoplay: boolean;
  unplayable: boolean;
  videoTitle: string | undefined;
  pageKey: string;
  linkState: VideoLinkState;
}) {
  const navigate = useNavigate();
  const toast = useToast();
  const { context, playback, playlistId, currentItem } = playlist;
  const skippedTitle =
    videoTitle ?? (currentItem === undefined ? "" : itemTitle(currentItem));
  const skippedFor = useRef<string | null>(null);
  useEffect(() => {
    skippedFor.current = null;
  }, [pageKey]);
  useEffect(() => {
    if (context.status !== "ready" || playback === undefined) return;
    if (!autoplay || !unplayable) return;
    if (skippedFor.current === pageKey) return;
    skippedFor.current = pageKey;
    const next = playback.next;
    if (next === undefined) {
      toast(t.player.playlist.skippedLast(skippedTitle));
      return;
    }
    toast(t.player.playlist.skipped(skippedTitle));
    void navigate(playlistStepPath(playlistId, next), {
      replace: true,
      state: { ...linkState, autoplay: true },
    });
  }, [
    autoplay,
    context.status,
    linkState,
    navigate,
    pageKey,
    playback,
    playlistId,
    skippedTitle,
    toast,
    unplayable,
  ]);
}

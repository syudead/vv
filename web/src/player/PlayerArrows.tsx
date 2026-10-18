import { t } from "../i18n";
import NeighborArrows, { type Neighbor } from "./NeighborArrows";
import type { VideoPlaylist } from "./useVideoPlaylist";

/** Playlist navigation also overlays unplayable items so users can advance. Otherwise, use related-video neighbors only while the player is visible. */
export default function PlayerArrows({
  playlist,
  previous,
  next,
  showPlayer,
  chromeVisible,
  ended,
  container,
}: {
  playlist: VideoPlaylist;
  previous: Neighbor | undefined;
  next: Neighbor | undefined;
  showPlayer: boolean;
  chromeVisible: boolean;
  ended: boolean;
  container: HTMLElement | null;
}) {
  if (playlist.context.status === "ready")
    return (
      <NeighborArrows
        previous={playlist.previousArrow}
        next={playlist.nextArrow}
        visible={!showPlayer || chromeVisible || ended}
        container={container}
        labels={{
          previous: t.player.playlist.previous,
          next: t.player.playlist.next,
        }}
      />
    );
  if (!showPlayer || playlist.inPlaylist) return null;
  return (
    <NeighborArrows
      previous={previous}
      next={next}
      visible={chromeVisible || ended}
      container={container}
    />
  );
}

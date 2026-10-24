import type { RelatedState, VideoReadContext } from "../api/useVideoDetail";
import PlaylistPanel from "../playlists/PlaylistPanel";
import { Separator } from "../ui/shadcn/separator";
import type { VideoLinkState } from "./pageDecisions";
import RelatedVideos, { type MemberTarget } from "./RelatedVideos";
import { panelCurrentItemId, type VideoPlaylist } from "./useVideoPlaylist";

/** Playlist, continuous-play, and related sections share one scroll container. See docs/design-docs/catalog-organization.md. */
export default function VideoPageAside({
  related,
  linkState,
  onRetry,
  onLoadMore,
  readContext,
  groupMemberTarget,
  playlist,
  backTo,
  autoplay,
}: {
  related: RelatedState;
  linkState: VideoLinkState;
  onRetry: () => void;
  onLoadMore: () => void;
  readContext: VideoReadContext;
  /** Preserve the group or smart-folder context in member links. */
  groupMemberTarget: MemberTarget | undefined;
  playlist: VideoPlaylist;
  backTo: string;
  autoplay: boolean;
}) {
  if (!playlist.inPlaylist)
    return (
      <RelatedVideos
        state={related}
        linkState={linkState}
        onRetry={onRetry}
        onLoadMore={onLoadMore}
        memberTarget={groupMemberTarget}
        groupContext={readContext}
      />
    );
  const { playback } = playlist;
  const ready = related.kind === "ready" ? related.related : undefined;
  return (
    <RelatedVideos
      state={related}
      linkState={linkState}
      onRetry={onRetry}
      onLoadMore={onLoadMore}
      memberTarget={playlist.memberTarget}
      groupContext={readContext}
      leading={
        <>
          <PlaylistPanel
            playlistId={playlist.playlistId}
            panel={playlist.context.panel}
            name={playback?.playlist.name}
            position={playback?.position ?? 0}
            count={playback?.count ?? 0}
            nothingToPlay={playback?.playableCount === 0}
            currentItemId={panelCurrentItemId(playback, playlist.itemId)}
            backTo={backTo}
            autoplay={autoplay}
            onRetry={playlist.context.retry}
            // Continuous-play handles scrolling to the current member when present.
            scrollToCurrent={ready?.group === undefined}
          />
          {}
          {!(
            ready !== undefined &&
            ready.group === undefined &&
            ready.items.length === 0
          ) && (
            <div className="shrink-0 pt-5 pb-2">
              <Separator decorative={false} />
            </div>
          )}
        </>
      }
    />
  );
}

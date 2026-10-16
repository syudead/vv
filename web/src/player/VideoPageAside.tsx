import type { RelatedState, VideoReadContext } from "../api/useVideoDetail";
import PlaylistPanel from "../playlists/PlaylistPanel";
import { Separator } from "../ui/shadcn/separator";
import type { VideoLinkState } from "./pageDecisions";
import RelatedVideos, { type MemberTarget } from "./RelatedVideos";
import { panelCurrentItemId, type VideoPlaylist } from "./useVideoPlaylist";

/**
 * VideoPageAside は動画ページの右の列（関連動画）である。プレイリストの文脈では、プレイリストの
 * パネルを続けて再生・関連動画と同じ入れ物の先頭に置き、一緒にスクロールさせる（広い画面。
 * specs/044-playlists/ui-design.md「Video page in a playlist」）。
 */
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
  /** `?group` か `?smartFolder` で開いた動画のメンバーの行の開き方（その文脈を保って開く）。 */
  groupMemberTarget: MemberTarget | undefined;
  playlist: VideoPlaylist;
  backTo: string;
  /** パネルの行を押して移った先で再生を始めるか。 */
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
            currentItemId={panelCurrentItemId(playback, playlist.itemId)}
            backTo={backTo}
            autoplay={autoplay}
            onRetry={playlist.context.retry}
            // 続けて再生があるときは、そちらが今のメンバーへ動かす。
            scrollToCurrent={ready?.group === undefined}
          />
          {/* 後に何も続かなければ境目を描かない（ui-design.md「Playlist panel」Divider）。 */}
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

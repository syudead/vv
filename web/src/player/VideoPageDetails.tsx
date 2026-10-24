import type { Video } from "../api/client";
import type { useRelatedVideos, useVideoDetail } from "../api/useVideoDetail";
import { Skeleton } from "../ui/shadcn/skeleton";
import AutoTagButton from "./AutoTagButton";
import type { usePageHandoff } from "./detailsEditor/handoff";
import GroupLine from "./GroupLine";
import PlaylistLine from "./PlaylistLine";
import SmartFolderLine from "./SmartFolderLine";
import type { ThumbnailCapture } from "./playerLayers";
import type { SpanPlayer } from "./tabs/PeoplePanel";
import { CreatingLine } from "./StatusOverlays";
import type { VideoPlaylist } from "./useVideoPlaylist";
import TitleRow from "./TitleRow";
import VideoPageTabs, { useVideoTabState } from "./tabs/VideoPageTabs";
import VideoTags from "./VideoTags";
import VideoTitle from "./VideoTitle";
import type { VirtualThumbnail } from "./virtualVideo/useVirtualThumbnail";
import LinkFetchLine from "./virtualVideo/LinkFetchLine";
import VirtualLinks from "./virtualVideo/VirtualLinks";

type Detail = ReturnType<typeof useVideoDetail>;
type Related = ReturnType<typeof useRelatedVideos>;

export default function VideoPageDetails({
  loading,
  video,
  owner,
  playlist,
  backTo,
  trail,
  autoplay,
  capture,
  player,
  detail,
  related,
  handoff,
  thumbnail,
  onGroupChanged,
  onEditingThumbnail,
  onDeleted,
  onMerged,
}: {
  loading: boolean;
  video: Video | undefined;
  owner: boolean;
  playlist: VideoPlaylist;
  backTo: string;
  trail: number[];
  /** Whether a version row continues playback on its page. */
  autoplay: boolean;
  capture: ThumbnailCapture | undefined;
  player: SpanPlayer;
  detail: Pick<Detail, "refresh" | "replace" | "setFavorite">;
  related: Pick<Related, "rename" | "rethumb">;
  handoff: ReturnType<typeof usePageHandoff>;
  /** Upload state a virtual video's Thumbnail tab shares with its media frame. */
  thumbnail: VirtualThumbnail;
  onGroupChanged: () => void;
  onEditingThumbnail: (editing: boolean) => void;
  onDeleted: () => void;
  onMerged: (merged: Video) => void;
}) {
  const tabState = useVideoTabState(video, owner);
  if (loading)
    return (
      <div aria-hidden="true" className="flex flex-col gap-3">
        <Skeleton className="h-8 w-2/3" />
        <Skeleton className="h-5 w-1/2" />
      </div>
    );
  if (video === undefined) return null;
  const { refresh, replace, setFavorite } = detail;
  // Preserve the previous value while refreshing or after failure.
  const reread = () => void refresh();
  const { playback } = playlist;
  return (
    <>
      <CreatingLine video={video} />
      <div className="flex flex-col gap-2">
        {playlist.context.status === "ready" && playback !== undefined && (
          <PlaylistLine
            playlistId={playlist.playlistId}
            name={playback.playlist.name}
            position={playback.position}
            count={playback.count}
            backTo={backTo}
          />
        )}
        {video.smartFolder !== undefined && (
          <SmartFolderLine folder={video.smartFolder} backTo={backTo} />
        )}
        {video.group !== undefined && (
          <GroupLine
            group={video.group}
            videoId={video.id}
            owner={owner}
            backTo={backTo}
            // Fall back to the smallest remaining group ID, or ordinary video navigation.
            onChanged={onGroupChanged}
          />
        )}
        <TitleRow
          key={`title-row:${String(video.id)}`}
          video={video}
          onFavorite={owner ? setFavorite : undefined}
          title={<VideoTitle video={video} owner={owner} />}
        />
        {owner && <OwnerTags video={video} reread={reread} />}
        {owner && <LinkFetchLine video={video} />}
      </div>
      {/* A virtual video has nothing to play, so its links and description take the main action's place. */}
      {video.virtual && <VirtualLinks videoId={video.id} info={video.workInfo} />}
      <VideoPageTabs
        video={video}
        owner={owner}
        capture={capture}
        player={player}
        onChanged={(saved, mark) => {
          replace(saved, mark, "keep");
          // Keep the group row's thumbnail and title in sync; metadata updates may rename the video.
          related.rethumb(saved);
          related.rename(saved);
        }}
        onStale={reread}
        onEditingChange={onEditingThumbnail}
        // Preserve navigation history and restore focus to the metadata editor's entry point.
        details={{
          linkState: { from: backTo, trail },
          focusEntry: handoff.focusEntry(video.id),
        }}
        state={tabState}
        versions={{
          navigation: { backTo, autoplay },
          // Refetch: the removed video's own work info replaces the bundle's.
          onReplace: (removed, mark) => replace(removed, mark, "refetch"),
        }}
        thumbnail={thumbnail}
        onDeleted={onDeleted}
        onMerged={onMerged}
      />
    </>
  );
}

function OwnerTags({ video, reread }: { video: Video; reread: () => void }) {
  return (
    <>
      <VideoTags
        // Key VideoTags by video ID because this route reuses VideoPage; pending tag changes must not leak.
        key={`tags:${String(video.id)}`}
        videoId={video.id}
        tags={video.tags}
        onStaleVideo={reread}
        onChanged={reread}
      />
      {}
      <AutoTagButton key={`auto-tag:${String(video.id)}`} videoId={video.id} />
    </>
  );
}

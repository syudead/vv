import type { Video } from "../api/client";
import type { useRelatedVideos, useVideoDetail } from "../api/useVideoDetail";
import { Skeleton } from "../ui/shadcn/skeleton";
import AutoTagButton from "./AutoTagButton";
import type { usePageHandoff } from "./detailsEditor/handoff";
import GroupLine from "./GroupLine";
import PlaylistLine from "./PlaylistLine";
import SmartFolderLine from "./SmartFolderLine";
import { CreatingLine } from "./StatusOverlays";
import type { VideoPlaylist } from "./useVideoPlaylist";
import VideoFacts, { type ThumbnailCapture } from "./VideoFacts";
import VideoTags from "./VideoTags";
import VideoTitle from "./VideoTitle";
import VisibilitySwitch from "./VisibilitySwitch";

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
  detail,
  related,
  handoff,
  onGroupChanged,
  onEditingThumbnail,
}: {
  loading: boolean;
  video: Video | undefined;
  owner: boolean;
  playlist: VideoPlaylist;
  backTo: string;
  trail: number[];
  autoplay: boolean;
  capture: ThumbnailCapture | undefined;
  detail: Pick<Detail, "refresh" | "replace" | "setFavorite">;
  related: Pick<Related, "rename" | "rethumb">;
  handoff: ReturnType<typeof usePageHandoff>;
  onGroupChanged: () => void;
  onEditingThumbnail: (editing: boolean) => void;
}) {
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
        <VideoTitle
          key={`title:${String(video.id)}`}
          video={video}
          owner={owner}
          onSaved={(saved, mark) => {
            replace(saved, mark, "keep");
            related.rename(saved);
          }}
          onStale={reread}
        />
        {owner && <OwnerControls video={video} reread={reread} />}
      </div>
      {/* Guest responses omit location, so file actions remain hidden. */}
      <VideoFacts
        key={`facts:${String(video.id)}`}
        video={video}
        owner={owner}
        capture={capture}
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
        //
        onFavorite={setFavorite}
        versions={{
          navigation: { backTo, autoplay },
          // Reload after unbundling to replace bundle metadata with the video's own metadata.
          onReplace: (removed, mark) => replace(removed, mark, "refetch"),
          onRefresh: reread,
        }}
      />
    </>
  );
}

function OwnerControls({ video, reread }: { video: Video; reread: () => void }) {
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
      {/* Key by video ID so errors do not survive navigation. */}
      <VisibilitySwitch
        key={`visibility:${String(video.id)}`}
        videoId={video.id}
        isPublic={video.public}
        onChanged={reread}
      />
    </>
  );
}

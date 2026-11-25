import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router";

import { listVideos, type Video } from "../api/client";
import { errorText, t, type UiText } from "../i18n";
import { libraryFilterPath } from "../library/libraryPath";
import { TAG_PARAM } from "../library/tagCriteria";
import { Shelf } from "../ui/patterns/shelf";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import { DEFAULT_SORT } from "../videoList/listCriteria";

const text = t.tags.page;
const shelfLimit = 6;

/** Up to six of the tag's videos in the library's default order, read again when its count changes. */
export function TagVideosShelf({
  tagId,
  videoCount,
}: {
  tagId: number;
  videoCount: number;
}) {
  const location = useLocation();
  const [videos, setVideos] = useState<readonly Video[] | undefined>(undefined);
  const [error, setError] = useState<UiText | null>(null);

  useEffect(() => {
    if (videoCount === 0) {
      setVideos([]);
      return;
    }
    const controller = new AbortController();
    listVideos({
      tag: [tagId],
      sort: DEFAULT_SORT,
      limit: shelfLimit,
      signal: controller.signal,
    }).then(
      (page) => {
        setVideos(page.items);
        setError(null);
      },
      (failure: unknown) => {
        if (!controller.signal.aborted) setError(errorText(failure));
      },
    );
    return () => controller.abort();
  }, [tagId, videoCount]);

  return (
    <Shelf
      title={text.videos}
      layout={videos !== undefined && videos.length > 0 ? "row" : "grid"}
      actions={
        <Button asChild variant="ghost" size="sm">
          <Link to={libraryFilterPath(TAG_PARAM, tagId)}>{text.openInLibrary}</Link>
        </Button>
      }
    >
      {error !== null ? (
        <p className="text-sm text-destructive">{text.videosFailed(error)}</p>
      ) : videos === undefined ? (
        <Skeleton className="aspect-video w-card-0" />
      ) : videos.length === 0 ? (
        <p className="text-sm text-muted-foreground">{text.notOnVideos}</p>
      ) : (
        videos.map((video) => (
          <Link
            key={video.id}
            to={`/videos/${String(video.id)}`}
            state={{ from: `${location.pathname}${location.search}` }}
            title={video.title}
            className="flex min-w-0 flex-col gap-2 rounded-md"
          >
            <VideoThumbnail>
              {video.thumbnailUrl !== undefined && (
                <VideoThumbnailImage src={video.thumbnailUrl} />
              )}
            </VideoThumbnail>
            <span className="truncate text-sm font-medium">{video.title}</span>
          </Link>
        ))
      )}
    </Shelf>
  );
}

import type { ReactNode } from "react";

import type { FolderRef, Video } from "../api/client";
import type { VideosState } from "../api/useVideos";
import { t } from "../i18n";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { resultCountText } from "../videoList/listSummary";
import type { PreviewCardProps } from "../videoList/usePreviewCoordination";
import type { VideoSelection } from "../videoList/useVideoSelection";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import { folderLocationLabel } from "./folderPath";
import FolderVideoGrid from "./FolderVideoGrid";
import { CardsLoading, LoadFailed, MoreRow, NoMatches, ResultCount } from "./states";

export default function FolderSearchResults({
  folder,
  videos,
  view,
  zoom,
  backTo,
  preview,
  selection,
  tagsRow,
}: {
  folder: FolderRef;
  videos: VideosState;
  view: ViewMode;
  zoom: Zoom;
  backTo: string;
  preview: PreviewCardProps;
  selection: VideoSelection;
  tagsRow: (video: Video) => ReactNode;
}) {
  const Card = view === "grid" ? VideoCard : VideoRow;
  const noMatch = !videos.loading && videos.error === null && videos.items.length === 0;
  const initialLoadError =
    !videos.loading && videos.items.length === 0 ? videos.error : null;
  return (
    <>
      <h2 className="sr-only">{t.folders.searchResults}</h2>
      {noMatch ? (
        <NoMatches />
      ) : (
        <>
          {initialLoadError === null && (
            <ResultCount>
              {videos.loading ? t.list.loading : resultCountText(videos.total)}
            </ResultCount>
          )}
          {initialLoadError !== null ? (
            <LoadFailed reason={initialLoadError} onRetry={videos.reload} />
          ) : videos.loading ? (
            <CardsLoading zoom={zoom} count={12} />
          ) : (
            <TagRowMeasureProvider>
              <FolderVideoGrid
                videos={videos}
                view={view}
                zoom={zoom}
                selectedKeys={selection.selectedKeys}
                onLift={preview.previewCoordinator.reset}
                onAccepted={selection.clear}
                card={(video) => (
                  <Card
                    key={video.id}
                    video={video}
                    backTo={backTo}
                    {...selection.cardProps(video.id)}
                    {...preview}
                    tagsRow={view === "grid" ? tagsRow : undefined}
                    location={
                      video.folder === undefined
                        ? undefined
                        : folderLocationLabel(folder, video.folder)
                    }
                  />
                )}
              />
            </TagRowMeasureProvider>
          )}
          <MoreRow
            loadingMore={videos.loadingMore}
            error={videos.items.length > 0 ? videos.error : null}
            onRetry={videos.retryLoadMore}
          />
        </>
      )}
    </>
  );
}

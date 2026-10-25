import type { ReactNode } from "react";

import type { FolderSummary, Video } from "../api/client";
import type { VideosState } from "../api/useVideos";
import { t } from "../i18n";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { CardGrid } from "../ui/patterns/card-grid";
import FolderVideoGrid from "./FolderVideoGrid";
import type { PreviewCardProps } from "../videoList/usePreviewCoordination";
import type { VideoSelection } from "../videoList/useVideoSelection";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import FolderCard from "./FolderCard";
import { Section } from "./layout";
import { CardsLoading, LoadFailed, MoreRow, NoMatches } from "./states";

export default function FolderContents({
  filtered,
  listingLoading,
  childFolders,
  videos,
  view,
  zoom,
  backTo,
  preview,
  selection,
  tagsRow,
  groupingMenu,
}: {
  /** Whether any search, filter or favorite condition applies. */
  filtered: boolean;
  listingLoading: boolean;
  childFolders: FolderSummary[];
  videos: VideosState;
  view: ViewMode;
  zoom: Zoom;
  /** Return URL for playback. */
  backTo: string;
  /** Coordinate previews so only one plays at a time. */
  preview: PreviewCardProps;
  selection: VideoSelection;
  tagsRow: (video: Video) => ReactNode;
  /** Owners only: grouping menu at the end of the video heading. */
  groupingMenu?: ReactNode;
}) {
  const Card = view === "grid" ? VideoCard : VideoRow;
  const showFolders = listingLoading || childFolders.length > 0;
  const filterOnly = filtered;
  const filterOnlyNoMatch =
    filterOnly && !videos.loading && videos.error === null && videos.items.length === 0;
  const showVideos =
    videos.loading ||
    videos.items.length > 0 ||
    videos.error !== null ||
    filterOnlyNoMatch;
  return (
    <>
      {showFolders && (
        <Section
          title={t.folders.subfolders}
          count={listingLoading ? undefined : childFolders.length}
        >
          {listingLoading ? (
            <CardsLoading zoom={zoom} count={3} />
          ) : (
            <CardGrid size={zoom}>
              {childFolders.map((child) => (
                <FolderCard key={child.path} folder={child} backTo={backTo} />
              ))}
            </CardGrid>
          )}
        </Section>
      )}
      {/*
       * Update one persistent polite live region so count changes are announced; heading text
       * alone is insufficient.
       */}
      {showVideos && (
        <p role="status" aria-live="polite" className="sr-only">
          {filterOnly && !videos.loading ? t.folders.directVideos(videos.total) : ""}
        </p>
      )}
      {showVideos &&
        (filterOnlyNoMatch ? (
          <NoMatches />
        ) : (
          <Section
            title={t.folders.videos}
            count={videos.loading ? undefined : videos.total}
            action={groupingMenu}
          >
            {videos.error !== null && videos.items.length === 0 ? (
              <LoadFailed reason={videos.error} onRetry={videos.reload} />
            ) : videos.loading ? (
              <CardsLoading zoom={zoom} count={6} />
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
          </Section>
        ))}
    </>
  );
}

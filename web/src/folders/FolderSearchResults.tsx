import type { ReactNode } from "react";

import type { FolderRef, Video } from "../api/client";
import { itemVideos } from "../api/libraryItems";
import type { VideosState } from "../api/useVideos";
import { t } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { CardGrid } from "../ui/patterns/card-grid";
import { resultCountText } from "../videoList/listSummary";
import type { PreviewCardProps } from "../videoList/usePreviewCoordination";
import VideoCard from "../videoList/VideoCard";
import { folderLocationLabel } from "./folderPath";
import { CardsLoading, LoadFailed, MoreRow, NoMatches, ResultCount } from "./states";

/**
 * FolderSearchResults はフォルダ1件の中で検索語があるときの検索結果である
 * （ui-design.md「Search results」）。子フォルダの一群と「動画 N」の見出しを
 * 出さず、ライブラリと同じ要約行と1つの格子にする。
 */
export default function FolderSearchResults({
  folder,
  videos,
  zoom,
  backTo,
  preview,
  tagsRow,
}: {
  folder: FolderRef;
  videos: VideosState;
  zoom: Zoom;
  /** 再生画面から戻る先（今の一覧の URL）。 */
  backTo: string;
  /** ホバープレビューを同時に 1 件に絞る調整（usePreviewCoordination）。 */
  preview: PreviewCardProps;
  /** カードの題名の下に出すタグの行（useFolderTagsRow）。 */
  tagsRow: (video: Video) => ReactNode;
}) {
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
              <CardGrid size={zoom}>
                {itemVideos(videos.items).map((video) => (
                  <VideoCard
                    key={video.id}
                    video={video}
                    backTo={backTo}
                    selected={false}
                    selectionMode={false}
                    {...preview}
                    tagsRow={tagsRow}
                    location={
                      video.folder === undefined
                        ? undefined
                        : folderLocationLabel(folder, video.folder)
                    }
                  />
                ))}
              </CardGrid>
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

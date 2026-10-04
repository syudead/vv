import type { ReactNode, Ref } from "react";

import type { FolderRef, Video } from "../api/client";
import { itemVideos } from "../api/libraryItems";
import type { VideosState } from "../api/useVideos";
import { t } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { resultCountText } from "../videoList/listSummary";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import type { PreviewCardProps } from "../videoList/usePreviewCoordination";
import VideoCard from "../videoList/VideoCard";
import VirtualGrid, {
  type ListAnchor,
  type VirtualGridHandle,
} from "../videoList/VirtualGrid";
import { folderLocationLabel } from "./folderPath";

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
  positionRef,
  initialAnchor,
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
  /** 格子の位置の口（控えの目印を読む）。 */
  positionRef?: Ref<VirtualGridHandle>;
  /** 控えから戻ったときに戻す位置。 */
  initialAnchor?: ListAnchor;
}) {
  const videoList = itemVideos(videos.items);
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
            <p
              role="status"
              aria-live="polite"
              className="text-center text-xs text-fg-muted tabular-nums"
            >
              {videos.loading ? t.list.loading : resultCountText(videos.total)}
            </p>
          )}
          {initialLoadError !== null ? (
            <LoadFailed reason={initialLoadError} onRetry={videos.reload} />
          ) : (
            <TagRowMeasureProvider>
              <VirtualGrid
                ref={positionRef}
                zoom={zoom}
                count={videos.loading ? 0 : videoList.length}
                itemKey={(index) => `v:${String(videoList[index]!.id)}`}
                renderItem={(index) => {
                  const video = videoList[index]!;
                  return (
                    <VideoCard
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
                  );
                }}
                placeholders={videos.loading ? 12 : videos.loadingMore ? 6 : 0}
                renderPlaceholder={() => <CardSkeleton count={1} />}
                initialAnchor={initialAnchor}
              />
            </TagRowMeasureProvider>
          )}
          {videos.error !== null && videos.items.length > 0 && (
            <LoadMoreFailed reason={videos.error} onRetry={videos.retryLoadMore} />
          )}
        </>
      )}
    </>
  );
}

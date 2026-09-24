import type { FolderRef } from "../api/client";
import type { VideosState } from "../api/useVideos";
import type { Zoom } from "../preferences/viewPreferences";
import { resultCountText } from "../videoList/listSummary";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import VideoCard from "../videoList/VideoCard";
import { folderLocationLabel } from "./folderPath";
import { Grid } from "./layout";

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
}: {
  folder: FolderRef;
  videos: VideosState;
  zoom: Zoom;
  /** 再生画面から戻る先（今の一覧の URL）。 */
  backTo: string;
}) {
  const noMatch = !videos.loading && videos.error === null && videos.items.length === 0;
  return (
    <>
      <h2 className="sr-only">検索結果</h2>
      {noMatch ? (
        <NoMatches />
      ) : (
        <>
          <p
            role="status"
            aria-live="polite"
            className="text-center text-xs text-fg-muted tabular-nums"
          >
            {videos.loading ? "読み込み中…" : resultCountText(videos.total)}
          </p>
          {videos.error !== null && videos.items.length === 0 ? (
            <LoadFailed reason={videos.error} onRetry={videos.reload} />
          ) : (
            <Grid zoom={zoom}>
              {videos.loading ? (
                <CardSkeleton count={12} />
              ) : (
                videos.items.map((video) => (
                  <VideoCard
                    key={video.id}
                    video={video}
                    backTo={backTo}
                    selected={false}
                    selectionMode={false}
                    location={
                      video.folder === undefined
                        ? undefined
                        : folderLocationLabel(folder, video.folder)
                    }
                  />
                ))
              )}
              {videos.loadingMore && <CardSkeleton count={6} />}
            </Grid>
          )}
          {videos.error !== null && videos.items.length > 0 && (
            <LoadMoreFailed reason={videos.error} onRetry={videos.retryLoadMore} />
          )}
        </>
      )}
    </>
  );
}

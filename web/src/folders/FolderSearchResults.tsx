import type { FolderRef } from "../api/client";
import type { VideosState } from "../api/useVideos";
import type { Zoom } from "../preferences/viewPreferences";
import type { ListCriteria } from "../videoList/listCriteria";
import { conditionLabels, summarize } from "../videoList/listSummary";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import VideoCard from "../videoList/VideoCard";
import { folderLocationLabel } from "./folderPath";
import { Grid, rangeLabel } from "./layout";

/**
 * FolderSearchResults はフォルダ1件の中で検索語があるときの検索結果である
 * （ui-design.md「Search results」）。子フォルダの一群と「動画 N」の見出しを
 * 出さず、ライブラリと同じ要約行と1つの格子にする。
 */
export default function FolderSearchResults({
  folder,
  name,
  criteria,
  videos,
  zoom,
  backTo,
  onClearNoMatches,
}: {
  folder: FolderRef;
  /** 一致なしのチップに添えるフォルダ名。まだ分からなければ undefined。 */
  name: string | undefined;
  criteria: ListCriteria;
  videos: VideosState;
  zoom: Zoom;
  /** 再生画面から戻る先（今の一覧の URL）。 */
  backTo: string;
  onClearNoMatches: () => void;
}) {
  const noMatch = !videos.loading && videos.error === null && videos.items.length === 0;
  return (
    <>
      <h2 className="sr-only">検索結果</h2>
      {noMatch ? (
        <NoMatches
          conditions={[
            ...conditionLabels(criteria),
            rangeLabel("subtree", name ?? "フォルダ"),
          ]}
          note={undefined}
          onClear={onClearNoMatches}
        />
      ) : (
        <>
          <p
            role="status"
            aria-live="polite"
            className="text-center text-xs text-fg-muted tabular-nums"
          >
            {videos.loading
              ? "読み込み中…"
              : `「${criteria.query}」 ${summarize(videos.items, videos.total)}`}
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

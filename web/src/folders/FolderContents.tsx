import type { FolderSummary } from "../api/client";
import type { VideosState } from "../api/useVideos";
import type { Zoom } from "../preferences/viewPreferences";
import { hasConditions, type ListCriteria } from "../videoList/listCriteria";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import VideoCard from "../videoList/VideoCard";
import FolderCard, { FolderCardSkeleton } from "./FolderCard";
import { Grid, Section } from "./layout";

/**
 * FolderContents はフォルダ1件の直下（子フォルダと動画）を並べる通常表示である。
 * 絞り込みだけのときも同じ形にする（ui-design.md「Filter only」）。
 */
export default function FolderContents({
  criteria,
  listingLoading,
  childFolders,
  videos,
  zoom,
  backTo,
}: {
  criteria: ListCriteria;
  /** 子フォルダの一覧を読んでいる間は true。 */
  listingLoading: boolean;
  /** 直下の子フォルダ。 */
  childFolders: FolderSummary[];
  videos: VideosState;
  zoom: Zoom;
  /** 再生画面から戻る先（今の一覧の URL）。 */
  backTo: string;
}) {
  const showFolders = listingLoading || childFolders.length > 0;
  const filterOnly = hasConditions(criteria);
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
          title="フォルダ"
          count={listingLoading ? undefined : childFolders.length}
        >
          <Grid zoom={zoom}>
            {listingLoading ? (
              <FolderCardSkeleton count={3} />
            ) : (
              childFolders.map((child) => (
                <FolderCard key={child.path} folder={child} showPath={false} />
              ))
            )}
          </Grid>
        </Section>
      )}
      {showVideos && (
        <div className={showFolders ? "mt-3" : undefined}>
          {/*
            件数の変化は、視覚的に隠した polite の状態の行で知らせる（子フォルダの
            上に「動画 N」の見出しは出しても、見出しの文字の変化だけでは読み上げ
            ソフトに伝わらないため）。分岐で作り直さず1つの要素の文字だけを
            変えることで、更新が確実に読み上げに乗る（絞り込みが無いときと
            読み込み中は空にする）。
          */}
          <p role="status" aria-live="polite" className="sr-only">
            {filterOnly && !videos.loading
              ? `直下の動画 ${videos.total.toLocaleString("ja-JP")} 件`
              : ""}
          </p>
          {filterOnlyNoMatch ? (
            <NoMatches />
          ) : (
            <Section title="動画" count={videos.loading ? undefined : videos.total}>
              {videos.error !== null && videos.items.length === 0 ? (
                <LoadFailed reason={videos.error} onRetry={videos.reload} />
              ) : (
                <Grid zoom={zoom}>
                  {videos.loading ? (
                    <CardSkeleton count={6} />
                  ) : (
                    videos.items.map((video) => (
                      <VideoCard
                        key={video.id}
                        video={video}
                        backTo={backTo}
                        selected={false}
                        selectionMode={false}
                      />
                    ))
                  )}
                  {videos.loadingMore && <CardSkeleton count={6} />}
                </Grid>
              )}
              {videos.error !== null && videos.items.length > 0 && (
                <LoadMoreFailed reason={videos.error} onRetry={videos.retryLoadMore} />
              )}
            </Section>
          )}
        </div>
      )}
    </>
  );
}

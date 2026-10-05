import type { ReactNode } from "react";

import type { FolderSummary, Video } from "../api/client";
import { itemVideos } from "../api/libraryItems";
import type { VideosState } from "../api/useVideos";
import { t } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { CardGrid } from "../ui/patterns/card-grid";
import { hasConditions, type ListCriteria } from "../videoList/listCriteria";
import type { PreviewCardProps } from "../videoList/usePreviewCoordination";
import VideoCard from "../videoList/VideoCard";
import FolderCard from "./FolderCard";
import { Section } from "./layout";
import { CardsLoading, LoadFailed, MoreRow, NoMatches } from "./states";

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
  preview,
  tagsRow,
  groupingMenu,
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
  /** ホバープレビューを同時に 1 件に絞る調整（usePreviewCoordination）。 */
  preview: PreviewCardProps;
  /** カードの題名の下に出すタグの行（useFolderTagsRow）。 */
  tagsRow: (video: Video) => ReactNode;
  /**
   * 「動画 N」の見出しの行の右端に置く、まとめ方のメニュー（ui-design.md
   * 「Folder grouping menu」）。ゲストでは渡さない。
   */
  groupingMenu?: ReactNode;
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
          title={t.folders.subfolders}
          count={listingLoading ? undefined : childFolders.length}
        >
          {listingLoading ? (
            <CardsLoading zoom={zoom} count={3} />
          ) : (
            <CardGrid size={zoom}>
              {childFolders.map((child) => (
                <FolderCard key={child.path} folder={child} showPath={false} />
              ))}
            </CardGrid>
          )}
        </Section>
      )}
      {/*
        件数の変化は、視覚的に隠した polite の状態の行で知らせる（子フォルダの
        上に「動画 N」の見出しは出しても、見出しの文字の変化だけでは読み上げ
        ソフトに伝わらないため）。分岐で作り直さず1つの要素の文字だけを
        変えることで、更新が確実に読み上げに乗る（絞り込みが無いときと
        読み込み中は空にする）。
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
            spaced={showFolders}
          >
            {videos.error !== null && videos.items.length === 0 ? (
              <LoadFailed reason={videos.error} onRetry={videos.reload} />
            ) : videos.loading ? (
              <CardsLoading zoom={zoom} count={6} />
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
          </Section>
        ))}
    </>
  );
}

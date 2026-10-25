import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

import {
  clearListSnapshot,
  saveListSnapshot,
  type takeListSnapshot,
} from "../api/listSnapshot";
import { itemVideos, videoKey } from "../api/libraryItems";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { t, type UiText } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { useScanControls } from "../shell/ScanProvider";
import { useWatchScanRefresh } from "../shell/useWatchScanRefresh";
import { criteriaKey, type ListCriteria } from "../videoList/listCriteria";
import { CardGrid } from "../ui/patterns/card-grid";
import { resultCountText } from "../videoList/listSummary";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useVideoSelection } from "../videoList/useVideoSelection";
import VideoCard from "../videoList/VideoCard";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { type RootDisplay, topLevelLocationLabel } from "./folderPath";
import { CardsLoading, LoadFailed, MoreRow, NoMatches, ResultCount } from "./states";
import { useFolderTagsRow } from "./useFolderTagsRow";

/** Must not collide with real folder keys, which use <id>\\0<path>. */
export const ROOT_SEARCH_KEY = "root-search";

export default function RootSearchResults({
  criteria,
  rootNames,
  roots,
  zoom,
  restored,
}: {
  criteria: ListCriteria;
  rootNames: Map<number, RootDisplay>;
  /** Wait for registered folders before showing location labels. */
  roots: { loading: boolean; error: UiText | null; reload: () => void };
  zoom: Zoom;
  restored: ReturnType<typeof takeListSnapshot>;
}) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    loadMore,
    retryLoadMore,
    reload,
    refreshInPlace,
    staleGroups,
  } = useVideos(criteria, restored);

  const { resetPreview, cardProps: preview } = usePreviewCoordination();
  const owner = useAudience() === "owner";
  const selection = useVideoSelection(items, owner, refreshInPlace);
  const { clear: clearSelection } = selection;
  const listKey = criteriaKey(criteria);
  useEffect(() => clearSelection(), [clearSelection, listKey]);
  const tagsRow = useFolderTagsRow();
  useEffect(() => {
    resetPreview();
  }, [items, resetPreview, zoom]);

  const scan = useScanControls();
  const { refresh: refreshScan } = scan;
  const knownScanId = useRef(restored?.scanId);
  useWatchScanRefresh(refreshInPlace);
  useEffect(() => refreshScan(), [refreshScan]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    clearListSnapshot();
    reload();
  }, [reload, scan.finished]);

  const saveSnapshot = useCallback(() => {
    saveListSnapshot(
      { ...criteria, folder: ROOT_SEARCH_KEY },
      {
        items,
        total,
        cursor,
        hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
        staleGroups: staleGroups(),
      },
    );
  }, [criteria, cursor, hasMore, items, staleGroups, total]);

  // Wait for roots before restoring scroll or loading more; otherwise a visible sentinel can fetch every page during failure.
  const rootsReady = !roots.loading && roots.error === null;
  const pendingScroll = useRef(restored?.scrollY);
  useEffect(() => {
    const previous = history.scrollRestoration;
    history.scrollRestoration = "manual";
    return () => {
      history.scrollRestoration = previous;
    };
  }, []);
  useLayoutEffect(() => {
    const top = pendingScroll.current;
    // Restore only after registered folders arrive and cards exist.
    if (top === undefined || !rootsReady || items.length === 0) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, [items.length, rootsReady]);

  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore || !rootsReady) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, loadMore, rootsReady]);

  // Location labels require root names; offer retry rather than indistinguishable same-name results.
  const waiting = loading || roots.loading;
  const failure = error ?? roots.error;
  // Retry only the failed request, or both if both failed.
  const retry = () => {
    if (error !== null) reload();
    if (roots.error !== null) roots.reload();
  };
  const noMatch = !waiting && failure === null && items.length === 0;
  const initialLoadFailed =
    !waiting && failure !== null && (items.length === 0 || roots.error !== null);
  const summaryText = waiting ? t.list.loading : resultCountText(total);

  // Use display: contents so the snapshot wrapper does not affect ListPage spacing.
  return (
    <div ref={selection.container} onClick={saveSnapshot} className="contents">
      {selection.marqueeBox}
      <h2 className="sr-only">{t.folders.searchResults}</h2>
      {noMatch ? (
        <NoMatches />
      ) : (
        <>
          {!initialLoadFailed && <ResultCount>{summaryText}</ResultCount>}
          {initialLoadFailed ? (
            <LoadFailed reason={failure} onRetry={retry} />
          ) : waiting ? (
            <CardsLoading zoom={zoom} count={12} />
          ) : (
            <TagRowMeasureProvider>
              <CardGrid size={zoom}>
                {itemVideos(items).map((video) => (
                  <div
                    key={video.id}
                    data-card-key={videoKey(video.id)}
                    className="grid min-w-0"
                  >
                    <VideoCard
                      video={video}
                      backTo={backTo}
                      {...selection.cardProps(video.id)}
                      {...preview}
                      tagsRow={tagsRow}
                      location={
                        video.folder === undefined
                          ? undefined
                          : topLevelLocationLabel(
                              video.folder,
                              rootNames.get(video.folder.rootId),
                            )
                      }
                    />
                  </div>
                ))}
              </CardGrid>
            </TagRowMeasureProvider>
          )}
          <MoreRow
            loadingMore={loadingMore}
            error={items.length > 0 ? error : null}
            onRetry={retryLoadMore}
          />
        </>
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
      {selection.bar}
    </div>
  );
}

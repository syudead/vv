import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { useLocation } from "react-router";

import {
  clearListSnapshot,
  saveListSnapshot,
  type takeListSnapshot,
} from "../api/listSnapshot";
import { useVideos } from "../api/useVideos";
import type { Zoom } from "../preferences/viewPreferences";
import { useScan } from "../shell/ScanProvider";
import type { ListCriteria } from "../videoList/listCriteria";
import { resultCountText } from "../videoList/listSummary";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import VideoCard from "../videoList/VideoCard";
import { type RootDisplay, topLevelLocationLabel } from "./folderPath";
import { Grid } from "./layout";

/**
 * ROOT_SEARCH_KEY は最上位の検索結果の控えの鍵である。実在するフォルダの鍵
 * （folderKey、`<id>\0<path>` の形）とは衝突しない文字列を選ぶ。
 */
export const ROOT_SEARCH_KEY = "root-search";

/**
 * RootSearchResults は最上位（`/folders`）で検索語があるときの検索結果である。
 * ライブラリ全体を `listVideos` で検索し、置き場所は登録フォルダの表示名から
 * 始める（ui-design.md「Search results」）。
 */
export default function RootSearchResults({
  criteria,
  rootNames,
  roots,
  zoom,
  restored,
}: {
  criteria: ListCriteria;
  rootNames: Map<number, RootDisplay>;
  /** 登録フォルダ一覧の取得の状態。置き場所の行はこれが揃ってから出す。 */
  roots: { loading: boolean; error: string | null; reload: () => void };
  zoom: Zoom;
  /** 呼び出し側（RootView）が取り出した控え。無ければ1ページ目から読む。 */
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
  } = useVideos(criteria, restored);

  const scan = useScan();
  const { refresh: refreshScan } = scan;
  const knownScanId = useRef(restored?.scanId);
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
      },
    );
  }, [criteria, cursor, hasMore, items, total]);

  // 置き場所を描けるのは登録フォルダ一覧が揃ったときだけ。それまでは位置の復元も
  // 続きの読み込みもしない（失敗中に観測点が見え続けて全件を読みに行かないように）。
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
    // 登録フォルダ一覧を待つ間や失敗中はカードが無いので、揃ってから戻す。
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

  // 置き場所は登録フォルダの表示名から始まるので、一覧が揃うまでは結果を確定させない。
  // 取得に失敗したら、同名の動画を見分けられないまま出さず、再試行を促す。
  const waiting = loading || roots.loading;
  const failure = error ?? roots.error;
  // 失敗した側だけを取り直す。両方が失敗していれば両方を取り直す。
  const retry = () => {
    if (error !== null) reload();
    if (roots.error !== null) roots.reload();
  };
  const noMatch = !waiting && failure === null && items.length === 0;
  const initialLoadFailed =
    !waiting && failure !== null && (items.length === 0 || roots.error !== null);
  const summaryText = waiting ? "読み込み中…" : resultCountText(total);

  return (
    <div onClick={saveSnapshot} className="flex flex-col gap-3">
      <h2 className="sr-only">検索結果</h2>
      {noMatch ? (
        <NoMatches />
      ) : (
        <>
          {!initialLoadFailed && (
            <p
              role="status"
              aria-live="polite"
              className="text-center text-xs text-fg-muted tabular-nums"
            >
              {summaryText}
            </p>
          )}
          {initialLoadFailed ? (
            <LoadFailed reason={failure} onRetry={retry} />
          ) : (
            <Grid zoom={zoom}>
              {waiting ? (
                <CardSkeleton count={12} />
              ) : (
                items.map((video) => (
                  <VideoCard
                    key={video.id}
                    video={video}
                    backTo={backTo}
                    selected={false}
                    selectionMode={false}
                    location={
                      video.folder === undefined
                        ? undefined
                        : topLevelLocationLabel(
                            video.folder,
                            rootNames.get(video.folder.rootId),
                          )
                    }
                  />
                ))
              )}
              {loadingMore && <CardSkeleton count={6} />}
            </Grid>
          )}
          {error !== null && items.length > 0 && (
            <LoadMoreFailed reason={error} onRetry={retryLoadMore} />
          )}
        </>
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </div>
  );
}

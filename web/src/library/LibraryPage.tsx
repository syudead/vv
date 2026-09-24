import {
  type CSSProperties,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useLocation } from "react-router";

import type { Video, VideoSort, WatchFilter } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useVideos } from "../api/useVideos";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import {
  clearConditions,
  hasConditions,
  type HistoryMode,
  type ListCriteria,
  newSeed,
} from "../videoList/listCriteria";
import { resultCountText } from "../videoList/listSummary";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import { useListCriteria } from "../videoList/useListCriteria";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import EmptyLibrary from "./EmptyLibrary";
import LibraryToolbar from "./LibraryToolbar";
import SelectionBar from "./SelectionBar";

const skeletonCount = 12;

const cardWidth: Record<Zoom, string> = {
  0: "var(--spacing-card-0)",
  1: "var(--spacing-card-1)",
  2: "var(--spacing-card-2)",
  3: "var(--spacing-card-3)",
};

/** topmostId は画面上端に最も近いカードの id を返す（大きさ切替で読んでいた位置を保つ）。 */
function topmostId(list: HTMLElement | null, top: number): number | undefined {
  if (list === null) return undefined;
  for (const child of Array.from(list.querySelectorAll<HTMLElement>("[data-video-id]"))) {
    if (child.getBoundingClientRect().bottom > top) {
      const id = Number(child.dataset.videoId);
      return Number.isNaN(id) ? undefined : id;
    }
  }
  return undefined;
}

export default function LibraryPage() {
  const location = useLocation();
  const [preferences, setPreferences] = useState(readViewPreferences);
  // 一覧の条件は URL が持つ（contracts/list-url.md）。端末に保存するのは sort だけ。
  const { criteria, apply } = useListCriteria(preferences.sort);
  const { query, watch, playable, sort } = criteria;
  const { zoom, view } = preferences;
  const searchField = useRef<HTMLInputElement | null>(null);
  const [activePreviewId, setActivePreviewId] = useState<number | null>(null);
  const [previewResetEpoch, setPreviewResetEpoch] = useState(0);
  const resetPreview = useCallback(() => {
    setActivePreviewId(null);
    setPreviewResetEpoch((epoch) => epoch + 1);
  }, []);
  const startPreview = useCallback((id: number) => setActivePreviewId(id), []);

  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  // --- 条件の変更（検索語の入力の続き以外は、それぞれ履歴を1つ増やす） ---
  const update = useCallback(
    (next: ListCriteria, mode: HistoryMode = "push") => {
      resetPreview();
      apply(next, mode);
    },
    [apply, resetPreview],
  );

  const changeSort = useCallback(
    (next: VideoSort) => {
      update({
        ...criteria,
        sort: next,
        seed: next === "random" ? newSeed(criteria.seed) : undefined,
      });
      savePreferences({ ...preferences, sort: next });
    },
    [criteria, preferences, savePreferences, update],
  );
  const shuffle = useCallback(
    () => update({ ...criteria, seed: newSeed(criteria.seed) }),
    [criteria, update],
  );
  const changeWatch = useCallback(
    (next: WatchFilter) => update({ ...criteria, watch: next }),
    [criteria, update],
  );
  const changePlayable = useCallback(
    (next: boolean) => update({ ...criteria, playable: next }),
    [criteria, update],
  );
  const commitQuery = useCallback(
    (next: string, mode: HistoryMode) => update({ ...criteria, query: next }, mode),
    [criteria, update],
  );
  const clearAll = useCallback(
    () => update(clearConditions(criteria)),
    [criteria, update],
  );
  // --- 大きさ切替で読んでいた位置を保つ ---
  const anchor = useRef<number | undefined>(undefined);
  const list = useRef<HTMLDivElement | null>(null);
  const topOffset = () =>
    parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue("--spacing-navbar"),
    ) || 48;

  const changeZoom = useCallback(
    (next: Zoom) => {
      anchor.current =
        window.scrollY > 0 ? topmostId(list.current, topOffset()) : undefined;
      resetPreview();
      savePreferences({ ...preferences, zoom: next });
    },
    [preferences, resetPreview, savePreferences],
  );

  useEffect(() => {
    const onResize = () => resetPreview();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [resetPreview]);

  useLayoutEffect(() => {
    const id = anchor.current;
    if (id === undefined) return;
    anchor.current = undefined;
    const target = list.current?.querySelector(`[data-video-id="${String(id)}"]`);
    if (!target) return;
    const top = window.scrollY + target.getBoundingClientRect().top - topOffset() - 8;
    window.scrollTo({ top: Math.max(top, 0), behavior: "auto" });
  }, [zoom]);

  // --- 一覧の取得（戻ってきたときはスナップショットから復元） ---
  const [restored] = useState(() => takeListSnapshot(criteria));
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

  // 絞り込みはサーバーが一覧の条件として適用する（Plan の Structural Decisions 7）。
  // 再生から戻って視聴状態が変わった項目も、その場では一覧から外さない。
  useEffect(() => {
    resetPreview();
  }, [items, playable, query, resetPreview, sort, view, watch, zoom]);

  // --- 選択 ---
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set());
  const changeSelection = useCallback((id: number, selected: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (selected) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);
  const clearSelection = useCallback(() => setSelectedIds(new Set()), []);
  const selectAll = useCallback(
    () => setSelectedIds(new Set(items.map((video) => video.id))),
    [items],
  );

  useEffect(() => {
    const visible = new Set(items.map((video) => video.id));
    setSelectedIds((current) => {
      const next = new Set([...current].filter((id) => visible.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [items]);

  useEffect(() => {
    if (selectedIds.size === 0) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") clearSelection();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [clearSelection, selectedIds.size]);

  // --- スクロール位置の復元 ---
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
    if (top === undefined || items.length === 0) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    // 初回の描画では TopBarPortal がツールバーを本文の流れに置き、直後にトップバーへ
    // 移す。その分だけ本文が縮み、スクロールの追従で位置がずれるので、描画の前に
    // もう一度合わせる（フォルダ画面と同じ）。
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, [items.length]);

  const listUrl = `${location.pathname}${location.search}`;

  // --- 取り込み完了で一覧を入れ替える ---
  const scan = useScan();
  const knownScanId = useRef(restored?.scanId);
  useEffect(() => scan.refresh(), [scan.refresh]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    clearListSnapshot();
    reload();
  }, [reload, scan.finished]);

  const saveSnapshot = useCallback(() => {
    saveListSnapshot(criteria, {
      items,
      total,
      cursor,
      hasMore,
      scrollY: window.scrollY,
      scanId: knownScanId.current,
    });
  }, [criteria, cursor, hasMore, items, total]);

  // --- 無限スクロール ---
  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          resetPreview();
          loadMore();
        }
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, loadMore, resetPreview]);

  const empty = !loading && error === null && items.length === 0;
  const conditioned = hasConditions(criteria);
  const selectionMode = selectedIds.size > 0;
  const resultStatus = loading ? "読み込み中…" : resultCountText(total);

  const rowProps = (video: Video) => ({
    video,
    backTo: listUrl,
    selected: selectedIds.has(video.id),
    selectionMode,
    onSelect: changeSelection,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
  });

  return (
    <div className="flex w-full flex-col gap-3 px-3 pt-3 pb-24 sm:px-4">
      <h1 className="sr-only">ライブラリ</h1>

      <TopBarPortal>
        <LibraryToolbar
          query={query}
          onQueryCommit={commitQuery}
          searchRef={searchField}
          sort={sort}
          onSortChange={changeSort}
          onShuffle={shuffle}
          watch={watch}
          onWatchChange={changeWatch}
          playable={playable}
          onPlayableChange={changePlayable}
          canClear={conditioned}
          onClear={clearAll}
          view={view}
          onViewChange={(next) => {
            resetPreview();
            savePreferences({ ...preferences, view: next });
          }}
          zoom={zoom}
          onZoomChange={changeZoom}
        />
      </TopBarPortal>

      <p
        role="status"
        aria-live="polite"
        className="text-center text-xs text-fg-muted tabular-nums"
      >
        {resultStatus}
      </p>

      {error !== null && items.length === 0 && (
        <LoadFailed reason={error} onRetry={reload} />
      )}

      {empty &&
        (conditioned ? (
          <NoMatches />
        ) : (
          <EmptyLibrary onScan={scan.start} scanning={scan.running} />
        ))}

      <div ref={list} onClick={saveSnapshot}>
        {view === "grid" ? (
          <div
            className="flex flex-wrap justify-center gap-2.5 [&>*]:w-[min(var(--card),100%)]"
            style={{ "--card": cardWidth[zoom] } as CSSProperties}
          >
            {loading ? (
              <CardSkeleton count={skeletonCount} />
            ) : (
              items.map((video) => <VideoCard key={video.id} {...rowProps(video)} />)
            )}
            {loadingMore && <CardSkeleton count={6} />}
          </div>
        ) : (
          !loading &&
          items.length > 0 && (
            <table className="w-full border-separate border-spacing-0 overflow-hidden rounded-lg bg-surface shadow-card">
              <thead>
                <tr className="text-left text-xs text-fg-muted">
                  <th className="w-10" />
                  <th className="w-32 py-2" />
                  <th className="py-2 pr-4 font-medium">題名</th>
                  <th className="hidden w-16 py-2 pr-4 sm:table-cell" />
                  <th className="w-20 py-2 pr-4 text-right font-medium">長さ</th>
                  <th className="hidden w-20 py-2 pr-4 text-right font-medium md:table-cell">
                    画質
                  </th>
                  <th className="hidden w-24 py-2 pr-4 text-right font-medium md:table-cell">
                    大きさ
                  </th>
                  <th className="hidden w-28 py-2 pr-3 text-right font-medium lg:table-cell">
                    追加
                  </th>
                </tr>
              </thead>
              <tbody className="[&>tr:nth-child(odd)]:bg-hover-wash/40">
                {items.map((video) => (
                  <VideoRow key={video.id} {...rowProps(video)} />
                ))}
              </tbody>
            </table>
          )
        )}
      </div>

      {error !== null && items.length > 0 && (
        <LoadMoreFailed reason={error} onRetry={retryLoadMore} />
      )}

      <div ref={sentinel} aria-hidden="true" className="h-px" />

      <SelectionBar
        count={selectedIds.size}
        total={items.length}
        onSelectAll={selectAll}
        onClear={clearSelection}
      />
    </div>
  );
}

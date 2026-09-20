import {
  type CSSProperties,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useSearchParams } from "react-router";

import { MAX_QUERY_LENGTH, type VideoSort } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useVideos } from "../api/useVideos";
import { watchState } from "../lib/format";
import {
  type Density,
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
} from "../preferences/viewPreferences";
import { useScan } from "../shell/ScanProvider";
import LibraryToolbar, { sortOptions, type WatchFilter } from "./LibraryToolbar";
import SelectionBar from "./SelectionBar";
import {
  EmptyLibrary,
  GridSkeleton,
  LoadFailed,
  NoFilterMatches,
  NoMatches,
} from "./states";
import VideoCard from "./VideoCard";

const skeletonCount = 12;

const tileWidth: Record<Density, string> = {
  dense: "var(--spacing-tile-sm)",
  standard: "var(--spacing-tile-md)",
  relaxed: "var(--spacing-tile-lg)",
};

/** toSort は URL の値を VideoSort に直す。想定外なら undefined。 */
function toSort(value: string | null): VideoSort | undefined {
  return sortOptions.find((option) => option.value === value)?.value;
}

/** topmostId は画面上端に最も近いカードの id を返す（密度切替で読んでいた位置を保つ）。 */
function topmostId(list: HTMLElement | null, top: number): number | undefined {
  if (list === null) return undefined;
  for (const child of Array.from(list.children)) {
    if (child.getBoundingClientRect().bottom > top) {
      const id = Number((child as HTMLElement).dataset.videoId);
      return Number.isNaN(id) ? undefined : id;
    }
  }
  return undefined;
}

export default function LibraryPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get("q") ?? "").trim().slice(0, MAX_QUERY_LENGTH);

  const [preferences, setPreferences] = useState(readViewPreferences);
  const sort = toSort(searchParams.get("sort")) ?? preferences.sort;
  const density = preferences.density;

  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  const changeSort = useCallback(
    (next: VideoSort) => {
      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          params.set("sort", next);
          return params;
        },
        { replace: true },
      );
      savePreferences({ ...preferences, sort: next });
    },
    [preferences, savePreferences, setSearchParams],
  );

  const clearQuery = useCallback(() => {
    setSearchParams(
      (current) => {
        const params = new URLSearchParams(current);
        params.delete("q");
        return params;
      },
      { replace: true },
    );
  }, [setSearchParams]);

  // --- 密度切替で読んでいた位置を保つ ---
  const anchor = useRef<number | undefined>(undefined);
  const grid = useRef<HTMLDivElement | null>(null);
  const topOffset = () =>
    parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue("--spacing-topbar"),
    ) || 56;

  const changeDensity = useCallback(
    (next: Density) => {
      anchor.current =
        window.scrollY > 0 ? topmostId(grid.current, topOffset()) : undefined;
      savePreferences({ ...preferences, density: next });
    },
    [preferences, savePreferences],
  );

  useLayoutEffect(() => {
    const id = anchor.current;
    if (id === undefined) return;
    anchor.current = undefined;
    const target = grid.current?.querySelector(`[data-video-id="${String(id)}"]`);
    if (!target) return;
    const top = window.scrollY + target.getBoundingClientRect().top - topOffset() - 16;
    window.scrollTo({ top: Math.max(top, 0), behavior: "auto" });
  }, [density]);

  // --- 一覧の取得（戻ってきたときはスナップショットから復元） ---
  const [restored] = useState(() => takeListSnapshot({ query, sort }));
  const { items, total, cursor, hasMore, loading, loadingMore, error, loadMore, reload } =
    useVideos(sort, query, restored);

  // --- 絞り込み（読み込み済みの範囲に対して手元で行う） ---
  const [watch, setWatch] = useState<WatchFilter>("all");
  const [playableOnly, setPlayableOnly] = useState(false);
  const filtered = useMemo(
    () =>
      items.filter(
        (video) =>
          (watch === "all" || watchState(video) === watch) &&
          (!playableOnly || video.playable),
      ),
    [items, playableOnly, watch],
  );
  const filtering = watch !== "all" || playableOnly;

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
    () => setSelectedIds(new Set(filtered.map((video) => video.id))),
    [filtered],
  );

  useEffect(() => {
    const visible = new Set(filtered.map((video) => video.id));
    setSelectedIds((current) => {
      const next = new Set([...current].filter((id) => visible.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [filtered]);

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
  }, [items.length]);

  const search = searchParams.toString();
  const listUrl = search === "" ? "/" : `/?${search}`;

  // --- 取り込み完了で一覧を入れ替える ---
  const scan = useScan();
  const knownScanId = useRef(restored?.scanId);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    clearListSnapshot();
    reload();
  }, [reload, scan.finished]);

  const saveSnapshot = useCallback(() => {
    saveListSnapshot(
      { query, sort },
      {
        items,
        total,
        cursor,
        hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
      },
    );
  }, [cursor, hasMore, items, query, sort, total]);

  // --- 無限スクロール ---
  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, loadMore]);

  const empty = !loading && error === null && items.length === 0;
  const shownCount = filtering ? filtered.length : total;
  const selectionMode = selectedIds.size > 0;

  return (
    <div className="mx-auto flex w-full max-w-[1920px] flex-col gap-5 px-4 pt-5 pb-24 sm:px-6 lg:px-8">
      <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="text-xl font-semibold tracking-tight text-fg">
            {query === "" ? "ライブラリ" : `「${query}」の検索結果`}
          </h1>
          <p
            role="status"
            aria-live="polite"
            className="text-sm text-fg-muted tabular-nums"
          >
            {loading
              ? "読み込み中…"
              : `${shownCount.toLocaleString("ja-JP")} 件${filtering ? "（絞り込み中）" : ""}`}
          </p>
        </div>
        <LibraryToolbar
          sort={sort}
          onSortChange={changeSort}
          watch={watch}
          onWatchChange={setWatch}
          playableOnly={playableOnly}
          onPlayableOnlyChange={setPlayableOnly}
          density={density}
          onDensityChange={changeDensity}
        />
      </header>

      {error !== null && items.length === 0 && (
        <LoadFailed reason={error} onRetry={reload} />
      )}

      {empty &&
        (query === "" ? (
          <EmptyLibrary onScan={scan.start} scanning={scan.running} />
        ) : (
          <NoMatches query={query} onClear={clearQuery} />
        ))}

      {!loading && !empty && filtered.length === 0 && (
        <NoFilterMatches
          onReset={() => {
            setWatch("all");
            setPlayableOnly(false);
          }}
        />
      )}

      <div
        ref={grid}
        onClick={saveSnapshot}
        className="grid gap-x-4 gap-y-6 grid-cols-[repeat(auto-fill,minmax(min(var(--tile),100%),1fr))]"
        style={{ "--tile": tileWidth[density] } as CSSProperties}
      >
        {loading ? (
          <GridSkeleton count={skeletonCount} />
        ) : (
          filtered.map((video) => (
            <VideoCard
              key={video.id}
              video={video}
              backTo={listUrl}
              selected={selectedIds.has(video.id)}
              selectionMode={selectionMode}
              onSelect={changeSelection}
            />
          ))
        )}
        {loadingMore && <GridSkeleton count={6} />}
      </div>

      {error !== null && items.length > 0 && (
        <p className="text-center text-sm text-danger">続きを取得できません: {error}</p>
      )}

      <div ref={sentinel} aria-hidden="true" className="h-px" />

      <SelectionBar
        count={selectedIds.size}
        total={filtered.length}
        onSelectAll={selectAll}
        onClear={clearSelection}
      />
    </div>
  );
}

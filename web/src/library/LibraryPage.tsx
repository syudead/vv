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

import { MAX_QUERY_LENGTH, type Video, type VideoSort } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useVideos } from "../api/useVideos";
import { formatBytes, formatDuration, watchState } from "../lib/format";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import Button from "../ui/Button";
import LibraryToolbar, { sortOptions, type WatchFilter } from "./LibraryToolbar";
import SelectionBar from "./SelectionBar";
import {
  CardSkeleton,
  EmptyLibrary,
  LoadFailed,
  NoFilterMatches,
  NoMatches,
} from "./states";
import VideoCard, { VideoRow } from "./VideoCard";

const skeletonCount = 12;

const cardWidth: Record<Zoom, string> = {
  0: "var(--spacing-card-0)",
  1: "var(--spacing-card-1)",
  2: "var(--spacing-card-2)",
  3: "var(--spacing-card-3)",
};

/** toSort は URL の値を VideoSort に直す。想定外なら undefined。 */
function toSort(value: string | null): VideoSort | undefined {
  return sortOptions.find((option) => option.value === value)?.value;
}

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

/** summarize は Stash の「1-8 of 8 (34m 11s - 263 MB)」に当たる一行。 */
function summarize(shown: Video[], total: number, filtering: boolean): string {
  const count = filtering ? shown.length : total;
  const durationMs = shown.reduce((sum, video) => sum + (video.durationMs ?? 0), 0);
  const bytes = shown.reduce((sum, video) => sum + video.sizeBytes, 0);
  const head =
    shown.length === 0
      ? "0 件"
      : shown.length >= count
        ? `${count.toLocaleString("ja-JP")} 件`
        : `1–${shown.length.toLocaleString("ja-JP")} / ${count.toLocaleString("ja-JP")} 件`;
  const detail = [durationMs > 0 ? formatDuration(durationMs) : "", formatBytes(bytes)]
    .filter((part) => part !== "")
    .join(" · ");
  return detail === "" ? head : `${head}（${detail}）`;
}

export default function LibraryPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get("q") ?? "").trim().slice(0, MAX_QUERY_LENGTH);

  const [preferences, setPreferences] = useState(readViewPreferences);
  const sort = toSort(searchParams.get("sort")) ?? preferences.sort;
  const { zoom, view } = preferences;

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
      savePreferences({ ...preferences, zoom: next });
    },
    [preferences, savePreferences],
  );

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
  const [restored] = useState(() => takeListSnapshot({ query, sort }));
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
  } = useVideos(sort, query, restored);

  // --- 絞り込み（有効な間は残ページも読み、ライブラリ全体を対象にする） ---
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

  useEffect(() => {
    if (filtering && hasMore && !loading && !loadingMore && error === null) {
      loadMore();
    }
  }, [error, filtering, hasMore, loadMore, loading, loadingMore]);

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
  const selectionMode = selectedIds.size > 0;
  const summary = loading ? "読み込み中…" : summarize(filtered, total, filtering);

  const rowProps = (video: Video) => ({
    video,
    backTo: listUrl,
    selected: selectedIds.has(video.id),
    selectionMode,
    onSelect: changeSelection,
  });

  return (
    <div className="flex w-full flex-col gap-3 px-3 pt-3 pb-24 sm:px-4">
      <h1 className="sr-only">ライブラリ</h1>

      <TopBarPortal>
        <LibraryToolbar
          sort={sort}
          onSortChange={changeSort}
          watch={watch}
          onWatchChange={setWatch}
          playableOnly={playableOnly}
          onPlayableOnlyChange={setPlayableOnly}
          view={view}
          onViewChange={(next) => savePreferences({ ...preferences, view: next })}
          zoom={zoom}
          onZoomChange={changeZoom}
        />
      </TopBarPortal>

      <p
        role="status"
        aria-live="polite"
        className="text-center text-xs text-fg-muted tabular-nums"
      >
        {query === "" ? summary : `「${query}」 ${summary}`}
      </p>

      {error !== null && items.length === 0 && (
        <LoadFailed reason={error} onRetry={reload} />
      )}

      {empty &&
        (query === "" ? (
          <EmptyLibrary onScan={scan.start} scanning={scan.running} />
        ) : (
          <NoMatches query={query} onClear={clearQuery} />
        ))}

      {!loading &&
        !loadingMore &&
        !hasMore &&
        error === null &&
        !empty &&
        filtered.length === 0 && (
          <NoFilterMatches
            onReset={() => {
              setWatch("all");
              setPlayableOnly(false);
            }}
          />
        )}

      <div ref={list} onClick={saveSnapshot}>
        {view === "grid" ? (
          <div
            className="flex flex-wrap justify-center gap-2.5 [&>*]:w-[min(var(--card),100%)]"
            style={{ "--card": cardWidth[zoom] } as CSSProperties}
          >
            {loading ? (
              <CardSkeleton count={skeletonCount} />
            ) : (
              filtered.map((video) => <VideoCard key={video.id} {...rowProps(video)} />)
            )}
            {loadingMore && <CardSkeleton count={6} />}
          </div>
        ) : (
          !loading &&
          filtered.length > 0 && (
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
                {filtered.map((video) => (
                  <VideoRow key={video.id} {...rowProps(video)} />
                ))}
              </tbody>
            </table>
          )
        )}
      </div>

      {error !== null && items.length > 0 && (
        <div className="flex items-center justify-center gap-2 text-sm text-danger">
          <p>続きを取得できません: {error}</p>
          <Button size="sm" onClick={retryLoadMore}>
            再試行
          </Button>
        </div>
      )}

      {!loading && filtered.length > 0 && (
        <p className="text-center text-xs text-fg-muted tabular-nums">{summary}</p>
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

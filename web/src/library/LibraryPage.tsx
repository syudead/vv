import {
  type CSSProperties,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useLocation } from "react-router";

import {
  listVideoIds,
  type TagRef,
  type Video,
  type VideoSort,
  type WatchFilter,
} from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { refreshTags } from "../api/tags";
import { useVideos } from "../api/useVideos";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScan } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import { useToast } from "../ui/Toast";
import {
  criteriaKey,
  type HistoryMode,
  type ListCriteria,
  newSeed,
} from "../videoList/listCriteria";
import { resultCountText } from "../videoList/listSummary";
import { CardSkeleton, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import { useListCriteria } from "../videoList/useListCriteria";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import ActiveTagFilters from "./ActiveTagFilters";
import CardTagRow from "./CardTagRow";
import EmptyLibrary from "./EmptyLibrary";
import LibraryToolbar from "./LibraryToolbar";
import SelectionBar from "./SelectionBar";
import { TagRowMeasureProvider } from "./TagRowMeasure";
import {
  addTagId,
  clearConditions as clearTagConditions,
  hasConditions as hasTagConditions,
  MAX_TAG_COUNT,
  parseTagParam,
  removeTagId,
  serializeTagIds,
  TAG_PARAM,
} from "./tagCriteria";

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
  const toast = useToast();
  const [preferences, setPreferences] = useState(readViewPreferences);
  // 一覧の条件は URL が持つ（contracts/list-url.md）。端末に保存するのは sort だけ。
  // タグ絞り込み（`tag`）はライブラリだけが持つ条件で、共有の ListCriteria には
  // 含めない（Plan の Structural Decisions 1・15）。useListCriteria の画面固有の
  // パラメータの口へ渡し、URL のすべての書き換え経路でその値を残す。
  const { criteria, apply } = useListCriteria(preferences.sort, TAG_PARAM);
  const { query, watch, playable, sort } = criteria;
  // URL が変わらない限り同じ配列を使い、タグの操作の関数とカードの memo を保つ。
  const rawTagParams = new URLSearchParams(location.search).getAll(TAG_PARAM);
  const rawTagKey = JSON.stringify(rawTagParams);
  // rawTagKey が rawTagParams の値を表す。
  const tagIds = useMemo(() => parseTagParam(rawTagParams), [rawTagKey]);
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
  const clearAll = useCallback(() => {
    resetPreview();
    const cleared = clearTagConditions(criteria);
    apply(cleared.criteria, "push", []);
  }, [apply, criteria, resetPreview]);

  // --- タグ絞り込み（list-url.md §2） ---
  // criteria は描画ごとに新しいオブジェクトになる。カードへ渡す pressTag は、
  // 最新の条件を ref から読んで参照を保ち、無関係な描画でカードを描き直させない。
  const latestConditions = useRef({ criteria, tagIds });
  latestConditions.current = { criteria, tagIds };
  const pressTag = useCallback(
    (tag: TagRef) => {
      const { criteria: current, tagIds: currentTagIds } = latestConditions.current;
      if (currentTagIds.includes(tag.id)) return; // すでに絞り込み中なら何も変わらない。
      if (currentTagIds.length >= MAX_TAG_COUNT) {
        toast("絞り込めるタグは 16 個までです");
        return;
      }
      resetPreview();
      apply(current, "push", serializeTagIds(addTagId(currentTagIds, tag.id)));
    },
    [apply, resetPreview, toast],
  );
  const removeActiveTag = useCallback(
    (id: number) => {
      resetPreview();
      apply(criteria, "push", serializeTagIds(removeTagId(tagIds, id)));
    },
    [apply, criteria, resetPreview, tagIds],
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
  const [restored] = useState(() => takeListSnapshot({ ...criteria, tags: tagIds }));
  const {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    missingTagIds,
    loadMore,
    retryLoadMore,
    reload,
  } = useVideos({ ...criteria, tag: tagIds }, restored);

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
  // タグの行の選択切り替え（選択中にカードのタグを押したとき）は、今の選択を
  // 読まずに関数形の更新で決める。依存を持たない安定した参照にし、CardTagRow へ
  // 渡す関数も再描画のたびに作り直さない（N4、memo(VideoCard) を効かせる）。
  const toggleSelection = useCallback((id: number) => {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);
  const clearSelection = useCallback(() => setSelectedIds(new Set()), []);

  // --- 「すべて選択」（Plan の Structural Decisions 4） ---
  // 読み込んでいないページを含む、今の条件の全件の id を選ぶ。選択は常に id の
  // 集合として持つ。
  const [selectingAll, setSelectingAll] = useState(false);
  const selectAll = useCallback(() => {
    setSelectingAll(true);
    listVideoIds({ query, watch, playable, tag: tagIds })
      .then((response) => {
        const missing = response.missingTagIds ?? [];
        if (missing.length > 0) {
          const remaining = tagIds.filter((id) => !missing.includes(id));
          toast("削除されたタグを絞り込みから外しました");
          refreshTags().catch(() => undefined);
          apply(criteria, "replace", serializeTagIds(remaining));
          return;
        }
        setSelectedIds(new Set(response.ids));
      })
      .catch(() => toast("すべてを選択できませんでした"))
      .finally(() => setSelectingAll(false));
  }, [apply, criteria, playable, query, tagIds, toast, watch]);

  useEffect(() => {
    const visible = new Set(items.map((video) => video.id));
    setSelectedIds((current) => {
      const next = new Set([...current].filter((id) => visible.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [items]);

  // 検索語・視聴状態・再生可否・タグのどれかを変えると選択を解除する
  // （ui-design.md「Active tag filters」）。条件の違う一覧で選んだ動画が、見えない
  // まま選択に残らないようにするため。同じ動画がたまたま両方の条件に一致しても
  // 解除する。
  const conditionsSignature = `${criteriaKey(criteria)}\u0000${tagIds.join(",")}`;
  const knownConditionsSignature = useRef(conditionsSignature);
  useEffect(() => {
    if (knownConditionsSignature.current === conditionsSignature) return;
    knownConditionsSignature.current = conditionsSignature;
    clearSelection();
  }, [clearSelection, conditionsSignature]);

  useEffect(() => {
    if (selectedIds.size === 0) return;
    const onKey = (event: KeyboardEvent) => {
      // 選択バーのタグ操作の Combobox・ポップオーバーが Esc を自分の操作として
      // 使ったとき（候補の一覧や吹き出しを閉じる）は、preventDefault 済みなので
      // ここでは見送り、選択を解除しない（ui-design.md「Combobox」・
      // Visual review criteria の操作の確認 手順2）。
      if (event.key === "Escape" && !event.defaultPrevented) clearSelection();
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
    saveListSnapshot(
      { ...criteria, tags: tagIds },
      {
        items,
        total,
        cursor,
        hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
      },
    );
  }, [criteria, cursor, hasMore, items, tagIds, total]);

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

  // --- もう無いタグ（list-url.md §1、ui-design.md「Stale tags in other screens」） ---
  // 一覧の応答の missingTagIds を受けたら、もう無いことを伝え、タグの一覧を取り直し、
  // 履歴を増やさずに URL から取り除く。URL を書き換えて新しい一覧を取りに行くまでの
  // 数フレームは、同じ missingTagIds を持つ再描画が起きうるので、内容が変わらない
  // 限り1回だけ処理する。
  const handledMissingTagIds = useRef<string | null>(null);
  useEffect(() => {
    if (missingTagIds.length === 0) {
      handledMissingTagIds.current = null;
      return;
    }
    const signature = [...missingTagIds].sort((a, b) => a - b).join(",");
    if (handledMissingTagIds.current === signature) return;
    handledMissingTagIds.current = signature;
    const remaining = tagIds.filter((id) => !missingTagIds.includes(id));
    if (remaining.length === tagIds.length) return;
    toast("削除されたタグを絞り込みから外しました");
    refreshTags().catch(() => undefined);
    apply(criteria, "replace", serializeTagIds(remaining));
  }, [apply, criteria, missingTagIds, tagIds, toast]);

  // 控えから一覧を戻したときは一覧の要求をしないので missingTagIds が届かない。
  // 画面が開くときに取り直す共有のタグの一覧と URL の tag を突き合わせ、無い id が
  // あれば同じく伝えて取り除く。マウント時に1回だけ行う。
  const mountRef = useRef({ criteria, tagIds, apply, toast });
  mountRef.current = { criteria, tagIds, apply, toast };
  useEffect(() => {
    if (restored === undefined || mountRef.current.tagIds.length === 0) return;
    let alive = true;
    refreshTags()
      .then((tags) => {
        if (!alive) return;
        const known = new Set(tags.map((tag) => tag.id));
        const {
          tagIds: current,
          criteria: currentCriteria,
          apply: currentApply,
        } = mountRef.current;
        const remaining = current.filter((id) => known.has(id));
        if (remaining.length === current.length) return;
        mountRef.current.toast("削除されたタグを絞り込みから外しました");
        currentApply(currentCriteria, "replace", serializeTagIds(remaining));
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [restored]);

  const selectedIdsArray = useMemo(() => Array.from(selectedIds), [selectedIds]);

  const empty = !loading && error === null && items.length === 0;
  const initialLoadFailed = !loading && error !== null && items.length === 0;
  const conditioned = hasTagConditions(criteria, tagIds);
  const selectionMode = selectedIds.size > 0;
  const resultStatus = loading ? "読み込み中…" : resultCountText(total);

  // renderTagsRow は VideoCard へ渡す安定した関数である（N4）。VideoCard は
  // memo で包まれており、props が前回と同じ参照であれば再描画しない。ここで
  // 毎描画ごとに新しい ReactNode を組み立てて渡すと、プレビューの開始・検索の
  // 入力など無関係な状態が変わるたびに全カードが作り直されてしまう。依存に
  // 挙げた値（selectionMode・pressTag・toggleSelection・view）が変わらない
  // 限り、この関数自体の参照は変わらない。
  const renderTagsRow = useCallback(
    (video: Video) =>
      view === "grid" ? (
        <CardTagRow
          tags={video.tags}
          selectionMode={selectionMode}
          onPress={pressTag}
          onToggleSelection={() => toggleSelection(video.id)}
        />
      ) : undefined,
    [pressTag, selectionMode, toggleSelection, view],
  );

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
    tagsRow: renderTagsRow,
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

      {tagIds.length > 0 && (
        <ActiveTagFilters
          tagIds={tagIds}
          onRemove={removeActiveTag}
          searchFieldRef={searchField}
        />
      )}

      {!initialLoadFailed && (
        <p
          role="status"
          aria-live="polite"
          className="text-center text-xs text-fg-muted tabular-nums"
        >
          {resultStatus}
        </p>
      )}

      {initialLoadFailed && <LoadFailed reason={error} onRetry={reload} />}

      {empty &&
        (conditioned ? (
          <NoMatches />
        ) : (
          <EmptyLibrary onScan={scan.start} scanning={scan.running} />
        ))}

      <div ref={list} onClick={saveSnapshot}>
        {view === "grid" ? (
          // タグの行の幅の見張りは一覧に1つだけ（B2、ui-design.md「Overflow」）。
          <TagRowMeasureProvider>
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
          </TagRowMeasureProvider>
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
        total={total}
        selectedIds={selectedIdsArray}
        selectingAll={selectingAll}
        onSelectAll={selectAll}
        onClear={clearSelection}
      />
    </div>
  );
}

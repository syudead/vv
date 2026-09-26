import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useLocation } from "react-router";

import {
  type LibraryGroup,
  listLibraryIds,
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
import { itemKey } from "../api/libraryItems";
import { refreshTags } from "../api/tags";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
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
import { Grid } from "../videoList/Grid";
import { resultCountText } from "../videoList/listSummary";
import {
  CardSkeleton,
  GuestEmpty,
  LoadFailed,
  LoadMoreFailed,
  NoMatches,
} from "../videoList/states";
import { useListCriteria } from "../videoList/useListCriteria";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import ActiveTagFilters from "./ActiveTagFilters";
import CardTagRow from "./CardTagRow";
import EmptyLibrary from "./EmptyLibrary";
import { GroupCard, GroupRow } from "./GroupCard";
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

export default function LibraryPage() {
  const location = useLocation();
  const toast = useToast();
  // ゲストには所有者のデータに依る操作（選択・タグの絞り込み・選択バー）を出さない
  // （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
  const owner = useAudience() === "owner";
  const [preferences, setPreferences] = useState(readViewPreferences);
  // 一覧の条件は URL が持つ（contracts/list-url.md）。端末に保存するのは sort だけ。
  // タグ絞り込み（`tag`）はライブラリだけが持つ条件で、共有の ListCriteria には
  // 含めない（Plan の Structural Decisions 1・15）。useListCriteria の画面固有の
  // パラメータの口へ渡し、URL のすべての書き換え経路でその値を残す。
  const { criteria, apply } = useListCriteria(preferences.sort, TAG_PARAM);
  const { query, watch, playable, sort } = criteria;
  // タグの値が変わらない限り同じ配列を使い、タグと無関係な URL の変更でも
  // タグの操作の関数とカードの memo を保つ。ゲストはタグで絞り込めない（URL に残った
  // `tag` は useListCriteria が取り除く）。
  const rawTagKey = JSON.stringify(
    new URLSearchParams(location.search).getAll(TAG_PARAM),
  );
  const tagIds = useMemo(
    () => (owner ? parseTagParam(JSON.parse(rawTagKey) as string[]) : []),
    [owner, rawTagKey],
  );
  const { zoom, view } = preferences;
  const searchField = useRef<HTMLInputElement | null>(null);
  const { activePreviewId, previewResetEpoch, startPreview, resetPreview } =
    usePreviewCoordination();

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
  const { listRef: list, capture: captureAnchor } = useZoomAnchor(zoom);

  const changeZoom = useCallback(
    (next: Zoom) => {
      captureAnchor();
      resetPreview();
      savePreferences({ ...preferences, zoom: next });
    },
    [captureAnchor, preferences, resetPreview, savePreferences],
  );

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
    staleGroups,
  } = useVideos({ ...criteria, tag: tagIds }, restored, "library");

  // 絞り込みはサーバーが一覧の条件として適用する（Plan の Structural Decisions 7）。
  // 再生から戻って視聴状態が変わった項目も、その場では一覧から外さない。
  useEffect(() => {
    resetPreview();
  }, [items, playable, query, resetPreview, sort, view, watch, zoom]);

  // --- 選択 ---
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set());
  // selectAllIds は直前の「すべて選択」（GET /api/library/ids）の応答の ids である。
  // 選択がこれと同じ集合の間だけ「すべて選択」を押せなくする（ui-design.md
  // 「Pressing and selection」。選んだ本数と total は数えるものが違うので比べない）。
  const [selectAllIds, setSelectAllIds] = useState<ReadonlySet<number> | null>(null);
  // 「すべて選択」の要求の間だけ true（下の selectAll が立てる）。
  const [selectingAll, setSelectingAll] = useState(false);
  // 「すべて選択」の進行中の要求を、手動の選択操作や条件の変化が起きたら
  // 無効にする通し番号（非ブロッキング指摘1）。あとから届く古い応答が、その
  // あとに起きたもっと新しい選択や条件を上書きしないようにする。
  const selectAllSeq = useRef(0);
  // 「すべて選択」の要求を無効にする（進行中なら selectingAll も戻す）。無効に
  // した後は、その要求の応答が届いても selectingAll を戻す側の分岐
  // （selectAll の finally）が selectAllSeq の不一致で素通りするので、ここで
  // 戻しておかないと「選択中…」のまま固まる（Devin の指摘1）。
  const invalidateSelectAll = useCallback(() => {
    selectAllSeq.current += 1;
    setSelectingAll(false);
  }, []);
  const changeSelection = useCallback(
    (id: number, selected: boolean) => {
      invalidateSelectAll();
      setSelectedIds((current) => {
        const next = new Set(current);
        if (selected) next.add(id);
        else next.delete(id);
        return next;
      });
    },
    [invalidateSelectAll],
  );
  // タグの行の選択切り替え（選択中にカードのタグを押したとき）は、今の選択を
  // 読まずに関数形の更新で決める。CardTagRow へ渡す関数も再描画のたびに作り
  // 直さないよう、依存は安定した invalidateSelectAll だけにする（N4、
  // memo(VideoCard) を効かせる）。
  const toggleSelection = useCallback(
    (id: number) => {
      invalidateSelectAll();
      setSelectedIds((current) => {
        const next = new Set(current);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      });
    },
    [invalidateSelectAll],
  );
  // グループのカードのチェックは、全メンバー（videoIds）を選択に入れる・外す
  // （specs/017-folder-groups/ui-design.md「Pressing and selection」）。
  const changeGroupSelection = useCallback(
    (ids: readonly number[], selected: boolean) => {
      invalidateSelectAll();
      setSelectedIds((current) => {
        const next = new Set(current);
        for (const id of ids) {
          if (selected) next.add(id);
          else next.delete(id);
        }
        return next;
      });
    },
    [invalidateSelectAll],
  );
  // 選択中にグループのカードのタグを押したときの切り替え。全メンバーが選択に
  // 入っていれば外し、そうでなければ入れる。
  const toggleGroupSelection = useCallback(
    (ids: readonly number[]) => {
      invalidateSelectAll();
      setSelectedIds((current) => {
        const next = new Set(current);
        const all = ids.length > 0 && ids.every((id) => current.has(id));
        for (const id of ids) {
          if (all) next.delete(id);
          else next.add(id);
        }
        return next;
      });
    },
    [invalidateSelectAll],
  );
  const clearSelection = useCallback(() => {
    invalidateSelectAll();
    setSelectedIds(new Set());
    // 選択が消えたら、前の「すべて選択」の応答はもう比べない。条件が変わった後に
    // 同じ id を手で選び直しても、読んでいない項目が残りうるので押せるままにする
    // （ui-design.md「Pressing and selection」）。
    setSelectAllIds(null);
  }, [invalidateSelectAll]);

  // --- 「すべて選択」（Plan の Structural Decisions 4） ---
  // 読み込んでいないページを含む、今の条件の全件の id を選ぶ。選択は常に id の
  // 集合として持つ。
  const selectAll = useCallback(() => {
    const seq = (selectAllSeq.current += 1);
    setSelectingAll(true);
    listLibraryIds({ query, watch, playable, tag: tagIds })
      .then((response) => {
        // 応答が届くまでの間に、手動の選択操作・別の「すべて選択」・条件の
        // 変化（下の conditionsSignature の効果も selectAllSeq を進める）が
        // 起きていたら、この応答はもう当てはまらないので捨てる（非ブロッキング
        // 指摘1）。
        if (selectAllSeq.current !== seq) return;
        const missing = response.missingTagIds ?? [];
        if (missing.length > 0) {
          // 取り除く id は、要求を送った時点の tagIds・criteria ではなく、
          // 応答が届いた時点の最新の値を使う（ref から読む。非ブロッキング
          // 指摘1）。
          const { criteria: latestCriteria, tagIds: latestTagIds } =
            latestConditions.current;
          const remaining = latestTagIds.filter((id) => !missing.includes(id));
          toast("削除されたタグを絞り込みから外しました");
          refreshTags().catch(() => undefined);
          apply(latestCriteria, "replace", serializeTagIds(remaining));
          return;
        }
        const selected = new Set(response.ids);
        setSelectedIds(selected);
        setSelectAllIds(selected);
      })
      .catch(() => {
        if (selectAllSeq.current !== seq) return;
        toast("すべてを選択できませんでした");
      })
      .finally(() => {
        if (selectAllSeq.current === seq) setSelectingAll(false);
      });
  }, [apply, playable, query, tagIds, toast, watch]);

  // 選択バーで一括して外したタグが、今の絞り込み（tagIds）に含まれているとき
  // の方針（Devin の指摘4、docs/design-docs/library-ui.md §6）。外した動画は
  // その絞り込みにもう合わなくなるかもしれないので、選択を解除し一覧を取り
  // 直して、件数と一覧を条件に合わせ直す。タグの絞り込み自体は外さない
  // （そのタグはまだ有効な絞り込み条件であり続ける）。
  const onTagRemoved = useCallback(
    (tagId: number) => {
      if (!latestConditions.current.tagIds.includes(tagId)) return;
      clearSelection();
      clearListSnapshot();
      reload();
    },
    [clearSelection, reload],
  );

  // 選択は常に id の集合として持ち、読み込み済みの items に合わせて刈り込まない
  // （Plan の Structural Decisions 4、issue 270 完了の条件2）。以前はここで
  // items に無い id を選択から落としていたが、items は タグの付け外し・
  // loadMore・進捗の反映のたびに新しい配列になるため、「すべて選択」で選んだ
  // まだ読み込んでいない id まで巻き込んで削ってしまっていた（B1）。条件
  // （検索語・視聴状態・再生可否・タグ）が変わったときの解除は、下の
  // conditionsSignature の効果が別に担う。

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
      { ...criteria, tags: tagIds },
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
  }, [criteria, cursor, hasMore, items, staleGroups, tagIds, total]);

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
  const allSelected = useMemo(
    () =>
      selectAllIds !== null &&
      selectAllIds.size === selectedIds.size &&
      Array.from(selectedIds).every((id) => selectAllIds.has(id)),
    [selectAllIds, selectedIds],
  );

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

  const renderGroupTagsRow = useCallback(
    (group: LibraryGroup) =>
      view === "grid" ? (
        <CardTagRow
          tags={group.tags}
          selectionMode={selectionMode}
          onPress={pressTag}
          onToggleSelection={() => toggleGroupSelection(group.videoIds)}
        />
      ) : undefined,
    [pressTag, selectionMode, toggleGroupSelection, view],
  );

  const groupProps = (group: LibraryGroup) => ({
    group,
    backTo: listUrl,
    selected:
      group.videoIds.length > 0 && group.videoIds.every((id) => selectedIds.has(id)),
    selectionMode,
    onSelect: owner ? changeGroupSelection : undefined,
    tagsRow: renderGroupTagsRow,
  });

  const rowProps = (video: Video) => ({
    video,
    backTo: listUrl,
    selected: selectedIds.has(video.id),
    selectionMode,
    onSelect: owner ? changeSelection : undefined,
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
        ) : owner ? (
          <EmptyLibrary onScan={scan.start} scanning={scan.running} />
        ) : (
          <GuestEmpty />
        ))}

      <div ref={list} onClick={saveSnapshot}>
        {view === "grid" ? (
          // タグの行の幅の見張りは一覧に1つだけ（B2、ui-design.md「Overflow」）。
          <TagRowMeasureProvider>
            <Grid zoom={zoom}>
              {loading ? (
                <CardSkeleton count={skeletonCount} />
              ) : (
                // グループはふつうの動画と同じ並びに1枚のカードで混ぜる。区画や
                // 見出しは設けない（ui-design.md「Screen boundary」、要件 15）。
                items.map((item) =>
                  item.kind === "video" ? (
                    <VideoCard key={itemKey(item)} {...rowProps(item.video)} />
                  ) : (
                    <GroupCard key={itemKey(item)} {...groupProps(item.group)} />
                  ),
                )
              )}
              {loadingMore && <CardSkeleton count={6} />}
            </Grid>
          </TagRowMeasureProvider>
        ) : (
          !loading &&
          items.length > 0 && (
            <table className="w-full border-separate border-spacing-0 overflow-hidden rounded-lg bg-surface shadow-card">
              <thead>
                <tr className="text-left text-xs text-fg-muted">
                  {owner && <th className="w-10" />}
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
                {items.map((item) =>
                  item.kind === "video" ? (
                    <VideoRow key={itemKey(item)} {...rowProps(item.video)} />
                  ) : (
                    <GroupRow key={itemKey(item)} {...groupProps(item.group)} />
                  ),
                )}
              </tbody>
            </table>
          )
        )}
      </div>

      {error !== null && items.length > 0 && (
        <LoadMoreFailed reason={error} onRetry={retryLoadMore} />
      )}

      <div ref={sentinel} aria-hidden="true" className="h-px" />

      {owner && (
        <SelectionBar
          count={selectedIds.size}
          allSelected={allSelected}
          selectedIds={selectedIdsArray}
          selectingAll={selectingAll}
          onSelectAll={selectAll}
          onClear={clearSelection}
          onTagRemoved={onTagRemoved}
        />
      )}
    </div>
  );
}

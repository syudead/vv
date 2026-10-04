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
import { groupRef, itemKey } from "../api/libraryItems";
import { refreshTags, revalidateTags } from "../api/tags";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { errorText, t } from "../i18n";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScanControls } from "../shell/ScanProvider";
import TopBarPortal from "../shell/TopBarPortal";
import { useToast } from "../ui/Toast";
import Skeleton from "../ui/Skeleton";
import {
  criteriaKey,
  type HistoryMode,
  type ListCriteria,
  newSeed,
} from "../videoList/listCriteria";
import { Grid } from "../videoList/Grid";
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
import {
  emptySelection,
  favoriteTargets,
  fromSelectAll,
  type LibrarySelection,
  reconcileGroups,
  sameSelection,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "./selection";
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
  // 含めない。useListCriteria の画面固有の
  // パラメータの口へ渡し、URL のすべての書き換え経路でその値を残す。
  const { criteria, apply } = useListCriteria(preferences.sort, TAG_PARAM);
  const { query, watch, playable, favorite, sort } = criteria;
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
  const changeFavorite = useCallback(
    (next: boolean) => update({ ...criteria, favorite: next }),
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
        toast(t.errors.reason.too_many_tag_filters({ limit: MAX_TAG_COUNT }));
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

  // 絞り込みはサーバーが一覧の条件として適用する。
  // 再生から戻って視聴状態が変わった項目も、その場では一覧から外さない。
  useEffect(() => {
    resetPreview();
  }, [favorite, items, playable, query, resetPreview, sort, view, watch, zoom]);

  // --- 選択 ---
  // 選択は動画の id の集合と、グループとして選んだグループ（フォルダとメンバー）を持つ
  // （specs/035-favorites/research.md R-7）。タグ・公開・束ねる操作は id の集合を送り、
  // 一括のお気に入りだけが選んだグループを `folders` として送る。
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const selectedIds = selection.ids;
  // selectAllSelection は直前の「すべて選択」（GET /api/library/ids）の応答の ids と groups
  // である。選択がこれと同じ（id の集合も選んだグループも）間だけ「すべて選択」を押せなく
  // する（ui-design.md「Pressing and selection」、035 ui-design.md「Selection bar」。選んだ
  // 本数と total は数えるものが違うので比べない）。
  const [selectAllSelection, setSelectAllSelection] = useState<LibrarySelection | null>(
    null,
  );
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
  // メンバーを 1 本でも外すと、そのグループはグループとしては選ばれていない扱いになる
  // （selection.ts の removeIds）。
  const changeSelection = useCallback(
    (id: number, selected: boolean) => {
      invalidateSelectAll();
      setSelection((current) => setVideo(current, id, selected));
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
      setSelection((current) => toggleVideo(current, id));
    },
    [invalidateSelectAll],
  );
  // グループのカードのチェックは、全メンバー（videoIds）を選択に入れる・外し、
  // グループとして覚える（specs/017-folder-groups/ui-design.md「Pressing and selection」、
  // 035 research.md R-7）。
  const changeGroupSelection = useCallback(
    (group: LibraryGroup, selected: boolean) => {
      invalidateSelectAll();
      setSelection((current) =>
        setGroup(
          current,
          { folder: groupRef(group), videoIds: group.videoIds },
          selected,
        ),
      );
    },
    [invalidateSelectAll],
  );
  // 選択中にグループのカードのタグを押したときの切り替え。全メンバーが選択に
  // 入っていれば外し、そうでなければグループとして選ぶ。
  const toggleGroupSelection = useCallback(
    (group: LibraryGroup) => {
      invalidateSelectAll();
      setSelection((current) =>
        toggleGroup(current, { folder: groupRef(group), videoIds: group.videoIds }),
      );
    },
    [invalidateSelectAll],
  );
  const clearSelection = useCallback(() => {
    invalidateSelectAll();
    setSelection(emptySelection);
    // 選択が消えたら、前の「すべて選択」の応答はもう比べない。条件が変わった後に
    // 同じ id を手で選び直しても、読んでいない項目が残りうるので押せるままにする
    // （ui-design.md「Pressing and selection」）。
    setSelectAllSelection(null);
  }, [invalidateSelectAll]);

  // --- 「すべて選択」 ---
  // 読み込んでいないページを含む、今の条件の全件の id を選ぶ。応答の `groups` は
  // 選んだグループにする（035 research.md R-7）。
  const selectAll = useCallback(() => {
    const seq = (selectAllSeq.current += 1);
    setSelectingAll(true);
    listLibraryIds({ query, watch, playable, favorite, tag: tagIds })
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
          toast(t.library.deletedTagsRemoved);
          refreshTags().catch(() => undefined);
          apply(latestCriteria, "replace", serializeTagIds(remaining));
          return;
        }
        const selected = fromSelectAll(response.ids, response.groups ?? []);
        setSelection(selected);
        setSelectAllSelection(selected);
      })
      .catch((failure: unknown) => {
        if (selectAllSeq.current !== seq) return;
        toast(t.library.selectAllFailed(errorText(failure)));
      })
      .finally(() => {
        if (selectAllSeq.current === seq) setSelectingAll(false);
      });
  }, [apply, favorite, playable, query, tagIds, toast, watch]);

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
  // 以前はここで
  // items に無い id を選択から落としていたが、items は タグの付け外し・
  // loadMore・進捗の反映のたびに新しい配列になるため、「すべて選択」で選んだ
  // まだ読み込んでいない id まで巻き込んで削ってしまっていた（B1）。条件
  // （検索語・視聴状態・再生可否・タグ）が変わったときの解除は、下の
  // conditionsSignature の効果が別に担う。

  // 選択を残したまま一覧を取り直したとき（取り込みの完了など）、選んだグループが
  // もうグループでなくなっていたりメンバーが変わっていたりすれば、読み込んだ項目に
  // 合わせて選んだグループだけを直す（selection.ts の reconcileGroups）。id は削らない。
  useEffect(() => {
    setSelection((current) => reconcileGroups(current, items));
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
      // ここでは見送り、選択を解除しない（ui-design.md「Combobox」）。
      if (event.key !== "Escape" || event.defaultPrevented) return;
      // 選択バーから開いた窓（「Bundle as versions」）の Esc は窓を閉じるだけにし、
      // 選択を残す（specs/030-video-versions/ui-design.md「Bundle dialog」）。窓が開いて
      // いるかはフォーカスの位置によらずに見る。送る間は押したボタンが disabled になって
      // フォーカスが body に落ちるので、event.target で見ると選択を解除して窓ごと消し、
      // 送り終えた後の一覧の取り直しとトーストまで失ってしまうため。
      if (document.querySelector('[aria-modal="true"]') !== null) return;
      clearSelection();
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

  // 選択バーの「Bundle as versions」で束ね終えたら、選択を解除して一覧を取り直す。代表以外の
  // バージョンが一覧から消える（specs/030-video-versions/ui-design.md「Bundle dialog」、
  // 受け入れ条件 6）。取り直しても格子の位置は保つ（下のスクロールの復元に今の位置を渡す）。
  // バーは消えるので、取り直した後のフォーカスは見えている最初の項目へ移す。
  // 取り直しを待つ間は "requested"、読み込みが始まったら "loading" にし、読み終えたら移す。
  const focusFirstItem = useRef<"idle" | "requested" | "loading">("idle");
  const onBundled = useCallback(() => {
    clearSelection();
    pendingScroll.current = window.scrollY;
    focusFirstItem.current = "requested";
    reload();
  }, [clearSelection, reload]);
  useEffect(() => {
    if (focusFirstItem.current === "idle") return;
    if (loading) {
      focusFirstItem.current = "loading";
      return;
    }
    if (focusFirstItem.current !== "loading") return;
    focusFirstItem.current = "idle";
    const links = Array.from(
      list.current?.querySelectorAll<HTMLElement>("a[href]") ?? [],
    );
    const target =
      links.find((link) => link.getBoundingClientRect().top >= 0) ?? links[0];
    // 位置を保ったまま移す（見えている項目なのでスクロールは要らない）。
    target?.focus({ preventScroll: true });
  }, [items, list, loading]);

  const listUrl = `${location.pathname}${location.search}`;

  // --- 取り込み完了で一覧を入れ替える ---
  const scan = useScanControls();
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
    toast(t.library.deletedTagsRemoved);
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
    revalidateTags()
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
        mountRef.current.toast(t.library.deletedTagsRemoved);
        currentApply(currentCriteria, "replace", serializeTagIds(remaining));
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [restored]);

  const selectedIdsArray = useMemo(() => Array.from(selectedIds), [selectedIds]);
  const allSelected = useMemo(
    () => selectAllSelection !== null && sameSelection(selectAllSelection, selection),
    [selectAllSelection, selection],
  );
  const favoriteSelection = useMemo(() => favoriteTargets(selection), [selection]);

  const empty = !loading && error === null && items.length === 0;
  const initialLoadFailed = !loading && error !== null && items.length === 0;
  const conditioned = hasTagConditions(criteria, tagIds);
  const selectionMode = selectedIds.size > 0;
  const resultStatus = loading ? t.list.loading : t.library.resultCount(total);

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
          onToggleSelection={() => toggleGroupSelection(group)}
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
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
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
    <div className="flex w-full flex-col gap-3 px-3 pt-3 pb-selection-bar-clearance sm:px-4">
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
          favorite={favorite}
          onFavoriteChange={changeFavorite}
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

      <div className="flex min-w-0 items-baseline justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-tight text-foreground">
          {t.library.title}
        </h1>
        {!initialLoadFailed && (
          <p
            role="status"
            aria-label={t.library.resultsLabel}
            aria-live="polite"
            className="shrink-0 text-xs text-muted-foreground tabular-nums sm:text-sm"
          >
            {resultStatus}
          </p>
        )}
      </div>

      {tagIds.length > 0 && (
        <ActiveTagFilters
          tagIds={tagIds}
          onRemove={removeActiveTag}
          searchFieldRef={searchField}
        />
      )}

      {initialLoadFailed && <LoadFailed reason={error} onRetry={reload} />}

      {empty &&
        (conditioned ? (
          <NoMatches onSearch={() => searchField.current?.focus()} />
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
        ) : loading ? (
          <div
            aria-hidden="true"
            className="space-y-2 rounded-md border border-border bg-card p-3"
          >
            {Array.from({ length: 6 }, (_, index) => (
              <div key={index} className="flex items-center gap-3 py-1">
                <Skeleton className="aspect-video w-list-thumb shrink-0" />
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-3/5" />
                  <Skeleton className="h-3 w-1/3" />
                </div>
              </div>
            ))}
          </div>
        ) : (
          items.length > 0 && (
            <table className="w-full border-separate border-spacing-0 overflow-hidden rounded-md border border-border bg-card">
              <thead>
                <tr className="text-left text-xs text-muted-foreground">
                  {owner && <th className="w-10" />}
                  <th className="w-list-thumb-cell py-2" />
                  <th className="py-2 pr-4 font-medium">{t.library.columns.title}</th>
                  {owner && <th className="w-8 py-2" />}
                  <th className="hidden w-16 py-2 pr-4 sm:table-cell" />
                  <th className="w-list-number py-2 pr-4 text-right font-medium">
                    {t.library.columns.duration}
                  </th>
                  <th className="hidden w-list-number py-2 pr-4 text-right font-medium md:table-cell">
                    {t.library.columns.quality}
                  </th>
                  <th className="hidden w-list-number-wide py-2 pr-4 text-right font-medium md:table-cell">
                    {t.library.columns.size}
                  </th>
                  <th className="hidden w-list-date py-2 pr-3 text-right font-medium lg:table-cell">
                    {t.library.columns.added}
                  </th>
                </tr>
              </thead>
              <tbody className="[&>tr:nth-child(odd)]:bg-accent/40">
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
          favoriteVideoIds={favoriteSelection.videoIds}
          favoriteFolders={favoriteSelection.folders}
          selectingAll={selectingAll}
          onSelectAll={selectAll}
          onClear={clearSelection}
          onTagRemoved={onTagRemoved}
          onBundled={onBundled}
        />
      )}
    </div>
  );
}

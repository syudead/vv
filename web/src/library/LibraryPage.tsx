import {
  type ReactNode,
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
  type LibraryItem,
  listLibraryIds,
  type SmartFolderSummary,
  type TagRef,
  type Video,
  type VideoSort,
  type VideoTag,
  type WatchFilter,
} from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import type { CreditKind } from "../api/credits";
import { itemKey, itemVideo } from "../api/libraryItems";
import { refreshTags, revalidateTags } from "../api/tags";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { GroupChangedContext } from "../groups/groupChanges";
import { errorText, t } from "../i18n";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScanControls } from "../shell/ScanProvider";
import { SmartFolderCardsProvider } from "../smartFolders/cardValues";
import { SmartFolderCard, SmartFolderRow } from "../smartFolders/SmartFolderCard";
import { useWatchScanRefresh } from "../shell/useWatchScanRefresh";
import TopBarPortal from "../shell/TopBarPortal";
import { DataTable } from "../ui/patterns/data-table";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { TableBody, TableHead, TableHeader, TableRow } from "../ui/shadcn/table";
import { useToast } from "../ui/Toast";
import {
  criteriaKey,
  type HistoryMode,
  type ListCriteria,
  newSeed,
} from "../videoList/listCriteria";
import { DraggableCard } from "../videoList/cardDrag";
import { Grid } from "../videoList/Grid";
import { useUndoToast } from "../videoList/undoToast";
import { useVideoDrops } from "../videoList/useVideoDrops";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";
import { GuestEmpty, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import { useListCriteria } from "../videoList/useListCriteria";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import SaveSmartFolderButton from "../smartFolders/SaveSmartFolderButton";
import ActiveTagFilters from "./ActiveTagFilters";
import {
  addedLengthCount,
  addedLengthParams,
  addedLengthSignature,
  addedLengthValues,
  clearedAddedLength,
  hasAddedLength,
  type LengthRange,
  useAddedLength,
  withAddedWithin,
  withLength,
} from "./addedLengthCriteria";
import { AddedSelect, LengthSelect } from "./AddedLengthSelects";
import {
  clearedCredits,
  creditSignature,
  hasFilterChips,
  ORGANIZATION_PARAM,
  PERSON_PARAM,
  TAG_KIND_PARAM,
  useCreditFilter,
  withoutCredit,
} from "./creditCriteria";
import CardTagRow from "./CardTagRow";
import EmptyLibrary from "./EmptyLibrary";
import { GroupCard, GroupRow } from "./GroupCard";
import {
  DraggableGroupRow,
  DraggableVideoRow,
  LibraryDragLayer,
  useLibraryMoves,
} from "./LibraryDrag";
import LibraryToolbar from "./LibraryToolbar";
import {
  emptySelection,
  favoriteTargets,
  fromSelectAll,
  type LibrarySelection,
  reconcileGroups,
  sameSelection,
  selectedGroupOf,
  isGroupSelected,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "./selection";
import SelectionBar from "./SelectionBar";
import TagKindSelect from "./TagKindSelect";
import { TagRowMeasureProvider } from "./TagRowMeasure";
import { useEscapeClearsSelection } from "./useEscapeClearsSelection";
import { type ApplyTagKind, useLibraryTagKinds } from "./useLibraryTagKinds";
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

/** libraryGroupItem は「Make a group」の新しいグループの項目である。 */
function libraryGroupItem(group: LibraryGroup): LibraryItem {
  return { kind: "group", group };
}

/** itemGroup は項目がグループならそのグループである。 */
function itemGroup(item: LibraryItem): LibraryGroup | undefined {
  return item.kind === "group" ? item.group : undefined;
}

/** ignoreToggle はスマートフォルダのカードのタグの行の、選択を切り替えない口である。 */
const ignoreToggle = () => undefined;

/**
 * withSmartFolderCards は一覧をスマートフォルダのカードの値の持ち主で包む。カードの値は
 * 画面に入ったカードだけを読む（specs/052-smart-folders/research.md R-14）。
 */
function withSmartFolderCards(content: ReactNode) {
  return <SmartFolderCardsProvider>{content}</SmartFolderCardsProvider>;
}

/** ライブラリだけが URL に持つ条件（タグ、人物、団体、タグの種類、追加日と長さ）。 */
const libraryParams = [
  TAG_PARAM,
  PERSON_PARAM,
  ORGANIZATION_PARAM,
  TAG_KIND_PARAM,
  ...addedLengthParams,
];

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
  // 人物・団体（`person`・`organization`）も同じ口で運ぶ（specs/047-video-work-info/
  // ui-design.md「Library: filter by a person or organization」）。
  // 追加日と長さはゲストも使える（specs/052-smart-folders/research.md R-4）。
  const { criteria, apply } = useListCriteria(
    preferences.sort,
    libraryParams,
    addedLengthParams,
  );
  const { query, watch, playable, favorite, sort } = criteria;
  // タグの値が変わらない限り同じ配列を使い、タグと無関係な URL の変更でも
  // タグの操作の関数とカードの memo を保つ。ゲストはタグ・人物・団体で絞り込めない
  // （URL に残った値は useListCriteria が取り除く）。
  const rawTagKey = JSON.stringify(
    new URLSearchParams(location.search).getAll(TAG_PARAM),
  );
  const tagIds = useMemo(
    () => (owner ? parseTagParam(JSON.parse(rawTagKey) as string[]) : []),
    [owner, rawTagKey],
  );
  const credits = useCreditFilter(location.search, owner);
  const { filter: addedLength, dropped: addedLengthDropped } = useAddedLength(
    location.search,
  );
  // 一覧の条件のうちライブラリだけのもの（人物・団体・タグの種類と、追加日と長さ）。
  const libraryFilter = useMemo(
    () => ({ ...credits, ...addedLength }),
    [credits, addedLength],
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
    apply(cleared.criteria, "push", {
      [TAG_PARAM]: serializeTagIds(cleared.tag),
      ...clearedCredits,
      ...clearedAddedLength,
    });
  }, [apply, criteria, resetPreview]);

  // --- タグ絞り込み（list-url.md §2） ---
  // criteria は描画ごとに新しいオブジェクトになる。カードへ渡す pressTag は、
  // 最新の条件を ref から読んで参照を保ち、無関係な描画でカードを描き直させない。
  const latestConditions = useRef({ criteria, tagIds });
  latestConditions.current = { criteria, tagIds };
  const applyTags = useCallback(
    (next: ListCriteria, mode: HistoryMode, ids: readonly number[]) =>
      apply(next, mode, { [TAG_PARAM]: serializeTagIds(ids) }),
    [apply],
  );

  // --- 人物・団体の絞り込み（047 ui-design.md「Library: filter by a person or organization」） ---
  // チップを押すとそのパラメータだけを外す。タグともう一方の種類は残る。
  const removeCredit = useCallback(
    (kind: CreditKind) => {
      resetPreview();
      apply(latestConditions.current.criteria, "push", withoutCredit(kind));
    },
    [apply, resetPreview],
  );
  // 名前を読んだら `404` だった（もう無い）人物・団体は、履歴を増やさずに URL から外して
  // 知らせる。条件が変わるので一覧も読み直される。
  const creditGone = useCallback(
    (kind: CreditKind) => {
      toast(
        kind === "person"
          ? t.library.activeTags.personGone
          : t.library.activeTags.organizationGone,
      );
      apply(latestConditions.current.criteria, "replace", withoutCredit(kind));
    },
    [apply, toast],
  );
  // --- タグの種類の絞り込み（051 ui-design.md「Library」） ---
  // 選ぶ・チップで外す・もう無い種類を外すのは `tagKind` だけで、ほかの条件は残る。
  const applyTagKind = useCallback<ApplyTagKind>(
    (extra, mode) => {
      resetPreview();
      apply(latestConditions.current.criteria, mode, extra);
    },
    [apply, resetPreview],
  );
  const tagKinds = useLibraryTagKinds(credits.tagKind, applyTagKind);
  // --- 追加日と長さ（052 ui-design.md「Added and length」） ---
  // 選ぶとその値だけを書き換えて履歴を 1 つ増やす。ほかの条件は残る。
  const changeAdded = useCallback(
    (days: number | undefined) => applyTagKind(withAddedWithin(days), "push"),
    [applyTagKind],
  );
  const changeLength = useCallback(
    (range: LengthRange) => applyTagKind(withLength(range), "push"),
    [applyTagKind],
  );
  // 送れない値（範囲の外、下限が上限を超える）は、要求の前に履歴を増やさずに URL から落とす。
  useEffect(() => {
    if (addedLengthDropped) applyTagKind(addedLengthValues(addedLength), "replace");
  }, [addedLength, addedLengthDropped, applyTagKind]);
  const changeTagKind = tagKinds.change;
  const removeTagKind = useCallback(() => changeTagKind(undefined), [changeTagKind]);

  const pressTag = useCallback(
    (tag: TagRef) => {
      const { criteria: current, tagIds: currentTagIds } = latestConditions.current;
      if (currentTagIds.includes(tag.id)) return; // すでに絞り込み中なら何も変わらない。
      if (currentTagIds.length >= MAX_TAG_COUNT) {
        toast(t.errors.reason.too_many_tag_filters({ limit: MAX_TAG_COUNT }));
        return;
      }
      resetPreview();
      applyTags(current, "push", addTagId(currentTagIds, tag.id));
    },
    [applyTags, resetPreview, toast],
  );
  const removeActiveTag = useCallback(
    (id: number) => {
      resetPreview();
      applyTags(criteria, "push", removeTagId(tagIds, id));
    },
    [applyTags, criteria, resetPreview, tagIds],
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
  const [restored] = useState(() =>
    takeListSnapshot({ ...criteria, tags: tagIds, ...libraryFilter }),
  );
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
    refreshInPlace,
    staleGroups,
  } = useVideos({ ...criteria, tag: tagIds, ...libraryFilter }, restored, "library");

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
      setSelection((current) => setGroup(current, selectedGroupOf(group), selected));
    },
    [invalidateSelectAll],
  );
  // 選択中にグループのカードのタグを押したときの切り替え。グループとして選んで
  // いれば外し、そうでなければグループとして選ぶ（selection.ts の isGroupSelected）。
  const toggleGroupSelection = useCallback(
    (group: LibraryGroup) => {
      invalidateSelectAll();
      setSelection((current) => toggleGroup(current, selectedGroupOf(group)));
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

  // --- ドラッグ（specs/050-drag-organize/ui-design.md「Library」） ---
  // グループへ落として移すかマージした項目はすぐ隠し、動画を動画に落とした結果はすぐ見せる。
  // 取り消しの通知は画面を離れると閉じる。
  const moves = useLibraryMoves(items, refreshInPlace, clearSelection);
  const undo = useUndoToast(refreshInPlace);
  const drops = useVideoDrops({
    items: moves.visible,
    keyOf: itemKey,
    videoOf: itemVideo,
    groupOf: itemGroup,
    from: null,
    groupItem: libraryGroupItem,
    reread: refreshInPlace,
    onAccepted: clearSelection,
    undo,
  });
  const shown = drops.shown;

  // --- 「すべて選択」 ---
  // 読み込んでいないページを含む、今の条件の全件の id を選ぶ。応答の `groups` は
  // 選んだグループにする（035 research.md R-7）。
  const selectAll = useCallback(() => {
    const seq = (selectAllSeq.current += 1);
    setSelectingAll(true);
    listLibraryIds({ query, watch, playable, favorite, tag: tagIds, ...libraryFilter })
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
          applyTags(latestCriteria, "replace", remaining);
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
  }, [applyTags, libraryFilter, favorite, playable, query, tagIds, toast, watch]);

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
  const conditionsSignature = [
    criteriaKey(criteria),
    tagIds.join(","),
    creditSignature(credits),
    addedLengthSignature(addedLength),
  ].join("\u0000");
  const knownConditionsSignature = useRef(conditionsSignature);
  useEffect(() => {
    if (knownConditionsSignature.current === conditionsSignature) return;
    knownConditionsSignature.current = conditionsSignature;
    clearSelection();
  }, [clearSelection, conditionsSignature]);
  useScrollTopOnChange(conditionsSignature);

  useEscapeClearsSelection(selectedIds.size > 0, clearSelection);

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
    // 画像や行の高さが決まる前の 1 フレームで位置がずれることがあるので、次の描画の前に
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
  // 自動の取り込みが終わったら、開いている一覧を消さずにその場で取り直す。
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
    // 落とした結果をまだ重ねている間（サーバーの答えか読み直しの前）の一覧は控えにしない。
    // 操作の成功は控えを捨てるので、ここで古い並びを覚え直すと戻ったときにそれが出てしまう。
    if (shown !== items) return;
    saveListSnapshot(
      { ...criteria, tags: tagIds, ...libraryFilter },
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
  }, [
    libraryFilter,
    criteria,
    cursor,
    hasMore,
    items,
    shown,
    staleGroups,
    tagIds,
    total,
  ]);

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
    applyTags(criteria, "replace", remaining);
  }, [applyTags, criteria, missingTagIds, tagIds, toast]);

  // 控えから一覧を戻したときは一覧の要求をしないので missingTagIds が届かない。
  // 画面が開くときに取り直す共有のタグの一覧と URL の tag を突き合わせ、無い id が
  // あれば同じく伝えて取り除く。マウント時に1回だけ行う。
  const mountRef = useRef({ criteria, tagIds, apply: applyTags, toast });
  mountRef.current = { criteria, tagIds, apply: applyTags, toast };
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
        currentApply(currentCriteria, "replace", remaining);
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
  const conditioned =
    hasTagConditions(criteria, tagIds, credits) || hasAddedLength(addedLength);
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

  // スマートフォルダのカードのタグを押すと、グループのカードと同じくそのタグで絞り込む。
  // カードは選択を持たないので、選択中に押しても何も選ばない。
  const renderFolderTagsRow = useCallback(
    (tags: readonly VideoTag[]) => (
      <CardTagRow
        tags={tags}
        selectionMode={selectionMode}
        onPress={pressTag}
        onToggleSelection={ignoreToggle}
      />
    ),
    [pressTag, selectionMode],
  );

  const groupProps = (group: LibraryGroup) => ({
    group,
    backTo: listUrl,
    selected: isGroupSelected(selection, group.id),
    selectionMode,
    onSelect: owner ? changeGroupSelection : undefined,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
    tagsRow: renderGroupTagsRow,
    nameField: drops.nameFieldFor(group.id),
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

  const smartFolderProps = (folder: SmartFolderSummary) => ({
    folder,
    backTo: listUrl,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
    tagsRow: view === "grid" ? renderFolderTagsRow : undefined,
  });

  // 所有者のカードと行はドラッグの層に加わる。ゲストは今のまま。スマートフォルダのカードは
  // 持ち上げず、上に落とすと拒否する（specs/052-smart-folders/ui-design.md「Smart folder card
  // and row」）。
  const card = (item: LibraryItem) => {
    if (item.kind === "smartFolder") {
      return (
        <SmartFolderCard
          key={itemKey(item)}
          {...smartFolderProps(item.smartFolder)}
          refusesDrop={owner}
        />
      );
    }
    const node =
      item.kind === "video" ? (
        <VideoCard key={itemKey(item)} {...rowProps(item.video)} />
      ) : (
        <GroupCard key={itemKey(item)} {...groupProps(item.group)} />
      );
    return owner ? (
      <DraggableCard key={itemKey(item)} id={itemKey(item)}>
        {node}
      </DraggableCard>
    ) : (
      node
    );
  };
  const row = (item: LibraryItem) => {
    if (item.kind === "smartFolder") {
      return (
        <SmartFolderRow
          key={itemKey(item)}
          {...smartFolderProps(item.smartFolder)}
          owner={owner}
        />
      );
    }
    if (item.kind === "video") {
      const Row = owner ? DraggableVideoRow : VideoRow;
      return <Row key={itemKey(item)} {...rowProps(item.video)} />;
    }
    const Row = owner ? DraggableGroupRow : GroupRow;
    return <Row key={itemKey(item)} {...groupProps(item.group)} />;
  };
  const withDrag = (content: ReactNode) =>
    owner ? (
      <LibraryDragLayer
        items={shown}
        list={view === "list"}
        selection={selection}
        onto={drops.onto}
        onDrop={drops.hold}
        onMove={moves.move}
        onMerge={moves.merge}
        onLift={resetPreview}
        held={drops.held}
        attached={drops.attached}
      >
        {content}
      </LibraryDragLayer>
    ) : (
      content
    );

  const body = initialLoadFailed ? (
    <LoadFailed reason={error} onRetry={reload} />
  ) : empty ? (
    <LibraryEmpty
      conditioned={conditioned}
      owner={owner}
      onSearch={() => searchField.current?.focus()}
    />
  ) : loading ? (
    <LoadingState
      label={t.list.loading}
      layout={view === "grid" ? "grid" : "table"}
      size={zoom}
      count={view === "grid" ? skeletonCount : 6}
    />
  ) : (
    <div ref={list} onClick={saveSnapshot}>
      {withSmartFolderCards(
        withDrag(
          view === "grid" ? (
            // タグの行の幅の見張りは一覧に1つだけ（B2、ui-design.md「Overflow」）。
            <TagRowMeasureProvider>
              <Grid zoom={zoom}>
                {/* グループはふつうの動画と同じ並びに1枚のカードで混ぜる。区画や
                見出しは設けない（ui-design.md「Screen boundary」、要件 15）。 */}
                {shown.map(card)}
              </Grid>
            </TagRowMeasureProvider>
          ) : (
            <DataTable label={t.library.title}>
              <LibraryTableHeader owner={owner} />
              <TableBody>{shown.map(row)}</TableBody>
            </DataTable>
          ),
        ),
      )}
    </div>
  );

  return (
    // 「New group」「Add to group」で足したものはいちばん上から外れるので、一覧をその場で
    // 読み直す（specs/048-groups/ui-design.md「Add to group」の Result）。
    <GroupChangedContext value={refreshInPlace}>
      <ListPage
        header={
          <PageHeader
            title={t.library.title}
            actions={
              owner &&
              conditioned && (
                <SaveSmartFolderButton
                  filter={{ criteria, tagIds, credits, addedLength }}
                  sort={sort}
                />
              )
            }
            count={
              initialLoadFailed ? undefined : (
                <span
                  role="status"
                  aria-label={t.library.resultsLabel}
                  aria-live="polite"
                >
                  {resultStatus}
                </span>
              )
            }
          />
        }
        toolbar={
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
              extraFilter={
                <>
                  {owner && (
                    <TagKindSelect
                      value={credits.tagKind}
                      kinds={tagKinds.kinds}
                      onValueChange={changeTagKind}
                    />
                  )}
                  <AddedSelect
                    value={addedLength.addedWithin}
                    onValueChange={changeAdded}
                  />
                  <LengthSelect value={addedLength} onValueChange={changeLength} />
                </>
              }
              extraFilterCount={
                (credits.tagKind === undefined ? 0 : 1) + addedLengthCount(addedLength)
              }
              onFilterOpen={owner ? tagKinds.load : undefined}
            />
          </TopBarPortal>
        }
        band={
          hasFilterChips(tagIds, credits) && (
            <ActiveTagFilters
              tagIds={tagIds}
              onRemove={removeActiveTag}
              credits={credits}
              onRemoveCredit={removeCredit}
              onCreditGone={creditGone}
              tagKindName={tagKinds.name}
              onRemoveTagKind={removeTagKind}
              searchFieldRef={searchField}
            />
          )
        }
        selectionBar={
          owner && (
            <SelectionBar
              count={selectedIds.size}
              allSelected={allSelected}
              selectedIds={selectedIdsArray}
              favoriteVideoIds={favoriteSelection.videoIds}
              favoriteGroupIds={favoriteSelection.groupIds}
              selectingAll={selectingAll}
              onSelectAll={selectAll}
              onClear={clearSelection}
              onTagRemoved={onTagRemoved}
              onBundled={onBundled}
            />
          )
        }
      >
        {body}
        {loadingMore && <LoadMoreRow status="loading" label={t.list.loading} />}
        {error !== null && items.length > 0 && (
          <LoadMoreFailed reason={error} onRetry={retryLoadMore} />
        )}
        <div ref={sentinel} aria-hidden="true" className="h-px" />
      </ListPage>
    </GroupChangedContext>
  );
}

/**
 * LibraryEmpty は一覧が空のときの本体である。条件で絞って空なら検索へ戻る案内、
 * 条件が無ければ所有者には最初の手順、ゲストには空であることだけを出す。
 */
function LibraryEmpty({
  conditioned,
  owner,
  onSearch,
}: {
  conditioned: boolean;
  owner: boolean;
  onSearch: () => void;
}) {
  if (conditioned) return <NoMatches onSearch={onSearch} />;
  return owner ? <EmptyLibrary /> : <GuestEmpty />;
}

/**
 * LibraryTableHeader はリスト表示の見出しの行である。選択とタグなどの列は所有者だけに出す。
 * スマートフォルダのページも同じ列で並べる。
 */
export function LibraryTableHeader({ owner }: { owner: boolean }) {
  return (
    <TableHeader>
      <TableRow className="hover:bg-transparent">
        {owner && <TableHead className="w-10" />}
        <TableHead className="w-list-thumb-cell" />
        <TableHead className="pr-4">{t.library.columns.title}</TableHead>
        {owner && <TableHead className="w-8" />}
        {owner && <TableHead className="hidden w-8 sm:table-cell" />}
        {owner && <TableHead className="hidden w-8 sm:table-cell" />}
        <TableHead className="hidden w-16 pr-4 sm:table-cell" />
        <TableHead className="w-list-number pr-4 text-right">
          {t.library.columns.duration}
        </TableHead>
        <TableHead className="hidden w-list-number pr-4 text-right md:table-cell">
          {t.library.columns.quality}
        </TableHead>
        <TableHead className="hidden w-list-number-wide pr-4 text-right md:table-cell">
          {t.library.columns.size}
        </TableHead>
        <TableHead className="hidden w-list-date pr-3 text-right lg:table-cell">
          {t.library.columns.added}
        </TableHead>
      </TableRow>
    </TableHeader>
  );
}

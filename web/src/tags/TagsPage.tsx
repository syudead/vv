import { Plus } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { RequestFailed } from "../api/client";
import {
  batchTags,
  confirmTag,
  createTag,
  deleteTag,
  maxTagBatch,
  rejectTag,
  renameTag,
  type Tag,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import {
  readTagListPreferences,
  writeTagListPreferences,
} from "../preferences/tagListPreferences";
import { AdminTablePage } from "../ui/patterns/admin-table-page";
import { DataTable } from "../ui/patterns/data-table";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { TableBody } from "../ui/shadcn/table";
import { useToast } from "../ui/Toast";
import type { HistoryMode } from "../videoList/listCriteria";
import BulkTagDialog, { type BulkTagAction } from "./BulkTagDialog";
import CreateTagRow from "./CreateTagRow";
import DeleteTagDialog from "./DeleteTagDialog";
import MergeTagDialog from "./MergeTagDialog";
import RejectedNames from "./RejectedNames";
import RejectTagDialog from "./RejectTagDialog";
import SynonymsDialog from "./SynonymsDialog";
import type { TagListSort } from "./tagListOrder";
import {
  addTag,
  confirmTags,
  insertionIndex,
  matchesTagQuery,
  removeTags,
  replaceTag,
} from "./tagPageRows";
import { tagFieldError, type TagFieldError } from "./tagNameField";
import { type TagListTab, useTagListCriteria } from "./tagListUrl";
import TagRow from "./TagRow";
import {
  TagListChanged,
  TagLoadFailed,
  TagLoadingMore,
  TagLoadMoreFailed,
  TagStaleList,
} from "./TagListNotices";
import TagSelectionBar from "./TagSelectionBar";
import { tagCountText, tagListView } from "./tagListView";
import {
  countSelectable,
  keepLoaded,
  selectAllCheck,
  selectedKinds,
  withoutIds,
} from "./tagSelection";
import { useRejectedNames } from "./useRejectedNames";
import TagFilterChips from "./TagFilterChips";
import TagListEmptyState from "./TagListEmptyState";
import TagListHead from "./TagListHead";
import TagsTabs, { tagsPanelId, tagsTabId } from "./TagsTabs";
import { useTagPage } from "./useTagPage";
import { ROW_OVERSCAN, useVirtualTagRows } from "./useVirtualTagRows";
import TopBarPortal from "../shell/TopBarPortal";
import TagToolbar from "./TagToolbar";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isTagNotTentative(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_tentative";
}

/** RowHandlers は行へ渡す操作である。行の再描画を減らすため、同じ関数を渡し続ける。 */
interface RowHandlers {
  onStartRename: (tag: Tag) => void;
  onCancelRename: (tag: Tag) => void;
  onSubmitRename: (tag: Tag, name: string) => void;
  onOpenSynonyms: (tag: Tag) => void;
  onOpenMerge: (tag: Tag) => void;
  onDelete: (tag: Tag) => void;
  onConfirm: (tag: Tag) => void;
  onReject: (tag: Tag) => void;
  onDraftChange: () => void;
  onSelect: (tag: Tag, selected: boolean) => void;
}

/**
 * TagsPage はサイドバーの「タグ」から開く管理画面である（ui-design.md「Tag
 * management page」）。一覧・検索・作成・改名・削除・統合・シノニムの登録と
 * 解除を持つ。
 *
 * タグの一覧は共有の保持（`web/src/api/tags.ts` の `getTags`）を使わず、検索・
 * 絞り込み・並び順の条件ごとに `GET /api/tags` から 100 件ずつ受け、スクロールに
 * 合わせて続きを読む（specs/036-tag-admin-scale/data-model.md §4、research.md
 * R-1・R-11）。1 件とまとめての操作の直後は一覧を取り直さず、読み込んだ行の中で
 * 書き換える（R-12、`tagPageRows.ts`）。タグがもう無いとき（`tag_not_found`、
 * `notFoundIds`）だけ、ほかのタグも変わっているかもしれないので先頭から取り直す。
 *
 * 仮のタグ（specs/031-tentative-tags/ui-design.md「Tag management page」）:
 * 「Tentative only」の絞り込み、仮の行の確定・却下、「Rejected names」のタブの
 * 却下した名前の一覧と取り外しも持つ。却下した名前の一覧はこの画面だけが読むので、
 * 共有の保持ではなくここで取る（research.md R-8）。
 *
 * 見た目の置き場（specs/036-tag-admin-scale/ui-design.md「Top bar」「Band」）: 検索・
 * 絞り込み・並び順はライブラリと同じく共通トップバー（`TagToolbar`）、本文の先頭は
 * 見出しの行（選んでいる間は選択の行）・タブ・絞り込みのチップ・列の見出しの帯。
 * 検索語・絞り込み・並び順・タブは URL に載せる（`tagListUrl.ts`）。
 */
export default function TagsPage() {
  const toast = useToast();
  const rejected = useRejectedNames();
  const {
    page,
    loadError,
    firstPending,
    more,
    appliedSearch,
    pageRef,
    load,
    reload,
    loadMore,
    retryMore,
    applyLocal,
  } = useTagPage(() => scrollListToTop());
  /**
   * 検索語・絞り込み・並び順・タブは URL に載せる（ライブラリの一覧の条件と同じ。
   * specs/036-tag-admin-scale/ui-design.md「URL state」）。URL に sort が無いときは
   * 端末に残した並び順（research.md R-7）を使う。
   */
  const [preferredSort] = useState<TagListSort>(() => readTagListPreferences().sort);
  const { criteria, apply: applyCriteria } = useTagListCriteria(preferredSort);
  const { query: search, tentativeOnly, unusedOnly, sort, tab: urlTab } = criteria;
  /**
   * tab は描いているタブである。ふだんは URL の `tab` に従うが、作成・改名の送信中は
   * 「Tags」のまま保つ（ui-design.md「Tabs」の「作成・改名の送信中は切り替えない」を、
   * 戻る・進むで URL が変わったときにも当てる）。下の `useLayoutEffect` が追わせる。
   */
  const [tab, setTab] = useState<TagListTab>(urlTab);
  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);

  /**
   * renaming は改名中のタグ（始めた時点の状態）である。条件を変えて読み直した行に
   * 無くても、これを並び順の位置に差し込んで残す（ui-design.md「Row checkbox」）。
   */
  const [renaming, setRenaming] = useState<Tag | null>(null);
  const renamingId = renaming?.id ?? null;
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);

  const [deletingTag, setDeletingTag] = useState<Tag | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);

  /**
   * merging は統合の窓の統合元である。行の「別のタグへ統合…」は 1 件、選択バーの
   * 「Merge into one tag…」は選んだタグ（`fromSelection`）。開いた時点で固定する。
   */
  const [merging, setMerging] = useState<{
    sources: readonly Tag[];
    fromSelection: boolean;
  } | null>(null);
  /**
   * synonymsTag はシノニムの窓のタグ（最新の状態）である。窓の中の操作で今の条件に
   * 合わなくなり読み込んだ行から外れても、窓は開いたままにする。
   */
  const [synonymsTag, setSynonymsTag] = useState<Tag | null>(null);
  const synonymsTagId = synonymsTag?.id ?? null;
  /** シノニムの窓を開いたときの並び。閉じたときに行が外れていればここから次の行を探す。 */
  const synonymsOrderRef = useRef<readonly Tag[]>([]);

  const [confirming, setConfirming] = useState<ReadonlySet<number>>(new Set());
  const confirmingRef = useRef(new Set<number>());

  const [rejectingTag, setRejectingTag] = useState<Tag | null>(null);
  const [rejectPending, setRejectPending] = useState(false);
  const [rejectError, setRejectError] = useState<UiText | null>(null);

  /**
   * selected は選んだ行の id である。見えている行の部分集合で、検索・絞り込みを
   * 変えて見えなくなった行と改名中の行は外す（specs/036-tag-admin-scale/data-model.md
   * §4「選択」）。
   */
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set());
  /** まとめての操作の送信中。`confirm` はバーの「Confirm」、`dialog` は確認の窓から。 */
  const [bulkPending, setBulkPending] = useState<"confirm" | "dialog" | null>(null);
  const [bulkDialog, setBulkDialog] = useState<{
    action: BulkTagAction;
    ids: readonly number[];
  } | null>(null);
  const [bulkError, setBulkError] = useState<UiText | null>(null);
  const selectAllRef = useRef<HTMLButtonElement | null>(null);
  const tabsId = useId();
  const barConfirmRef = useRef<HTMLButtonElement | null>(null);
  const barMergeRef = useRef<HTMLButtonElement | null>(null);
  const barRejectRef = useRef<HTMLButtonElement | null>(null);
  const barDeleteRef = useRef<HTMLButtonElement | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  /** filterButtonRef はトップバーの「Filter」のボタンである。 */
  const filterButtonRef = useRef<HTMLButtonElement | null>(null);
  /** chipRefs は見出しの下の絞り込みのチップ（外したあとのフォーカス先）である。 */
  const tentativeChipRef = useRef<HTMLButtonElement | null>(null);
  const unusedChipRef = useRef<HTMLButtonElement | null>(null);
  /** listBoxRef は帯の下の一覧の箱（作成の行・行・空の状態を入れる）である。 */
  const listBoxRef = useRef<HTMLDivElement | null>(null);
  /** scrollMargin は、一覧の上端の文書の中での位置（px）である。 */
  const [scrollMargin, setScrollMargin] = useState(0);
  /**
   * bandRef は上部バーの下に留める帯（見出しか選択バー・タブ・絞り込みのチップ）で、
   * stuckBottom は留まったときの帯の下端の表示域の中での位置（`top` の上部バーの高さ＋
   * 帯の高さ、px）である。行へスクロールするときに、行が帯の下に隠れないよう
   * これを差し引く（ui-design.md「Band」）。帯の高さは幅で変わるので測り直す。
   */
  const bandRef = useRef<HTMLDivElement | null>(null);
  const [stuckBottom, setStuckBottom] = useState(0);
  /**
   * 表の列の見出しは帯の直下（stuckBottom）に貼り付く。行へスクロールするときは、
   * 帯に加えて見出しの高さ（headHeight）も差し引く。
   */
  const [headHeight, setHeadHeight] = useState(0);
  // 列の見出しの高さを測る。表が無い間（読み込み中・空）は 0。
  const headRef = useCallback((head: HTMLTableSectionElement | null) => {
    if (head === null) return;
    const measure = () => setHeadHeight(head.offsetHeight);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(head);
    return () => {
      observer.disconnect();
      setHeadHeight(0);
    };
  }, []);
  const rowsTop = stuckBottom + headHeight;

  /**
   * filtersRef は今描いている「Tentative only」「Unused only」である。要求の応答を
   * 待つ間に絞り込みが変わることがあるので、応答のあとのフォーカス先は
   * 閉じ込めた（押した時点の）値ではなくこれで決める。
   */
  const filtersRef = useRef({ tentativeOnly: false, unusedOnly: false });

  /**
   * fallbackFocus は、行へ移せないときの最後の行き先である。ふだんは
   * 「新しいタグ」、「Tentative only」「Unused only」が効いている間はトップバーの
   * 「Filter」（次の操作が「絞り込みを外す」だから。specs/031-tentative-tags/ui-design.md
   * 「Toolbar」、specs/036-tag-admin-scale/ui-design.md「Active filters」）。選択の行が見出しと
   * 入れ替わっていて「新しいタグ」が無いときは、列の見出しの先頭のチェックへ。
   */
  function fallbackFocus() {
    const filtered = filtersRef.current.tentativeOnly || filtersRef.current.unusedOnly;
    const candidates = filtered
      ? [filterButtonRef.current, createButtonRef.current, selectAllRef.current]
      : [createButtonRef.current, selectAllRef.current, filterButtonRef.current];
    candidates
      .find((element) => element?.isConnected === true && !element.disabled)
      ?.focus();
    // 候補を確かめるために描かせた行（`renderRow`）を Tab の順に残さない。
    releasePinIfFocusOutside();
  }

  /** focusFilterButton は「Filter」へ移す。押せなければ「新しいタグ」へ。 */
  function focusFilterButton() {
    const filter = filterButtonRef.current;
    if (filter?.isConnected === true && !filter.disabled) filter.focus();
    else createButtonRef.current?.focus();
  }

  /**
   * scrollListToTop は、一覧の先頭が帯の下に隠れていれば先頭まで戻す（ui-design.md
   * 「Band」）。条件を変えて先頭のページが届いたときと「新しいタグ」で使う。
   */
  function scrollListToTop() {
    const listTop = listBoxRef.current?.getBoundingClientRect().top;
    if (listTop !== undefined && listTop < stuckBottom) {
      window.scrollTo({ top: Math.max(0, window.scrollY + listTop - stuckBottom) });
    }
  }

  /**
   * reloadList は「一覧が変わった」の「Reload」で、選択を空にして先頭から読み直す
   * （ui-design.md「Loading more」）。
   */
  function reloadList() {
    setSelected(new Set());
    void reload();
  }

  function changeSort(next: TagListSort) {
    applyCriteria({ sort: next }, "push");
    writeTagListPreferences({ sort: next });
  }

  function commitQuery(next: string, mode: HistoryMode) {
    applyCriteria({ query: next }, mode);
  }

  /**
   * changeTab は「Tags」「Rejected names」のタブを切り替える。選択・作成・改名は
   * 「Tags」のタブの中のものなので、離れるときに閉じる。
   */
  function changeTab(next: TagListTab): boolean {
    if (next === tab) return true;
    if (createPending || renamePending) return false;
    setSelected(new Set());
    setCreating(false);
    setCreateError(null);
    setRenaming(null);
    setRenameError(null);
    applyCriteria({ tab: next }, "push");
    return true;
  }

  // 検索はサーバーが全部のタグに掛ける（research.md R-1）。照合形は、空白だけの検索を
  // 絞り込み中と数えないためと、同じ照合形になる打ち直しで読み直さないために使う。
  const normalizedQuery = foldForMatch(search).trim();
  const searchRef = useRef(search);
  useLayoutEffect(() => {
    searchRef.current = search;
  });

  // 検索・絞り込み・並び順のどれかが変わるたびに、選択を空にして先頭のページを読み
  // 直す（data-model.md §4「条件」、ui-design.md「Top bar」）。開いたときもここで読む。
  useEffect(() => {
    setSelected((current) => (current.size === 0 ? current : new Set()));
    void load({
      query: normalizedQuery,
      search: searchRef.current,
      tentativeOnly,
      unusedOnly,
      sort,
    });
  }, [normalizedQuery, tentativeOnly, unusedOnly, sort, load]);

  useLayoutEffect(() => {
    filtersRef.current = { tentativeOnly, unusedOnly };
  }, [tentativeOnly, unusedOnly]);

  // 戻る・進むで URL のタブが変わったら、描くタブを追わせる。「Rejected names」へ
  // 移るときは、選択と作成・改名を閉じる（`changeTab` と同じ）。作成・改名の送信中は
  // 「Tags」のまま待つ — 閉じると行が消え、失敗したときに下書きと誤りを出す先が無い。
  // 送信が終わって作成・改名の行が残っている（失敗した）ときは、切り替えを断ったことに
  // して URL を描いているタブへ戻す（`changeTab` が送信中に何もしないのと同じ結果）。
  const editPending = createPending || renamePending;
  const heldTabRef = useRef(false);
  useLayoutEffect(() => {
    if (urlTab === tab) {
      heldTabRef.current = false;
      return;
    }
    if (editPending) {
      heldTabRef.current = true;
      return;
    }
    if (heldTabRef.current && (creating || renaming !== null)) {
      heldTabRef.current = false;
      applyCriteria({ tab }, "replace");
      return;
    }
    heldTabRef.current = false;
    if (urlTab !== "tags") {
      setSelected((current) => (current.size === 0 ? current : new Set()));
      setCreating(false);
      setCreateError(null);
      setRenaming(null);
      setRenameError(null);
    }
    setTab(urlTab);
  }, [urlTab, tab, editPending, creating, renaming, applyCriteria]);

  /**
   * shown は、その1件が読み込んだ行の条件（検索と「Tentative only」「Unused only」）で
   * 一覧に出るかである。要求の応答のあとで呼ぶので、`pageRef` の今の条件で決める。
   */
  function shown(tag: Tag): boolean {
    const query = pageRef.current?.query;
    return query === undefined || matchesTagQuery(tag, query);
  }

  const rows = useMemo(() => page?.rows ?? [], [page]);

  /**
   * visibleRows は実際に並べる行である。改名中の行は、条件を変えて読み直した行に
   * 無くても一覧から外さない（外すとその行が消え、打っている途中の名前を失う）。
   * 並び順の位置に差し込む。件数の行はこれを数えない。
   */
  const visibleRows = useMemo(() => {
    if (renaming === null || page === undefined) return rows;
    if (rows.some((tag) => tag.id === renaming.id)) return rows;
    const index = insertionIndex(rows, renaming, page.query.sort);
    return [...rows.slice(0, index), renaming, ...rows.slice(index)];
  }, [rows, page, renaming]);

  // 選択は読み込んだ行の部分集合に保つ。操作や取り直しで行から消えた id と、
  // 改名中の行を外す（specs/036-tag-admin-scale/data-model.md §4「選択」）。条件の
  // 変更では、読み直しを始めるときに空にする。
  useLayoutEffect(() => {
    setSelected((current) => keepLoaded(current, rows, renamingId));
  }, [rows, renamingId]);

  const selectableCount = countSelectable(rows, renamingId);
  /**
   * 上限 `maxTagBatch` は送る id の数に掛かる（research.md R-4）。先頭のチェックは
   * 読み込んだ行の数で、まとめての操作は選んだ数で止める（ui-design.md「Column header」
   * 「Enabled and disabled」）。
   */
  const selectAllOverLimit = rows.length > maxTagBatch;
  const selectionOverLimit = selected.size > maxTagBatch;
  const selection = useMemo(() => selectedKinds(rows, selected), [selected, rows]);
  const selectAllState = selectAllCheck(selected.size, selectableCount);

  const {
    listRef,
    registerRefs,
    visibleRowsRef,
    virtualizer,
    virtualItems,
    afterCommit,
    focusRow,
    focusAfterRemoval,
    releasePinIfFocusOutside,
    handleListFocus,
    handleListBlur,
    handleListKeyDown,
  } = useVirtualTagRows({
    visibleRows,
    renamingId,
    scrollMargin,
    rowsTop,
    fallbackFocus: () => fallbackFocus(),
  });

  // 続きを読むきっかけ: 仮想化が描く最後の行が読み込んだ行の末尾から overscan 行
  // 以内に入ったら、続きを 1 回要求する。操作で行が 1 つも残らなかったとき（描く行が
  // 無い）も、ここで要求する（data-model.md §4「続きを読むきっかけ」、ui-design.md
  // 「Loading more」）。読んでいる間・失敗・食い違いの間は `loadMore` が読まない。
  const lastRendered = virtualItems.at(-1)?.index ?? -1;
  const nextCursor = page?.nextCursor;
  // 「Rejected names」のタブの間は一覧を描かないので、続きも読まない。
  const onTagsTab = tab === "tags";
  useEffect(() => {
    if (nextCursor === undefined || !onTagsTab) return;
    if (lastRendered >= visibleRows.length - ROW_OVERSCAN) loadMore();
  }, [
    onTagsTab,
    lastRendered,
    visibleRows.length,
    nextCursor,
    more,
    firstPending,
    loadError,
    loadMore,
  ]);

  // 一覧の上端は作成の行の有無やツールバーの折り返しで動くので、文書の
  // 大きさが変わるたびに測り直す。
  const hasList = page !== undefined && visibleRows.length > 0 && onTagsTab;
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!hasList || list === null) return;
    const measure = () =>
      setScrollMargin(list.getBoundingClientRect().top + window.scrollY);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(document.body);
    return () => observer.disconnect();
  }, [hasList, listRef]);

  // 帯の下端（上部バー＋帯の高さ）を測る。帯は幅や選択で高さが変わる。
  useLayoutEffect(() => {
    const band = bandRef.current;
    if (band === null) return;
    const measure = () => {
      const top = Number.parseFloat(getComputedStyle(band).top);
      setStuckBottom((Number.isFinite(top) ? top : 0) + band.offsetHeight);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(band);
    return () => observer.disconnect();
  }, []);

  // ブラウザがフォーカスした要素を表示域へ寄せるときも、帯の下に隠さない。
  // 文書のスクロールはこの画面のものではないので、離れるときに戻す。
  useEffect(() => {
    const root = document.documentElement;
    const previous = root.style.scrollPaddingTop;
    root.style.scrollPaddingTop = `${String(rowsTop)}px`;
    return () => {
      root.style.scrollPaddingTop = previous;
    };
  }, [rowsTop]);

  /**
   * applied は読み込んだ行の条件である。件数の行と空の状態は、入力中の条件ではなく
   * 届いた行の条件で決める（先頭のページを待つ間は前の行と件数を残す）。
   */
  const applied = page?.query;
  /** totalAll は全部のタグの数（読み込んでいないタグを含む）。 */
  const totalAll = page?.totalAll ?? 0;
  const countText = tagCountText(page);

  function openCreate() {
    // 改名の送信中は、その応答が届くまで新しく作成を始めない（B2 と同じ規則。
    // 「新しいタグ」自体も createPending・creating では disabled だが、
    // renamePending はボタンの disabled 条件に含めているので、ここは主に
    // キーボード操作などボタンを介さない呼び出しへの保険である）。
    if (renamePending) return;
    // 作成の行はいつも一覧の先頭に入るので、一覧の先頭が帯の下に隠れていれば
    // 先に先頭まで戻す（見えない位置に入力を作らない。ui-design.md「Band」）。
    scrollListToTop();
    setCreating(true);
    setCreateError(null);
    setRenaming(null);
  }

  function clearSearch() {
    applyCriteria({ query: "" }, "push");
    searchInputRef.current?.focus();
  }

  /**
   * changeTentativeOnly は「Filter」の吹き出しの「Tentative only」を押す・外す。
   * フォーカスは吹き出しの中に残る（specs/036-tag-admin-scale/ui-design.md
   * 「Top bar」）。
   */
  function changeTentativeOnly(value: boolean) {
    applyCriteria({ tentativeOnly: value }, "push");
  }

  /** changeUnusedOnly は「Unused only」を押す・外す。`changeTentativeOnly` と同じ。 */
  function changeUnusedOnly(value: boolean) {
    applyCriteria({ unusedOnly: value }, "push");
  }

  /**
   * removeFilterChip は見出しの下のチップの × である。その絞り込みを外し、残る
   * チップへ、無ければ「Filter」へフォーカスを移す（タグが 0 なら「新しいタグ」へ。
   * ui-design.md「Active filters」）。
   */
  function removeFilterChip(which: "tentative" | "unused") {
    const other = which === "tentative" ? unusedOnly : tentativeOnly;
    applyCriteria(
      which === "tentative" ? { tentativeOnly: false } : { unusedOnly: false },
      "push",
    );
    setTimeout(() => {
      if (other) {
        (which === "tentative" ? unusedChipRef : tentativeChipRef).current?.focus();
        return;
      }
      if (totalAll === 0) createButtonRef.current?.focus();
      else focusFilterButton();
    }, 0);
  }

  /**
   * clearFilters は「Filter」の吹き出しの「Clear filters」である。両方の絞り込みを
   * 外す。吹き出しが閉じると、フォーカスは Radix が「Filter」へ戻す。
   */
  function clearFilters() {
    applyCriteria({ tentativeOnly: false, unusedOnly: false }, "push");
  }

  /**
   * showAllFromFilter は「No unused tags」「No tentative tags」などの
   * 「Show all tags」である。絞り込みを外し、「Filter」へ（タグが 0 なら
   * 「新しいタグ」へ）移す（ui-design.md「States」）。
   */
  function showAllFromFilter() {
    applyCriteria({ tentativeOnly: false, unusedOnly: false }, "push");
    setTimeout(
      () => (totalAll === 0 ? createButtonRef.current?.focus() : focusFilterButton()),
      0,
    );
  }

  /**
   * showAllFromFilterSearch は「No unused (tentative) tags match」などの
   * 「Show all tags」である。絞り込みと検索を外し、検索の入力へ移す。
   */
  function showAllFromFilterSearch() {
    applyCriteria({ tentativeOnly: false, unusedOnly: false, query: "" }, "push");
    setTimeout(
      () => (totalAll === 0 ? createButtonRef : searchInputRef).current?.focus(),
      0,
    );
  }

  /**
   * replaceRow は書き換わったタグ 1 件を読み込んだ行へ反映する。書き換わる前の状態は
   * 読み込んだ行（無ければ `before`）から取り、前後を今の条件に照らして件数を
   * 数え直し、並び順の位置へ置き直すか取り除く（data-model.md §4「操作のあとの反映」）。
   */
  function replaceRow(updated: Tag, before?: Tag) {
    applyLocal((current) => {
      if (current === undefined) return current;
      const previous = current.rows.find((item) => item.id === updated.id) ?? before;
      return replaceTag(current, previous, updated);
    });
  }

  /**
   * removeRows はもう無いタグを読み込んだ行から取り除き、件数を減らす。消える前の
   * 状態は読み込んだ行にあればそれを使う。
   */
  function removeRows(removed: readonly Tag[]) {
    applyLocal((current) => {
      if (current === undefined) return current;
      const byId = new Map(current.rows.map((item) => [item.id, item]));
      return removeTags(
        current,
        removed.map((item) => byId.get(item.id) ?? item),
      );
    });
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTag(name);
      // 今の条件に合えば並び順の位置へ置き、件数を数え直す（data-model.md §4
      // 「操作のあとの反映」）。一覧は取り直さない。
      applyLocal((current) =>
        current === undefined ? current : addTag(current, created),
      );
      setCreating(false);
      rejected.reload();
      focusRow(created.id, "name");
    } catch (failure) {
      setCreateError(tagFieldError(failure, { submitted: name }));
    } finally {
      setCreatePending(false);
    }
  }

  async function submitRename(tag: Tag, name: string) {
    setRenameError(null);
    setRenamePending(true);
    const order = visibleRows;
    try {
      const updated = await renameTag(tag.id, name);
      // 改名の前の状態は、読み込んだ行に無ければ（条件を変えて読み直した行の範囲の外）
      // 改名中の行 `tag` から取る。無いと、改名前も条件に合っていたタグを新しく合った
      // ものと数え、`total` が 1 つ増えてしまう。
      replaceRow(updated, tag);
      setRenaming(null);
      rejected.reload();
      // 改名で確定になった仮のタグは「Tentative only」から外れる（031 の
      // ui-design.md「Toolbar」）。検索に一致しなくなったときも同じ扱い。
      if (shown(updated)) focusRow(tag.id, "rename");
      else focusAfterRemoval(order, tag.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRenaming(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      setRenameError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
    } finally {
      setRenamePending(false);
    }
  }

  async function performDelete() {
    if (deletingTag === null) return;
    const target = deletingTag;
    const order = visibleRows;
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTag(target.id);
      removeRows([target]);
      setDeletingTag(null);
      toast(t.tags.deleted(target.name));
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setDeletingTag(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      setDeleteError(errorText(failure));
    } finally {
      setDeletePending(false);
    }
  }

  /**
   * submitConfirm は仮の行の「確定する」を扱う（031 の ui-design.md「Confirm」）。
   * 送信中の行は二度押しを無視する（`confirmingRef` は同じ呼吸の連打も
   * 止める）。成功したら行を差し替え、同じ行の「改名」へ、絞り込みから
   * 外れたら次の行の「改名」へフォーカスを移す。
   */
  async function submitConfirm(tag: Tag) {
    if (confirmingRef.current.has(tag.id)) return;
    confirmingRef.current.add(tag.id);
    setConfirming(new Set(confirmingRef.current));
    const order = visibleRows;
    try {
      const updated = await confirmTag(tag.id);
      replaceRow(updated);
      toast(t.tags.confirmed(updated.name));
      if (shown(updated)) focusRow(updated.id, "rename");
      else focusAfterRemoval(order, updated.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      // 行の中に失敗の1行を置く場所が無いので、トーストで伝える。
      toast(errorText(failure));
    } finally {
      confirmingRef.current.delete(tag.id);
      setConfirming(new Set(confirmingRef.current));
    }
  }

  /** performReject は却下の窓の「Reject」である（031 の ui-design.md「Reject」）。 */
  async function performReject() {
    if (rejectingTag === null) return;
    const target = rejectingTag;
    const order = visibleRows;
    setRejectError(null);
    setRejectPending(true);
    try {
      await rejectTag(target.id);
      removeRows([target]);
      setRejectingTag(null);
      toast(t.tags.rejected(target.name));
      rejected.reload();
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRejectingTag(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      if (isTagNotTentative(failure)) {
        // 別のタブや API で先に確定された（Edge Case「操作の競合」）。
        // 行は確定したタグとして残り、「Tentative only」中は一覧から外れる。
        setRejectingTag(null);
        toast(t.tags.alreadyConfirmed);
        await reload();
        if (filtersRef.current.tentativeOnly) focusAfterRemoval(order, target.id);
        else focusRow(target.id, "menu");
        return;
      }
      setRejectError(errorText(failure));
    } finally {
      setRejectPending(false);
    }
  }

  /** cancelReject は却下の窓を何も変えずに閉じ、その行の「その他の操作」へ戻す。 */
  function cancelReject() {
    if (rejectPending || rejectingTag === null) return;
    const target = rejectingTag;
    setRejectingTag(null);
    setRejectError(null);
    focusRow(target.id, "menu");
  }

  function cancelDelete() {
    if (deletePending || deletingTag === null) return;
    const target = deletingTag;
    setDeletingTag(null);
    setDeleteError(null);
    // 窓は「その他の操作」のメニューの項目から開いた。その項目はメニューが
    // 閉じるともう無いので、ModalFrame の「前のフォーカスへ戻す」には頼らず、
    // その行の「その他の操作」へ明示的に戻す（B2）。
    focusRow(target.id, "menu");
  }

  /**
   * cancelMerge は統合の確認の窓を、何も変えずに閉じる（キャンセル・Esc）。
   * 行の「別のタグへ統合…」は「その他の操作」のメニューの項目から開いたので、
   * cancelDelete と同じくその行の「その他の操作」へ明示的に戻す（B2）。選択バーから
   * 開いたときはバーの「More」へ戻す。
   */
  function cancelMerge() {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    setMerging(null);
    if (fromSelection) afterCommit(() => barMergeRef.current?.focus());
    else focusRow(sources[0]!.id, "menu");
  }

  /**
   * performMerge は統合が成功したときに呼ぶ。統合元は一覧から消え、統合先は
   * サーバーが返した最新の状態（シノニムに統合元の名前を含み、本数が合算）に差し
   * 替わる。選択から開いたときは選択を空にする。トーストの数は実際に統合した数で、
   * もう無かった統合元があれば一覧を取り直す（specs/036-tag-admin-scale/ui-design.md
   * 「Confirmation」、014 の受け入れ条件 12）。
   */
  function performMerge(
    merged: Tag,
    sourceIds: readonly number[],
    notFoundIds: readonly number[],
    target: Tag,
  ) {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    const order = visibleRows;
    const removed = new Set(sourceIds);
    // 統合元を取り除き、統合先を応答の tag で書き換える。読み込んでいない統合先は、
    // 位置が読み込んだ範囲の中なら差し込む。統合の前の統合先（窓の候補）と応答を
    // それぞれ今の条件に照らして件数を数え直す（data-model.md §4「操作のあとの反映」）。
    removeRows(sources.filter((item) => removed.has(item.id)));
    replaceRow(merged, target);
    if (fromSelection) setSelected(new Set());
    else setSelected((current) => withoutIds(current, removed));
    setMerging(null);
    const missing = new Set(notFoundIds);
    const done = sources.filter((item) => removed.has(item.id) && !missing.has(item.id));
    toast(
      done.length === 1
        ? t.tags.merged(done[0]!.name, merged.name)
        : t.tags.selection.merged(done.length, merged.name),
    );
    afterStale(notFoundIds);
    // 統合先は確定になるので、「Tentative only」を押している間は統合元・
    // 統合先のどちらも一覧に無い。そのときは最初の統合元の位置から次の行へ
    // （031 の ui-design.md「Merge」）。
    if (shown(merged)) {
      focusRow(merged.id, "name");
      return;
    }
    const first = order.find((item) => removed.has(item.id));
    if (first === undefined) afterCommit(fallbackFocus);
    else focusAfterRemoval(order, first.id, new Set([...removed, merged.id]));
  }

  /**
   * staleMerge は統合先がもう無かった（tag_not_found）か、統合元がすべてもう無かった
   * （応答の notFoundIds）ときに呼ぶ。ほかの操作の tag_not_found と同じく、窓を閉じて
   * トーストを出し、一覧を取り直す（ui-design.md「States」「Confirmation」）。
   */
  function staleMerge() {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    const order = visibleRows;
    setMerging(null);
    toast(fromSelection ? t.tags.selection.stale : t.tags.gone);
    void reload().then(() => {
      if (fromSelection) {
        const merge = barMergeRef.current;
        if (merge?.isConnected === true && !merge.disabled) merge.focus();
        else focusSelectAllOr(fallbackFocus);
        return;
      }
      focusAfterRemoval(order, sources[0]!.id);
    });
  }

  /** openMergeSelected は選択バーの「Merge into one tag…」である。 */
  function openMergeSelected() {
    if (bulkPending !== null || selected.size === 0) return;
    setMerging({
      sources: visibleRows.filter((tag) => selected.has(tag.id)),
      fromSelection: true,
    });
  }

  /**
   * cancelSynonyms はシノニムの窓を閉じる。「シノニム」はその行に直接置いた
   * ボタンなので（メニューの項目ではない）、その行の「シノニム」へ明示的に
   * 戻す。シノニムの追加で確定になり「Tentative only」から外れていれば、
   * 窓を開いたときの並びから次の行の「改名」へ移す（031 の ui-design.md
   * 「Toolbar」。窓を開いている間はフォーカスを窓の中に残す）。
   */
  function cancelSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    setSynonymsTag(null);
    if (rows.some((tag) => tag.id === id)) {
      focusRow(id, "synonyms");
      return;
    }
    const loadedIds = new Set(rows.map((tag) => tag.id));
    const order = synonymsOrderRef.current;
    const gone = new Set(
      order.filter((tag) => !loadedIds.has(tag.id)).map((tag) => tag.id),
    );
    focusAfterRemoval(order, id, gone);
  }

  const synonymsTagRef = useRef<Tag | null>(synonymsTag);
  useLayoutEffect(() => {
    synonymsTagRef.current = synonymsTag;
  });

  /**
   * updateSynonymsTag はシノニムの登録・解除・シノニム登録に伴う統合が
   * 成功したときに、読み込んだ行の中のその1件を差し替える（一覧は取り直さない。
   * 作成・改名・削除と同じ扱い）。足したシノニムで検索に合うようになる・確定に
   * なって「Tentative only」から外れることがあるので、今の条件に照らして置き直す。
   *
   * `removed` は、シノニム登録に伴う統合（承諾したとき）でだけ渡す、統合元の統合の
   * 前の状態（窓が確かめ直した一覧のもの）である。統合元のタグは統合先のシノニムに
   * なって一覧から消えるので、そのタグを読み込んだ行から取り除いてから統合先を差し
   * 替える。読み込んでいない統合元も、今の条件に合っていれば `total` から引く。
   */
  function updateSynonymsTag(updated: Tag, removed?: Tag) {
    const opened = synonymsTagRef.current;
    applyLocal((current) => {
      if (current === undefined) return current;
      let next = current;
      if (removed !== undefined) {
        const source = next.rows.find((item) => item.id === removed.id) ?? removed;
        next = removeTags(next, [source]);
      }
      const previous =
        next.rows.find((item) => item.id === updated.id) ??
        (opened?.id === updated.id ? opened : undefined);
      return replaceTag(next, previous, updated);
    });
    setSynonymsTag((current) => (current?.id === updated.id ? updated : current));
    // シノニムに足した名前が却下した名前だったなら、一覧から外れる（要件 15）。
    rejected.reload();
  }

  /**
   * removeSynonymFromTag は、1件のシノニムの解除が成功したときに呼ぶ。
   * `SynonymsDialog` に渡した `tag` の閉じ込め（古いかもしれない）ではなく、
   * 関数形で常に最新の行からその名前だけを取り除く。複数のシノニムをほぼ同時に
   * 解除したとき、それぞれの応答が別々にここへ届いても、互いの結果を巻き戻さない
   * （N5・並行する解除）。解除で検索に合わなくなれば行から外す。
   */
  function removeSynonymFromTag(tagId: number, name: string) {
    const without = (item: Tag): Tag => ({
      ...item,
      synonyms: item.synonyms.filter((synonym) => synonym !== name),
    });
    applyLocal((current) => {
      const previous = current?.rows.find((item) => item.id === tagId);
      if (current === undefined || previous === undefined) return current;
      return replaceTag(current, previous, without(previous));
    });
    setSynonymsTag((current) => (current?.id === tagId ? without(current) : current));
  }

  /**
   * staleSynonyms は、シノニムの窓を開いていたタグがもう無かった
   * （tag_not_found）ときに呼ぶ。
   */
  function staleSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    const order = visibleRows;
    setSynonymsTag(null);
    toast(t.tags.gone);
    void reload().then(() => focusAfterRemoval(order, id));
  }

  /**
   * toggleSelectAll は件数の行の先頭のチェックである。空・中間なら読み込んだ行の
   * うち選べる行をすべて選び、全部なら選択を解く。読み込んでいないタグは選ばない
   * （要件 10、ui-design.md「Column header」）。
   */
  function toggleSelectAll() {
    if (selectAllState === true) {
      setSelected(new Set());
      return;
    }
    setSelected(
      new Set(rows.filter((tag) => tag.id !== renamingId).map((tag) => tag.id)),
    );
  }

  function clearSelection() {
    setSelected(new Set());
    // バーが消えるので、フォーカスを件数の行の先頭のチェックへ移す。
    afterCommit(() => {
      const header = selectAllRef.current;
      if (header !== null && !header.disabled) header.focus();
      else fallbackFocus();
    });
  }

  /**
   * afterStale は、まとめての操作の対象の一部がもう無かったときに一覧を取り直す
   * （data-model.md §4「まとめての操作の結果」）。
   */
  function afterStale(notFoundIds: readonly number[]) {
    if (notFoundIds.length === 0) return;
    toast(t.tags.selection.stale);
    void reload();
  }

  /**
   * submitBulkConfirm はバーの「Confirm」である。確認なしで選んだ id 全部を 1 回
   * 送り、確定した行を差し替えて選択から外す。既に確定していた行は選んだまま
   * 残す。失敗したら何も変えない（ui-design.md「Bulk confirm」）。
   */
  async function submitBulkConfirm() {
    if (bulkPending !== null || selected.size === 0) return;
    const ids = [...selected];
    setBulkPending("confirm");
    try {
      const result = await batchTags("confirm", ids);
      const applied = new Set(result.appliedIds);
      applyLocal((current) =>
        current === undefined ? current : confirmTags(current, applied),
      );
      setSelected((current) => withoutIds(current, applied));
      toast(t.tags.selection.confirmed(applied.size, result.notApplicableIds.length));
      afterStale(result.notFoundIds);
      afterCommit(focusAfterBulkConfirm);
    } catch (failure) {
      toast(errorText(failure));
    } finally {
      setBulkPending(null);
    }
  }

  /**
   * focusAfterBulkConfirm はまとめての確定のあとのフォーカス先である。「Tentative
   * only」中に一覧が空になれば「Filter」、選択の行が残れば「Confirm」、仮のタグが
   * 残っていなければ「Merge into one tag…」、選択が無くなれば列の見出しの先頭の
   * チェックへ（ui-design.md「Bulk confirm」）。押せないボタンへは置かない。
   */
  function focusAfterBulkConfirm() {
    if (filtersRef.current.tentativeOnly && visibleRowsRef.current.length === 0) {
      focusFilterButton();
      return;
    }
    for (const candidate of [barConfirmRef.current, barMergeRef.current]) {
      if (candidate?.isConnected === true && !candidate.disabled) {
        candidate.focus();
        return;
      }
    }
    focusSelectAllOr(fallbackFocus);
  }

  function focusSelectAllOr(otherwise: () => void) {
    const header = selectAllRef.current;
    if (header?.isConnected === true && !header.disabled) header.focus();
    else otherwise();
  }

  function openBulkDialog(action: BulkTagAction) {
    if (bulkPending !== null || selected.size === 0) return;
    setBulkError(null);
    setBulkDialog({ action, ids: [...selected] });
  }

  /**
   * performBulk は確認の窓の「Reject」「Delete」である。処理した行を一覧から消して
   * 選択から外し、働かない種類だった行は選んだまま残す。失敗は窓の中に出し、窓と
   * 選択を残す（ui-design.md「Bulk reject and delete」）。
   */
  async function performBulk() {
    if (bulkDialog === null || bulkPending !== null) return;
    const { action, ids } = bulkDialog;
    const order = visibleRows;
    setBulkError(null);
    setBulkPending("dialog");
    try {
      const result = await batchTags(action, ids);
      const applied = new Set(result.appliedIds);
      removeRows(rows.filter((item) => applied.has(item.id)));
      setSelected((current) => withoutIds(current, applied));
      setBulkDialog(null);
      const skipped = result.notApplicableIds.length;
      toast(
        action === "reject"
          ? t.tags.selection.rejected(applied.size, skipped)
          : t.tags.selection.deleted(applied.size, skipped),
      );
      if (action === "reject") rejected.reload();
      afterStale(result.notFoundIds);
      // 消えた行の位置の次の行の「改名」、無ければ前の行。1 つも無ければ
      // 「Tentative only」が効いていれば「Filter」、効いていなければ先頭のチェック。
      const fallback = () => {
        if (filtersRef.current.tentativeOnly) focusFilterButton();
        else focusSelectAllOr(fallbackFocus);
      };
      const first = order.find((tag) => applied.has(tag.id));
      if (first === undefined) afterCommit(fallback);
      else focusAfterRemoval(order, first.id, applied, fallback);
    } catch (failure) {
      setBulkError(errorText(failure));
    } finally {
      setBulkPending(null);
    }
  }

  /** cancelBulk は確認の窓を何も変えずに閉じ、フォーカスを開いたボタンへ戻す。 */
  function cancelBulk() {
    if (bulkPending !== null || bulkDialog === null) return;
    const opener = bulkDialog.action === "reject" ? barRejectRef : barDeleteRef;
    setBulkDialog(null);
    setBulkError(null);
    afterCommit(() => (opener.current ?? barMergeRef.current)?.focus());
  }

  /**
   * rowHandlersRef は、行の操作の今の実体である。行へは `rowHandlers`（同じ
   * 関数を渡し続ける包み）を渡し、`React.memo` の行が操作のたびに描き直され
   * ないようにする。実体は描くたびに差し替える。
   */
  const rowHandlersRef = useRef<RowHandlers | null>(null);
  useLayoutEffect(() => {
    rowHandlersRef.current = {
      onStartRename: (target) => {
        // ほかの行の改名や作成が送信中は、新しく改名を始めない
        // （B2 と同じ規則）。行の「改名」自体も blockStart で
        // disabled だが、ここでも二重に守る。
        if (createPending || renamePending) return;
        setCreating(false);
        setRenameError(null);
        setRenaming(target);
        // 選んでいる行の改名を始めると、その行を選択から外す（ui-design.md
        // 「Row checkbox」）。
        setSelected((current) => withoutIds(current, new Set([target.id])));
      },
      onCancelRename: (target) => {
        if (renamePending) return;
        setRenaming(null);
        setRenameError(null);
        focusRow(target.id, "rename");
      },
      onSubmitRename: (target, name) => void submitRename(target, name),
      onOpenSynonyms: (target) => {
        synonymsOrderRef.current = visibleRows;
        setSynonymsTag(target);
      },
      onOpenMerge: (target) => setMerging({ sources: [target], fromSelection: false }),
      onDelete: (target) => {
        setDeleteError(null);
        setDeletingTag(target);
      },
      onConfirm: (target) => void submitConfirm(target),
      onReject: (target) => {
        setRejectError(null);
        setRejectingTag(target);
      },
      onDraftChange: () => setRenameError(null),
      onSelect: (target, next) => {
        if (target.id === renamingId) return;
        setSelected((current) => {
          if (current.has(target.id) === next) return current;
          const updated = new Set(current);
          if (next) updated.add(target.id);
          else updated.delete(target.id);
          return updated;
        });
      },
    };
  });
  const rowHandlers = useMemo<RowHandlers>(
    () => ({
      onStartRename: (tag) => rowHandlersRef.current?.onStartRename(tag),
      onCancelRename: (tag) => rowHandlersRef.current?.onCancelRename(tag),
      onSubmitRename: (tag, name) => rowHandlersRef.current?.onSubmitRename(tag, name),
      onOpenSynonyms: (tag) => rowHandlersRef.current?.onOpenSynonyms(tag),
      onOpenMerge: (tag) => rowHandlersRef.current?.onOpenMerge(tag),
      onDelete: (tag) => rowHandlersRef.current?.onDelete(tag),
      onConfirm: (tag) => rowHandlersRef.current?.onConfirm(tag),
      onReject: (tag) => rowHandlersRef.current?.onReject(tag),
      onDraftChange: () => rowHandlersRef.current?.onDraftChange(),
      onSelect: (tag, next) => rowHandlersRef.current?.onSelect(tag, next),
    }),
    [],
  );

  const { empty, showRows, staleList, tail, noTags } = tagListView({
    page,
    loadError,
    more,
    visibleCount: visibleRows.length,
    creating,
  });
  const appliedTentative = applied?.tentativeOnly ?? false;
  const sortDisabled = noTags;

  const selecting = onTagsTab && selected.size > 0;
  // 作成・改名の送信中はタブを切り替えない（押せなくし、矢印でも移らない）。
  const tabsLocked = createPending || renamePending;

  /**
   * 仮想化した行を表の中に描く。描かない行の高さは、行の間と前後に置く空の行で保つ
   * （行を absolute で重ねると、表の列の幅がそろわないため）。
   */
  const scrollStart = virtualizer.options.scrollMargin;
  const bodyRows: ReactNode[] = [];
  let cursor = scrollStart;
  for (const item of virtualItems) {
    const gap = item.start - cursor;
    if (gap > 0) {
      bodyRows.push(<SpacerRow key={`gap-${String(item.index)}`} height={gap} />);
    }
    const tag = visibleRows[item.index]!;
    bodyRows.push(
      <TagRow
        key={item.key}
        index={item.index}
        measureRef={virtualizer.measureElement}
        tag={tag}
        renaming={renamingId === tag.id}
        pending={renamingId === tag.id && renamePending}
        blockStart={createPending || (renamePending && renamingId !== tag.id)}
        error={renamingId === tag.id ? renameError : null}
        registerRefs={registerRefs}
        confirming={confirming.has(tag.id)}
        selected={selected.has(tag.id)}
        {...rowHandlers}
      />,
    );
    cursor = item.end;
  }
  const listEnd = scrollStart + virtualizer.getTotalSize();
  if (virtualItems.length > 0 && listEnd > cursor) {
    bodyRows.push(<SpacerRow key="gap-end" height={listEnd - cursor} />);
  }

  return (
    <AdminTablePage
      bandRef={bandRef}
      toolbar={
        onTagsTab && (
          <TopBarPortal>
            <TagToolbar
              query={search}
              onQueryCommit={commitQuery}
              searchRef={searchInputRef}
              searchDisabled={page !== undefined && page.totalAll === 0}
              tentativeOnly={tentativeOnly}
              onTentativeOnlyChange={changeTentativeOnly}
              unusedOnly={unusedOnly}
              onUnusedOnlyChange={changeUnusedOnly}
              onClearFilters={clearFilters}
              // 効いている間は、タグが 0 になっても押せる（絞り込みを外す唯一の手でもある）。
              filterDisabled={
                page === undefined || (noTags && !tentativeOnly && !unusedOnly)
              }
              filterRef={filterButtonRef}
              sort={sort}
              onSortChange={changeSort}
              sortDisabled={sortDisabled}
            />
          </TopBarPortal>
        )
      }
      header={
        // 選んでいる間は、見出しの行の代わりに選択バーを出す（ui-design.md「Selection bar」）。
        selecting ? (
          <TagSelectionBar
            count={selected.size}
            hasTentative={selection.tentative}
            hasConfirmed={selection.confirmed}
            overLimit={selectionOverLimit}
            busy={bulkPending !== null}
            confirming={bulkPending === "confirm"}
            onConfirm={() => void submitBulkConfirm()}
            onReject={() => openBulkDialog("reject")}
            onDelete={() => openBulkDialog("delete")}
            onMerge={openMergeSelected}
            onClear={clearSelection}
            confirmRef={barConfirmRef}
            mergeRef={barMergeRef}
            rejectRef={barRejectRef}
            deleteRef={barDeleteRef}
          />
        ) : (
          // 見出しの行（ui-design.md「Header」）。件数はライブラリの「N items」と同じ形で、
          // 読み込んだ行の数やページの区切りは出さない。
          <PageHeader
            title={t.tags.title}
            count={
              onTagsTab ? (
                <span role="status" aria-live="polite" className="whitespace-nowrap">
                  {countText}
                </span>
              ) : undefined
            }
            actions={
              onTagsTab ? (
                <Button
                  ref={createButtonRef}
                  size="sm"
                  onClick={openCreate}
                  disabled={
                    page === undefined || creating || createPending || renamePending
                  }
                >
                  <Plus aria-hidden="true" />
                  {t.tags.newTag}
                </Button>
              ) : undefined
            }
          />
        )
      }
      band={
        <>
          <TagsTabs
            idPrefix={tabsId}
            tab={tab}
            onChange={changeTab}
            tagsCount={page?.totalAll}
            rejectedCount={rejected.page?.total}
            locked={tabsLocked}
          />
          {onTagsTab && (
            <TagFilterChips
              tentativeOnly={tentativeOnly}
              unusedOnly={unusedOnly}
              onRemove={removeFilterChip}
              tentativeRef={tentativeChipRef}
              unusedRef={unusedChipRef}
            />
          )}
        </>
      }
    >
      {onTagsTab ? (
        <div
          ref={listBoxRef}
          id={tagsPanelId(tabsId, "tags")}
          role="tabpanel"
          aria-labelledby={tagsTabId(tabsId, "tags")}
          className="flex flex-col gap-3"
        >
          {staleList && loadError !== null && (
            <TagStaleList
              reason={loadError}
              pending={firstPending}
              onRetry={() => void reload()}
            />
          )}

          {page === undefined && loadError === null && (
            <LoadingState label={t.tags.loading} layout="table" count={6} />
          )}

          {page === undefined && loadError !== null && (
            <TagLoadFailed pending={firstPending} onRetry={() => void reload()} />
          )}

          {empty !== null && (
            <TagListEmptyState
              empty={empty}
              appliedSearch={appliedSearch}
              appliedTentative={appliedTentative}
              onCreate={openCreate}
              onShowAll={showAllFromFilter}
              onShowAllAndSearch={showAllFromFilterSearch}
              onClearSearch={clearSearch}
            />
          )}

          {showRows && (
            <DataTable label={t.tags.title} stickyHeaderTop={stuckBottom}>
              <TagListHead
                headRef={headRef}
                selectAllRef={selectAllRef}
                checked={selectAllState}
                selectableCount={selectableCount}
                overLimit={selectAllOverLimit}
                onToggle={toggleSelectAll}
              />
              {creating && (
                <TableBody>
                  <CreateTagRow
                    columnCount={tagColumnCount}
                    pending={createPending}
                    error={createError}
                    onCancel={() => {
                      // 送信中は、その応答が届くまで閉じない（B2 と同じ規則。
                      // CreateTagRow 自身の Esc・「キャンセル」も pending を見るが、
                      // ここでも二重に守る）。
                      if (createPending) return;
                      setCreating(false);
                      setCreateError(null);
                      // 作成を始めた「新しいタグ」へ戻す（「Tentative only」を
                      // 押していても。そちらは行が外れたときの行き先である）。
                      // creating が false になって押せるようになってから移す。
                      setTimeout(() => createButtonRef.current?.focus(), 0);
                    }}
                    onSubmit={(name) => void submitCreate(name)}
                    onDraftChange={() => setCreateError(null)}
                  />
                </TableBody>
              )}
              {/*
                行は文書の中の位置に並べ、描かない行の分は空の行で高さを保つ。フォーカスと
                Tab の受け渡しはこの tbody が持つ（ui-design.md「Keyboard across virtualized
                rows」）。
              */}
              <TableBody
                ref={listRef}
                aria-busy={tail.kind === "loading" ? true : undefined}
                onFocus={handleListFocus}
                onBlur={handleListBlur}
                onKeyDown={handleListKeyDown}
              >
                {bodyRows}
              </TableBody>
            </DataTable>
          )}
          {showRows && tail.kind === "loading" && <TagLoadingMore />}
          {showRows && tail.kind === "failed" && (
            <TagLoadMoreFailed reason={tail.error} onRetry={retryMore} />
          )}
          {showRows && tail.kind === "inconsistent" && (
            <TagListChanged onReload={reloadList} />
          )}
        </div>
      ) : (
        <div
          id={tagsPanelId(tabsId, "rejected")}
          role="tabpanel"
          aria-labelledby={tagsTabId(tabsId, "rejected")}
          className="flex flex-col gap-3"
        >
          <RejectedNames
            page={rejected.page}
            error={rejected.error}
            onRetry={rejected.reload}
            morePending={rejected.morePending}
            moreError={rejected.moreError}
            onLoadMore={rejected.loadMore}
            resetKey={rejected.epoch}
            onForget={rejected.forget}
            onFocusFallback={() =>
              document.getElementById(tagsTabId(tabsId, "rejected"))?.focus()
            }
          />
        </div>
      )}

      {bulkDialog !== null && (
        <BulkTagDialog
          action={bulkDialog.action}
          ids={bulkDialog.ids}
          pending={bulkPending === "dialog"}
          error={bulkError}
          onClose={cancelBulk}
          onSubmit={() => void performBulk()}
        />
      )}

      {deletingTag !== null && (
        <DeleteTagDialog
          tag={deletingTag}
          pending={deletePending}
          error={deleteError}
          onClose={cancelDelete}
          onDelete={() => void performDelete()}
        />
      )}

      {rejectingTag !== null && (
        <RejectTagDialog
          tag={rejectingTag}
          pending={rejectPending}
          error={rejectError}
          onClose={cancelReject}
          onReject={() => void performReject()}
        />
      )}

      {merging !== null && page !== undefined && (
        <MergeTagDialog
          sources={merging.sources}
          fromSelection={merging.fromSelection}
          onClose={cancelMerge}
          onMerged={performMerge}
          onStale={staleMerge}
        />
      )}

      {synonymsTag !== null && (
        <SynonymsDialog
          tag={synonymsTag}
          onClose={cancelSynonyms}
          onTagUpdated={updateSynonymsTag}
          onSynonymRemoved={removeSynonymFromTag}
          onStale={staleSynonyms}
        />
      )}
    </AdminTablePage>
  );
}

/** tagColumnCount は表の列の数（チェック・名前・本数・操作）。 */
const tagColumnCount = 4;

/** SpacerRow は描かない行の高さを保つ空の行である。読み上げない。 */
function SpacerRow({ height }: { height: number }) {
  return (
    <tr aria-hidden="true" style={{ height }}>
      <td colSpan={tagColumnCount} className="p-0" />
    </tr>
  );
}

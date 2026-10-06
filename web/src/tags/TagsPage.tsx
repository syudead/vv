import {
  defaultRangeExtractor,
  useWindowVirtualizer,
  type Range,
} from "@tanstack/react-virtual";
import { CircleDashed, Plus, SearchX, Tags as TagsIcon, VideoOff, X } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { flushSync } from "react-dom";

import { RequestFailed } from "../api/client";
import {
  batchTags,
  confirmTag,
  createTag,
  deleteTag,
  forgetRejectedTagName,
  listRejectedTagNamePage,
  listTagPage,
  maxTagBatch,
  rejectTag,
  renameTag,
  type RejectedTagNameList,
  tagPageLimit,
  type Tag,
} from "../api/tags";
import { errorText, formatNumber, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import {
  readTagListPreferences,
  writeTagListPreferences,
} from "../preferences/tagListPreferences";
import { AdminTablePage } from "../ui/patterns/admin-table-page";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { Checkbox } from "../ui/shadcn/checkbox";
import { TableBody, TableHead, TableHeader, TableRow } from "../ui/shadcn/table";
import { Tabs, TabsList, TabsTrigger } from "../ui/shadcn/tabs";
import { Toggle } from "../ui/shadcn/toggle";
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
  appendUniqueTags,
  confirmTags,
  insertionIndex,
  matchesTagQuery,
  removeTags,
  replaceTag,
  type TagPageRows,
  type TagRowsQuery,
} from "./tagPageRows";
import { tagFieldError, type TagFieldError } from "./tagNameField";
import { type TagListTab, useTagListCriteria } from "./tagListUrl";
import TagRow, { type TagRowRefs } from "./TagRow";
import {
  TagListChanged,
  TagLoadFailed,
  TagLoadingMore,
  TagLoadMoreFailed,
  TagStaleList,
} from "./TagListNotices";
import TagSelectionBar from "./TagSelectionBar";
import TopBarPortal from "../shell/TopBarPortal";
import TagToolbar from "./TagToolbar";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isTagNotTentative(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_tentative";
}

type FocusTarget = "rename" | "synonyms" | "name" | "menu";

/**
 * MoreState は一覧の末尾の続きの状態である（specs/036-tag-admin-scale/ui-design.md
 * 「Loading more」）。同時に 1 つだけで、`inconsistent` は続きの応答の `totalAll` が
 * 画面の値と違ったとき（research.md R-11）。
 */
type MoreState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "failed"; error: UiText }
  | { kind: "inconsistent" };

const moreIdle: MoreState = { kind: "idle" };

/**
 * ROW_ESTIMATE は、まだ描いていない行の高さの見積り（px）である。行（`py-2` と
 * `size-8` の操作、下の線 1px）の高さで、シノニムの行・改名の失敗の文言を持つ
 * 行は描いたあとに測った高さ（`measureElement`）で置き換わる。
 */
const ROW_ESTIMATE = 49;

/**
 * rejectedPageLimit は却下した名前の 1 ページの件数である（ui-design.md
 * 「Rejected names」の 100 件。research.md R-13）。
 */
const rejectedPageLimit = 100;

/** ROW_OVERSCAN は、表示域の前後に余分に描く行の数である。 */
const ROW_OVERSCAN = 8;

/** 行の中で Tab が止まる要素。 */
const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * isShown は、要素が描かれている（CSS で隠れていない）かである。行の操作は
 * タッチの端末と `sm` 未満で「Actions」1 つにまとめ、残りを CSS で隠すので、
 * フォーカスの行き先を選ぶときに見る。`checkVisibility` の無い環境（jsdom）は
 * 描かれているものとして扱う。
 */
function isShown(element: HTMLElement): boolean {
  return "checkVisibility" in element ? element.checkVisibility() : true;
}

function rowFocusables(row: Element): HTMLElement[] {
  return [...row.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
    (element) => element.tabIndex >= 0 && isShown(element),
  );
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
  /** page は読み込んだ行と件数である。先頭のページをまだ一度も受けていなければ undefined。 */
  const [page, setPage] = useState<TagPageRows | undefined>(undefined);
  /**
   * loadError は先頭のページの失敗である。一覧を持っていなければ失敗の表示、
   * 持っていれば帯の中の「Stale list」の箱になる（ui-design.md「Stale list」）。
   */
  const [loadError, setLoadError] = useState<UiText | null>(null);
  /** firstPending は先頭のページを待っているかである。 */
  const [firstPending, setFirstPending] = useState(false);
  const [more, setMore] = useState<MoreState>(moreIdle);
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
  /**
   * appliedSearch は、今の行を読んだときの検索の入力である。空の状態の文言はこれを
   * 出す。新しい検索の先頭のページを待つ間は前の結果が残るので、打ち直した入力を
   * 出すと、まだ読んでいない語に「一致が無い」と言ってしまう。
   */
  const [appliedSearch, setAppliedSearch] = useState("");

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
  const selectAllLimitId = useId();
  const tabsId = useId();
  const barConfirmRef = useRef<HTMLButtonElement | null>(null);
  const barMergeRef = useRef<HTMLButtonElement | null>(null);
  const barRejectRef = useRef<HTMLButtonElement | null>(null);
  const barDeleteRef = useRef<HTMLButtonElement | null>(null);

  /**
   * 却下した名前は先頭から読み込んだ分（`items`）と、全部の数（`total`。入口の件数）、
   * 続きのカーソル（`nextCursor`）で持つ（specs/036-tag-admin-scale/data-model.md §4、
   * research.md R-13）。undefined の間は読み込み中か、先頭のページの読み込みの失敗。
   */
  const [rejectedPage, setRejectedPage] = useState<RejectedTagNameList | undefined>(
    undefined,
  );
  const [rejectedError, setRejectedError] = useState<UiText | null>(null);
  /** 続きの読み込みの送信中と、その失敗。 */
  const [rejectedMorePending, setRejectedMorePending] = useState(false);
  const [rejectedMoreError, setRejectedMoreError] = useState<UiText | null>(null);
  /** 先頭のページを受けるたびに 1 増える。窓が開いていればスクロール位置を先頭へ戻す。 */
  const [rejectedEpoch, setRejectedEpoch] = useState(0);
  const rejectedGeneration = useRef(0);
  /** 取り直しの応答を待っている世代。待っていなければ null。 */
  const rejectedInFlight = useRef<number | null>(null);
  /** 続きの要求を送っている間 true（同時に 1 つだけ送る）。 */
  const rejectedMoreInFlight = useRef(false);
  /**
   * 先頭のページを受けたあとに × で外した名前。外す前に送った続きの応答に
   * 載っていても並びに戻さない。
   */
  const rejectedForgotten = useRef(new Set<string>());

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  /** filterButtonRef はトップバーの「Filter」のボタンである。 */
  const filterButtonRef = useRef<HTMLButtonElement | null>(null);
  /** chipRefs は見出しの下の絞り込みのチップ（外したあとのフォーカス先）である。 */
  const tentativeChipRef = useRef<HTMLButtonElement | null>(null);
  const unusedChipRef = useRef<HTMLButtonElement | null>(null);
  const rowRefs = useRef(new Map<number, TagRowRefs>());
  const listRef = useRef<HTMLTableSectionElement | null>(null);
  /** listBoxRef は帯の下の一覧の箱（作成の行・行・空の状態を入れる）である。 */
  const listBoxRef = useRef<HTMLDivElement | null>(null);
  /**
   * pinnedRowId は、見えている範囲の外でも描き続ける行である。フォーカスを
   * 持つ行（行から開いたメニューにあるときも含む）と、これからフォーカスを移す
   * 行を指す。フォーカスが一覧の外へ出たら外す（`handleListBlur`。
   * specs/036-tag-admin-scale/ui-design.md「Keyboard across virtualized rows」）。
   */
  const [pinnedRowId, setPinnedRowId] = useState<number | null>(null);
  /**
   * focusInListRef は、フォーカスが一覧（React の木での一覧で、行から開いた
   * メニューを含む）の中にあるかである。`handleListBlur` が見る。
   */
  const focusInListRef = useRef(false);
  /** afterCommitQueue は `afterCommit` が描き終えるのを待たせている処理である。 */
  const afterCommitQueue = useRef<(() => void)[]>([]);
  const [afterCommitTick, setAfterCommitTick] = useState(0);
  useEffect(() => {
    if (afterCommitQueue.current.length === 0) return;
    const queued = afterCommitQueue.current;
    afterCommitQueue.current = [];
    setTimeout(() => {
      for (const fn of queued) fn();
    }, 0);
  }, [afterCommitTick]);
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

  const registerRefs = useCallback((id: number, refs: Partial<TagRowRefs>) => {
    const current = rowRefs.current.get(id) ?? {
      nameLink: null,
      renameButton: null,
      synonymsButton: null,
      menuButton: null,
      actionsButton: null,
    };
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

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
   * releasePinIfFocusOutside は、フォーカスが一覧の外にあれば、行を描き続ける
   * のをやめる（`pinnedRowId`）。フォーカスを持つ要素は外さない。
   */
  function releasePinIfFocusOutside() {
    if (focusInListRef.current) return;
    if (listRef.current?.contains(document.activeElement)) return;
    setPinnedRowId(null);
  }

  /**
   * afterCommit は、いま置いた状態（応答で変えたタグの一覧や送信中の印）を
   * React が描き終えてから、さらに setTimeout(0) で1呼吸置いて fn を呼ぶ
   * （settings/SettingsPage.tsx の focusFolderAction と同じ1呼吸）。
   * setTimeout(0) だけでは、応答のあとの再描画より先に走ることがあり、
   * 新しい行がまだ一覧に無い、送信中で「新しいタグ」が押せない、といった
   * 古い画面でフォーカス先を決めてしまう。
   */
  function afterCommit(fn: () => void) {
    afterCommitQueue.current.push(fn);
    setAfterCommitTick((tick) => tick + 1);
  }

  /**
   * renderRow は、一覧にあるのに描いていない（見えている範囲の外の）行を
   * 描かせる。描く範囲にその行を足して同期で描き直す（`pinnedRowId`）。
   * 描いていなかった行なら一覧での位置を返し、もう描いていた行や一覧に無い
   * 行なら -1 を返す。
   */
  function renderRow(id: number): number {
    const index = visibleRowsRef.current.findIndex((tag) => tag.id === id);
    if (index === -1) return -1;
    const drawn = listRef.current?.querySelector(`[data-tag-id="${String(id)}"]`);
    if (drawn !== null && drawn !== undefined) return -1;
    flushSync(() => setPinnedRowId(id));
    return index;
  }

  /**
   * revealRow は、描いていない行を描かせてその行の位置へスクロールする。
   * フォーカスを見えている範囲の外の行へ移す前に呼ぶ（行が描かれていなければ
   * 移せない）。もう描いている行は、フォーカスを移すとブラウザが表示域へ
   * 寄せる。
   */
  function revealRow(id: number) {
    const index = renderRow(id);
    if (index !== -1) virtualizer.scrollToIndex(index, { align: "auto" });
  }

  /**
   * focusRow はタグの行のフォーカス先へ移す。その行がもう無ければ
   * `fallbackFocus` へ移す。行の差し替えが DOM に反映されたあとで移す必要が
   * あるので、同時に置いた状態を描き終えてから1呼吸置く（`afterCommit`）。
   * 行が描かれていなければ、先にその行の位置へスクロールする（`revealRow`）。
   */
  function focusRow(id: number, part: FocusTarget) {
    afterCommit(() => {
      revealRow(id);
      const refs = rowRefs.current.get(id);
      const preferred =
        part === "rename"
          ? refs?.renameButton
          : part === "synonyms"
            ? refs?.synonymsButton
            : part === "menu"
              ? refs?.menuButton
              : refs?.nameLink;
      const target = rowActionTarget(refs, preferred);
      if (target === null || target === undefined || !target.isConnected) {
        fallbackFocus();
      } else target.focus();
    });
  }

  /**
   * focusAfterRemoval は、行が一覧から消えた（削除・却下、tag_not_found の
   * 取り直しで消えた、または「Tentative only」の絞り込みから外れた）あとの
   * フォーカス先を決める。次の行の「改名」、無ければ前の行、1つも無ければ
   * `fallbackFocus`（ui-design.md「Merge and delete」、031 の「Toolbar」）。
   * order は消える前の（絞り込み後の）並びで、`alsoGone` は同時に一覧から
   * 外れるほかの行（統合先が確定になって絞り込みから外れるときなど）である。
   *
   * 候補は、移す時点でまだ一覧にあって押せる「改名」に限る。ほかの行の確定が
   * 並行していると、order を控えたあとにその行も外れていたり、送信中で
   * 「改名」が disabled だったりするので、それを飛ばして次の行へ進む。
   * 候補が見えている範囲の外で描かれていなければ、描かせてから確かめ、移す
   * 前にその行の位置へスクロールする。
   */
  function focusAfterRemoval(
    order: readonly Tag[],
    removedId: number,
    alsoGone: ReadonlySet<number> = new Set(),
    fallback: () => void = fallbackFocus,
  ) {
    const index = order.findIndex((tag) => tag.id === removedId);
    const stays = (tag: Tag) => tag.id !== removedId && !alsoGone.has(tag.id);
    const candidates = [
      ...order.slice(index + 1).filter(stays),
      ...order.slice(0, Math.max(index, 0)).reverse().filter(stays),
    ];
    afterCommit(() => {
      const listed = new Set(visibleRowsRef.current.map((tag) => tag.id));
      for (const tag of candidates) {
        if (!listed.has(tag.id)) continue;
        const index = renderRow(tag.id);
        const refs = rowRefs.current.get(tag.id);
        const button = rowActionTarget(refs, refs?.renameButton);
        if (button?.isConnected === true && !(button as HTMLButtonElement).disabled) {
          if (index !== -1) virtualizer.scrollToIndex(index, { align: "auto" });
          button.focus();
          return;
        }
      }
      fallback();
    });
  }

  /**
   * rowActionTarget は、行の操作へフォーカスを移すときの実際の行き先である。
   * タッチの端末と `sm` 未満では行の `IconButton` が隠れて「Actions」1 つに
   * まとまるので、隠れている操作の代わりに「Actions」を指す（ui-design.md
   * 「Actions on touch and narrow widths」）。名前のリンクは隠れない。
   */
  function rowActionTarget(
    refs: TagRowRefs | undefined,
    preferred: HTMLElement | null | undefined,
  ): HTMLElement | null | undefined {
    if (preferred === null || preferred === undefined) return preferred;
    if (preferred === refs?.nameLink || !preferred.isConnected) return preferred;
    return isShown(preferred) ? preferred : refs?.actionsButton;
  }

  /**
   * reloadRejectedNames は却下した名前の先頭の 1 ページを取り直す。却下・作成・
   * 改名・シノニムの追加のあと（どれも一覧を変えうる。要件 15、受け入れ条件 14）と、
   * 画面を開いたときに呼ぶ。読み込んだ続きは捨て、先頭の 1 ページに戻す
   * （ui-design.md「Rejected names」）。追い越された古い取得の結果（続きも含む）は
   * 捨てる。取り直しの失敗は、最初の読み込みの失敗と同じ見え方（件数を出さず、
   * 「Couldn't load the rejected names」と Retry）にする。古い一覧を黙って残すと、
   * 却下や作成で変わったはずの並びを正しいものとして見せ続けてしまう。
   */
  const reloadRejectedNames = useCallback(() => {
    rejectedGeneration.current += 1;
    const generation = rejectedGeneration.current;
    rejectedInFlight.current = generation;
    rejectedMoreInFlight.current = false;
    setRejectedError(null);
    setRejectedMorePending(false);
    setRejectedMoreError(null);
    listRejectedTagNamePage(undefined, rejectedPageLimit)
      .then((page) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedInFlight.current = null;
        rejectedForgotten.current = new Set();
        setRejectedPage(page);
        setRejectedEpoch((epoch) => epoch + 1);
      })
      .catch((failure: unknown) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedInFlight.current = null;
        setRejectedPage(undefined);
        setRejectedError(errorText(failure));
      });
  }, []);

  /**
   * loadMoreRejectedNames は窓の中身を末尾までスクロールしたときに、続きの
   * 1 ページを読んで並びの末尾に足す。同時に 1 つだけ送る。失敗しても読み込んだ
   * 名前は残し、Retry は同じカーソルで読み直す。入口の件数は先頭のページの
   * `total` から外した数を引いたままにする（続きの応答で上書きすると、送ったあとの
   * 取り外しの分がずれる）。
   */
  function loadMoreRejectedNames() {
    const cursor = rejectedPage?.nextCursor;
    if (cursor === undefined || rejectedMoreInFlight.current) return;
    if (rejectedInFlight.current !== null) return;
    const generation = rejectedGeneration.current;
    rejectedMoreInFlight.current = true;
    setRejectedMorePending(true);
    setRejectedMoreError(null);
    listRejectedTagNamePage(cursor, rejectedPageLimit)
      .then((next) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedMoreInFlight.current = false;
        setRejectedMorePending(false);
        setRejectedPage((current) => {
          if (current === undefined) return current;
          const known = new Set(current.items);
          const added = next.items.filter(
            (name) => !known.has(name) && !rejectedForgotten.current.has(name),
          );
          return {
            items: [...current.items, ...added],
            total: current.total,
            ...(next.nextCursor === undefined ? {} : { nextCursor: next.nextCursor }),
          };
        });
      })
      .catch((failure: unknown) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedMoreInFlight.current = false;
        setRejectedMorePending(false);
        setRejectedMoreError(errorText(failure));
      });
  }

  /**
   * forgetRejectedName は × の取り外しである。`204` でそのチップを消し、入口の
   * 件数を 1 減らす（一覧は取り直さない）。取り外しの送信中に先頭のページの
   * 取り直しが重なったとき（送る前から待っていた・送信中に始まった）は、その
   * 応答が取り外しの前か後かが分からない。外した名前が先頭のページの外にあると、
   * 局所の 1 減らしもできない。そのため、待っている応答は捨て、取り外しのあとで
   * 取り直す。
   */
  async function forgetRejectedName(name: string) {
    const generation = rejectedGeneration.current;
    const refreshing = rejectedInFlight.current !== null;
    await forgetRejectedTagName(name);
    const overlapped =
      refreshing ||
      generation !== rejectedGeneration.current ||
      rejectedInFlight.current !== null;
    rejectedForgotten.current.add(name);
    setRejectedPage((current) => {
      if (current === undefined || !current.items.includes(name)) return current;
      return {
        ...current,
        items: current.items.filter((item) => item !== name),
        total: Math.max(current.total - 1, 0),
      };
    });
    if (overlapped) reloadRejectedNames();
  }

  /**
   * pageRef・moreRef・firstPendingRef・loadErrorRef は、要求の応答のあとや
   * 続きのきっかけで今の状態を読むための控えである。描くたびに差し替える。
   */
  const pageRef = useRef<TagPageRows | undefined>(page);
  const moreRef = useRef<MoreState>(more);
  const firstPendingRef = useRef(firstPending);
  const loadErrorRef = useRef<UiText | null>(loadError);
  useLayoutEffect(() => {
    pageRef.current = page;
    moreRef.current = more;
    firstPendingRef.current = firstPending;
    loadErrorRef.current = loadError;
  });

  /**
   * generationRef は先頭のページの要求の通し番号である。条件を変えるたびに進め、
   * 古い条件の応答（先頭のページも続きも）を捨てる（research.md R-11）。
   */
  const generationRef = useRef(0);
  /**
   * mutationRef は、操作の結果を読み込んだ行へ局所で反映した回数である（`applyLocal`）。
   * 要求を送ったあとに反映があれば、その応答は反映した変更の前の一覧かもしれないので、
   * 行と件数に入れずに同じ条件で読み直す（先頭のページは先頭から、続きは同じカーソルで）。
   * 入れると、消したタグが戻る・件数が操作の前に戻るなど、局所の反映が巻き戻る。
   */
  const mutationRef = useRef(0);
  const firstAbortRef = useRef<AbortController | null>(null);
  const moreAbortRef = useRef<AbortController | null>(null);
  /** queryRef は最後に読み直しを始めた条件である。「Retry」「Reload」が同じ条件で読む。 */
  const queryRef = useRef<TagRowsQuery & { search: string }>({
    query: "",
    search: "",
    tentativeOnly: false,
    unusedOnly: false,
    sort: "name",
  });

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
  const scrollListToTopRef = useRef(scrollListToTop);
  useLayoutEffect(() => {
    scrollListToTopRef.current = scrollListToTop;
  });

  /**
   * loadFirst は今の条件（`queryRef`）で先頭のページを読み直す。進行中の要求
   * （先頭のページと続き）を打ち切り、世代を進めて古い応答を捨てる。届くまで前の
   * 行と件数を残し（`Skeleton` に戻さない）、届いたら差し替えて一覧の先頭へ戻す。
   * 失敗したら、一覧を持っていなければ失敗の表示、持っていれば「Stale list」の箱に
   * する（data-model.md §4「条件」「読み込み失敗」）。選択はここでは変えない
   * （条件の変更と「Reload」は呼ぶ側が空にし、`notFoundIds` のあとの取り直しは
   * 読み直した行に無い id だけが外れる）。
   */
  const loadFirst = useCallback((): Promise<void> => {
    const run = (): Promise<void> => {
      firstAbortRef.current?.abort();
      moreAbortRef.current?.abort();
      generationRef.current += 1;
      const generation = generationRef.current;
      const controller = new AbortController();
      firstAbortRef.current = controller;
      const { search: searched, ...query } = queryRef.current;
      const mutation = mutationRef.current;
      firstPendingRef.current = true;
      setFirstPending(true);
      moreRef.current = moreIdle;
      setMore(moreIdle);
      return listTagPage(
        {
          q: query.query === "" ? undefined : searched.trim(),
          tentative: query.tentativeOnly,
          unused: query.unusedOnly,
          sort: query.sort,
          limit: tagPageLimit,
        },
        controller.signal,
      ).then(
        (result) => {
          if (generation !== generationRef.current) return;
          // 送ったあとに局所の反映があった。応答はその変更を映していないかもしれない。
          if (mutation !== mutationRef.current) return run();
          setAppliedSearch(searched);
          setPage({
            rows: result.items,
            total: result.total,
            totalAll: result.totalAll,
            nextCursor: result.nextCursor,
            boundary: result.nextCursor === undefined ? undefined : result.items.at(-1),
            query,
          });
          setLoadError(null);
          setFirstPending(false);
          scrollListToTopRef.current();
        },
        (failure: unknown) => {
          if (generation !== generationRef.current) return;
          setFirstPending(false);
          setLoadError(errorText(failure));
        },
      );
    };
    return run();
  }, []);

  /**
   * loadMore は続きの 1 ページを `nextCursor` で読み、`id` の重複を捨てて末尾に足す。
   * 続きを読んでいる間・失敗や食い違いを出している間・先頭のページを待つ間・
   * 「Stale list」の間は読まない（同時に 1 つだけ。持っているカーソルが前の条件の
   * ものかもしれない）。応答の `totalAll` が画面の値と違えば、行は残して続きを止め、
   * 「一覧が変わった」を出す（research.md R-11）。
   */
  const loadMore = useCallback(() => {
    const run = () => {
      const current = pageRef.current;
      if (current?.nextCursor === undefined) return;
      if (moreRef.current.kind !== "idle") return;
      if (firstPendingRef.current || loadErrorRef.current !== null) return;
      const generation = generationRef.current;
      const mutation = mutationRef.current;
      const controller = new AbortController();
      moreAbortRef.current = controller;
      moreRef.current = { kind: "loading" };
      setMore(moreRef.current);
      const { query } = current;
      listTagPage(
        {
          q: query.query === "" ? undefined : queryRef.current.search.trim(),
          tentative: query.tentativeOnly,
          unused: query.unusedOnly,
          sort: query.sort,
          cursor: current.nextCursor,
          limit: tagPageLimit,
        },
        controller.signal,
      ).then(
        (result) => {
          if (generation !== generationRef.current) return;
          if (mutation !== mutationRef.current) {
            // 送ったあとに局所の反映があった。応答の行と件数はその変更の前かもしれない
            // ので捨て、同じカーソルで読み直す（keyset なので、境より後ろの今の行が返る）。
            moreRef.current = moreIdle;
            setMore(moreIdle);
            run();
            return;
          }
          if (result.totalAll !== pageRef.current?.totalAll) {
            moreRef.current = { kind: "inconsistent" };
            setMore(moreRef.current);
            return;
          }
          setPage((latest) =>
            latest === undefined
              ? latest
              : {
                  ...latest,
                  rows: appendUniqueTags(latest.rows, result.items),
                  total: result.total,
                  nextCursor: result.nextCursor,
                  boundary:
                    result.nextCursor === undefined ? undefined : result.items.at(-1),
                },
          );
          moreRef.current = moreIdle;
          setMore(moreIdle);
        },
        (failure: unknown) => {
          if (generation !== generationRef.current) return;
          moreRef.current = { kind: "failed", error: errorText(failure) };
          setMore(moreRef.current);
        },
      );
    };
    run();
  }, []);

  /** retryMore は続きの失敗の「Retry」で、同じカーソルで読み直す。 */
  function retryMore() {
    moreRef.current = moreIdle;
    setMore(moreIdle);
    loadMore();
  }

  /**
   * reloadList は「一覧が変わった」の「Reload」で、選択を空にして先頭から読み直す
   * （ui-design.md「Loading more」）。
   */
  function reloadList() {
    setSelected(new Set());
    void loadFirst();
  }

  /** reload は、もう無いタグに当たったときなどに先頭から取り直す。選択は残る。 */
  const reload = loadFirst;

  useEffect(() => {
    reloadRejectedNames();
  }, [reloadRejectedNames]);

  useEffect(
    () => () => {
      firstAbortRef.current?.abort();
      moreAbortRef.current?.abort();
    },
    [],
  );

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
    queryRef.current = {
      query: normalizedQuery,
      search: searchRef.current,
      tentativeOnly,
      unusedOnly,
      sort,
    };
    setSelected((current) => (current.size === 0 ? current : new Set()));
    void loadFirst();
  }, [normalizedQuery, tentativeOnly, unusedOnly, sort, loadFirst]);

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

  const visibleRowsRef = useRef<readonly Tag[]>(visibleRows);
  useLayoutEffect(() => {
    visibleRowsRef.current = visibleRows;
  }, [visibleRows]);

  // 選択は読み込んだ行の部分集合に保つ。操作や取り直しで行から消えた id と、
  // 改名中の行を外す（specs/036-tag-admin-scale/data-model.md §4「選択」）。条件の
  // 変更では、読み直しを始めるときに空にする。
  useLayoutEffect(() => {
    setSelected((current) => {
      if (current.size === 0) return current;
      const loaded = new Set(rows.map((tag) => tag.id));
      const next = new Set<number>();
      for (const id of current) {
        if (loaded.has(id) && id !== renamingId) next.add(id);
      }
      return next.size === current.size ? current : next;
    });
  }, [rows, renamingId]);

  /**
   * selectableCount は「読み込んだものをすべて選ぶ」の対象の数である。改名中の行は
   * 入らない（ui-design.md「Column header」）。読み込んでいないタグは選ばない（要件 10）。
   */
  const selectableCount =
    rows.length -
    (renamingId !== null && rows.some((tag) => tag.id === renamingId) ? 1 : 0);
  /**
   * 上限 `maxTagBatch` は送る id の数に掛かる（research.md R-4）。先頭のチェックは
   * 読み込んだ行の数で、まとめての操作は選んだ数で止める（ui-design.md「Column header」
   * 「Enabled and disabled」）。
   */
  const selectAllOverLimit = rows.length > maxTagBatch;
  const selectionOverLimit = selected.size > maxTagBatch;
  const selection = useMemo(() => {
    let tentative = false;
    let confirmed = false;
    if (selected.size > 0) {
      for (const tag of rows) {
        if (!selected.has(tag.id)) continue;
        if (tag.tentative) tentative = true;
        else confirmed = true;
        if (tentative && confirmed) break;
      }
    }
    return { tentative, confirmed };
  }, [selected, rows]);
  const selectAllState: boolean | "indeterminate" =
    selected.size === 0
      ? false
      : selected.size >= selectableCount
        ? true
        : "indeterminate";

  // 一覧は表示域と前後の少数の行だけを描く（specs/036-tag-admin-scale/research.md
  // R-2）。スクロールの持ち主は文書で、行の高さは描いた要素を測る。
  const pinnedIndex = useMemo(
    () =>
      pinnedRowId === null ? -1 : visibleRows.findIndex((tag) => tag.id === pinnedRowId),
    [visibleRows, pinnedRowId],
  );
  /**
   * renamingIndex は改名中の行の位置である。改名中の行は、フォーカスが一覧の
   * 外へ出ても（並び順のメニューを開くなど）描き続ける。外すと `TagRow` が
   * 外れて打っている途中の名前を失う（並び順・絞り込みを変えても改名中の行を
   * 消さない。specs/036-tag-admin-scale/plan.md）。
   */
  const renamingIndex = useMemo(
    () =>
      renamingId === null ? -1 : visibleRows.findIndex((tag) => tag.id === renamingId),
    [visibleRows, renamingId],
  );
  const rangeExtractor = useCallback(
    (range: Range) => {
      const indexes = defaultRangeExtractor(range);
      const extra = [pinnedIndex, renamingIndex].filter(
        (index, i, all) =>
          index >= 0 &&
          index < range.count &&
          !indexes.includes(index) &&
          all.indexOf(index) === i,
      );
      if (extra.length === 0) return indexes;
      return [...indexes, ...extra].sort((a, b) => a - b);
    },
    [pinnedIndex, renamingIndex],
  );
  const getItemKey = useCallback(
    (index: number) => visibleRows[index]!.id,
    [visibleRows],
  );
  const virtualizer = useWindowVirtualizer({
    count: visibleRows.length,
    estimateSize: () => ROW_ESTIMATE,
    overscan: ROW_OVERSCAN,
    scrollMargin,
    scrollPaddingStart: rowsTop,
    rangeExtractor,
    getItemKey,
  });
  const virtualItems = virtualizer.getVirtualItems();

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
  }, [hasList]);

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
   * handleListFocus は、フォーカスを持った行を描き続ける行にする。スクロール
   * で画面の外へ出ても、その行は外れず、フォーカスが `body` へ落ちない。
   */
  function handleListFocus(event: FocusEvent<HTMLTableSectionElement>) {
    focusInListRef.current = true;
    const row = (event.target as Element).closest<HTMLElement>("[data-tag-id]");
    if (row === null || !event.currentTarget.contains(row)) return;
    setPinnedRowId(Number(row.dataset.tagId));
  }

  /**
   * handleListBlur は、フォーカスが一覧の外へ出たら行を描き続けるのをやめる。
   * 残すと、表示域から遠い行が Tab の順に残り、ツールバーなどからの Tab が
   * その行へ飛んで表示域が動く。フォーカスが決まったあとで見て、行の中や
   * 行の間の移動、行から開いたメニュー（React の木では一覧の中なので focus が
   * 一覧まで届く）への移動では外さない。ウィンドウ自体がフォーカスを失った
   * ときは `document.activeElement` が一覧の中に残るので外さない。フォーカスを
   * 持つ要素は外さない。
   */
  function handleListBlur() {
    focusInListRef.current = false;
    setTimeout(releasePinIfFocusOutside, 0);
  }

  /**
   * handleListKeyDown は、描いている範囲の端の Tab を次の（Shift+Tab は前の）
   * 行へ渡す。描いていない行は DOM に無いので、既定の Tab では一覧の外へ
   * 飛んでしまう。全件の最後の行の Tab、最初の行の Shift+Tab は既定のまま
   * 一覧の外へ進める（ui-design.md「Keyboard across virtualized rows」）。
   */
  function handleListKeyDown(event: KeyboardEvent<HTMLTableSectionElement>) {
    if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) return;
    if (event.defaultPrevented) return;
    const list = event.currentTarget;
    const target = event.target as HTMLElement;
    const row = target.closest<HTMLElement>("[data-index]");
    if (row === null || !list.contains(row)) return;
    const focusables = rowFocusables(row);
    const edge = event.shiftKey ? focusables[0] : focusables[focusables.length - 1];
    if (target !== edge) return;
    const nextIndex = Number(row.dataset.index) + (event.shiftKey ? -1 : 1);
    const next = visibleRows[nextIndex];
    if (next === undefined) return;
    if (list.querySelector(`[data-index="${String(nextIndex)}"]`) !== null) return;
    event.preventDefault();
    revealRow(next.id);
    const nextRow = list.querySelector(`[data-index="${String(nextIndex)}"]`);
    if (nextRow === null) return;
    const nextFocusables = rowFocusables(nextRow);
    const into = event.shiftKey
      ? nextFocusables[nextFocusables.length - 1]
      : nextFocusables[0];
    into?.focus();
  }

  /**
   * applied は読み込んだ行の条件である。件数の行と空の状態は、入力中の条件ではなく
   * 届いた行の条件で決める（先頭のページを待つ間は前の行と件数を残す）。
   */
  const applied = page?.query;
  const searching = applied !== undefined && applied.query !== "";
  /** totalAll は全部のタグの数（読み込んでいないタグを含む）。 */
  const totalAll = page?.totalAll ?? 0;
  // 件数は応答の total・totalAll で出す。読み込んだ行の数（ページの区切り）は出さない。
  // 差し込んで残した改名中の行は数えない（ui-design.md「Header」）。
  const countText =
    page === undefined
      ? t.tags.loading
      : searching || page.query.tentativeOnly || page.query.unusedOnly
        ? t.tags.filteredCount(page.total, page.totalAll)
        : t.tags.count(page.totalAll);

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
   * applyLocal は、操作の結果を読み込んだ行と件数へ局所で反映する。反映の前に送った
   * 先頭のページ・続きの応答を捨てて読み直させるため、`mutationRef` を進める。
   */
  function applyLocal(
    update: (current: TagPageRows | undefined) => TagPageRows | undefined,
  ) {
    mutationRef.current += 1;
    setPage(update);
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
      reloadRejectedNames();
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
      reloadRejectedNames();
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
      reloadRejectedNames();
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
    reloadRejectedNames();
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

  /** withoutIds は選択から ids を外した集合を返す。 */
  function withoutIds(current: ReadonlySet<number>, ids: ReadonlySet<number>) {
    const next = new Set(current);
    for (const id of ids) next.delete(id);
    return next;
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
      if (action === "reject") reloadRejectedNames();
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

  // 空の状態は、届いた行の条件（`applied`）で選ぶ。条件を変えて先頭のページを待つ
  // 間は、前の状態をそのまま残す（ui-design.md「States」）。
  const appliedTentative = applied?.tentativeOnly ?? false;
  const appliedUnused = applied?.unusedOnly ?? false;
  const showEmptyTags =
    page !== undefined &&
    page.totalAll === 0 &&
    !creating &&
    !appliedTentative &&
    !appliedUnused;
  /**
   * moreToShow は、読み込んだ行が無くても続きがあるかである。操作で行が 1 つも
   * 残らなかったときは、空の状態を出さずに末尾の続きの状態を出す（ui-design.md
   * 「Loading more」）。
   */
  const moreToShow = page?.nextCursor !== undefined && page.total > 0;
  const nothingShown =
    page !== undefined &&
    visibleRows.length === 0 &&
    !creating &&
    !showEmptyTags &&
    !moreToShow;
  const showNoUnused = nothingShown && appliedUnused && !searching;
  const showNoUnusedMatch = nothingShown && appliedUnused && searching;
  const showNoTentative =
    nothingShown && !appliedUnused && appliedTentative && !searching;
  const showNoTentativeMatch =
    nothingShown && !appliedUnused && appliedTentative && searching;
  const showNoMatch = nothingShown && !appliedTentative && !appliedUnused;
  const showRows =
    page !== undefined &&
    (visibleRows.length > 0 || creating || moreToShow) &&
    !showEmptyTags;
  /**
   * 先頭のページをまだ一度も受けていない間・一覧を持たないまま失敗したときは
   * 何も押せない。押していない絞り込みと検索・並び順は、タグが 1 つも無いときも
   * 押せない（ui-design.md「Top bar」）。
   */
  const noTags = page === undefined || page.totalAll === 0;
  const sortDisabled = noTags;
  /** staleList は、一覧を持ったまま先頭のページを読めなかったか（「Stale list」）。 */
  const staleList = page !== undefined && loadError !== null;
  /** tail は一覧の末尾の続きの状態で、「Stale list」の間は出さない。 */
  const tail = page === undefined || staleList ? moreIdle : more;

  const tagsPanelId = `${tabsId}-panel-tags`;
  const rejectedPanelId = `${tabsId}-panel-rejected`;
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
          <Tabs
            value={tab}
            onValueChange={(next) => {
              changeTab(next as TagListTab);
            }}
          >
            <TabsList aria-label={t.tags.tabs.label}>
              <TabsTrigger
                value="tags"
                id={tabId(tabsId, "tags")}
                aria-controls={tagsPanelId}
                disabled={tabsLocked && tab !== "tags"}
              >
                {t.tags.tabs.tags}
                {page !== undefined && (
                  <span className="font-normal text-muted-foreground tabular-nums">
                    {formatNumber(page.totalAll)}
                  </span>
                )}
              </TabsTrigger>
              <TabsTrigger
                value="rejected"
                id={tabId(tabsId, "rejected")}
                aria-controls={rejectedPanelId}
                disabled={tabsLocked && tab !== "rejected"}
              >
                {t.tags.rejectedNames.heading}
                {rejectedPage !== undefined && (
                  <span className="font-normal text-muted-foreground tabular-nums">
                    {formatNumber(rejectedPage.total)}
                  </span>
                )}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          {onTagsTab && (tentativeOnly || unusedOnly) && (
            // 効いている絞り込みのチップ（ui-design.md「Active filters」）。押された
            // Toggle で、押し戻すと外れる（components.md「Toggle and ToggleGroup」）。
            <ul
              aria-label={t.tags.activeFilters.label}
              className="flex flex-wrap items-center gap-2"
            >
              {tentativeOnly && (
                <li>
                  <Toggle
                    ref={tentativeChipRef}
                    variant="outline"
                    size="sm"
                    pressed
                    onPressedChange={() => removeFilterChip("tentative")}
                    aria-label={t.tags.activeFilters.remove(t.tags.tentativeOnly)}
                    title={t.tags.activeFilters.remove(t.tags.tentativeOnly)}
                  >
                    <CircleDashed aria-hidden="true" />
                    {t.tags.tentativeOnly}
                    <X aria-hidden="true" />
                  </Toggle>
                </li>
              )}
              {unusedOnly && (
                <li>
                  <Toggle
                    ref={unusedChipRef}
                    variant="outline"
                    size="sm"
                    pressed
                    onPressedChange={() => removeFilterChip("unused")}
                    aria-label={t.tags.activeFilters.remove(t.tags.unusedOnly)}
                    title={t.tags.activeFilters.remove(t.tags.unusedOnly)}
                  >
                    <VideoOff aria-hidden="true" />
                    {t.tags.unusedOnly}
                    <X aria-hidden="true" />
                  </Toggle>
                </li>
              )}
            </ul>
          )}
        </>
      }
    >
      {onTagsTab ? (
        <div
          ref={listBoxRef}
          id={tagsPanelId}
          role="tabpanel"
          aria-labelledby={tabId(tabsId, "tags")}
          className="flex flex-col gap-3"
        >
          {staleList && (
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

          {showEmptyTags && (
            <TagsEmpty
              icon={<TagsIcon aria-hidden="true" />}
              title={t.tags.empty.title}
              description={t.tags.empty.description}
              action={
                <Button size="sm" onClick={openCreate}>
                  <Plus aria-hidden="true" />
                  {t.tags.newTag}
                </Button>
              }
            />
          )}

          {showNoUnused && (
            <TagsEmpty
              icon={<VideoOff aria-hidden="true" />}
              title={appliedTentative ? t.tags.noUnusedTentative : t.tags.noUnused.title}
              description={appliedTentative ? undefined : t.tags.noUnused.description}
              action={
                <Button variant="outline" size="sm" onClick={showAllFromFilter}>
                  {t.tags.clearSearch}
                </Button>
              }
            />
          )}

          {showNoUnusedMatch && (
            <TagsEmpty
              icon={<SearchX aria-hidden="true" />}
              title={
                appliedTentative
                  ? t.tags.noUnusedTentativeMatches(appliedSearch)
                  : t.tags.noUnusedMatches(appliedSearch)
              }
              action={
                <Button variant="outline" size="sm" onClick={showAllFromFilterSearch}>
                  {t.tags.clearSearch}
                </Button>
              }
            />
          )}

          {showNoTentative && (
            <TagsEmpty
              icon={<CircleDashed aria-hidden="true" />}
              title={t.tags.noTentative.title}
              description={t.tags.noTentative.description}
              action={
                <Button variant="outline" size="sm" onClick={showAllFromFilter}>
                  {t.tags.clearSearch}
                </Button>
              }
            />
          )}

          {showNoTentativeMatch && (
            <TagsEmpty
              icon={<SearchX aria-hidden="true" />}
              title={t.tags.noTentativeMatches(appliedSearch)}
              action={
                <Button variant="outline" size="sm" onClick={showAllFromFilterSearch}>
                  {t.tags.clearSearch}
                </Button>
              }
            />
          )}

          {showNoMatch && (
            <TagsEmpty
              icon={<SearchX aria-hidden="true" />}
              title={t.tags.noMatches(appliedSearch)}
              action={
                <Button variant="outline" size="sm" onClick={clearSearch}>
                  {t.tags.clearSearch}
                </Button>
              }
            />
          )}

          {showRows && (
            <DataTable label={t.tags.title} stickyHeaderTop={stuckBottom}>
              {/*
                列の見出し（ui-design.md「Column header」）。先頭のチェックは「読み込んだ
                ものをすべて選ぶ」。読み込んだ行が上限を超えると押せず、理由を包みの title と
                sr-only で添える。
              */}
              <TableHeader ref={headRef}>
                <TableRow className="hover:bg-transparent">
                  <TableHead>
                    <div
                      className="flex size-8 items-center justify-center"
                      title={
                        selectAllOverLimit
                          ? t.tags.selectAllOverLimit(maxTagBatch)
                          : undefined
                      }
                    >
                      <Checkbox
                        ref={selectAllRef}
                        checked={selectAllState}
                        onCheckedChange={toggleSelectAll}
                        aria-label={
                          selectAllState === true
                            ? t.tags.clearSelection
                            : t.tags.selectAllLoaded(selectableCount)
                        }
                        aria-describedby={
                          selectAllOverLimit ? selectAllLimitId : undefined
                        }
                        disabled={selectableCount === 0 || selectAllOverLimit}
                      />
                      {selectAllOverLimit && (
                        <span id={selectAllLimitId} className="sr-only">
                          {t.tags.selectAllOverLimit(maxTagBatch)}
                        </span>
                      )}
                    </div>
                  </TableHead>
                  <TableHead className="w-full">{t.tags.columns.name}</TableHead>
                  <TableHead className="text-right">{t.tags.columns.videos}</TableHead>
                  <TableHead aria-hidden="true" />
                </TableRow>
              </TableHeader>
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
          id={rejectedPanelId}
          role="tabpanel"
          aria-labelledby={tabId(tabsId, "rejected")}
          className="flex flex-col gap-3"
        >
          <RejectedNames
            page={rejectedPage}
            error={rejectedError}
            onRetry={reloadRejectedNames}
            morePending={rejectedMorePending}
            moreError={rejectedMoreError}
            onLoadMore={loadMoreRejectedNames}
            resetKey={rejectedEpoch}
            onForget={forgetRejectedName}
            onFocusFallback={() =>
              document.getElementById(tabId(tabsId, "rejected"))?.focus()
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

/** tabId はタブの id である。パネルの `aria-labelledby` とフォーカスの行き先に使う。 */
function tabId(prefix: string, value: string): string {
  return `${prefix}-tab-${value}`;
}

/** SpacerRow は描かない行の高さを保つ空の行である。読み上げない。 */
function SpacerRow({ height }: { height: number }) {
  return (
    <tr aria-hidden="true" style={{ height }}>
      <td colSpan={tagColumnCount} className="p-0" />
    </tr>
  );
}

/**
 * TagsEmpty は見出しつきの空の状態である。題は見出し（h2）にし、読み上げソフトで
 * 見出しから状態へ飛べるようにする（状態の部品の題は見出しの要素を持たないため）。
 */
function TagsEmpty({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: UiText;
  description?: UiText;
  action: ReactNode;
}) {
  return (
    <EmptyState
      icon={icon}
      title={<h2>{title}</h2>}
      description={description}
      action={action}
    />
  );
}

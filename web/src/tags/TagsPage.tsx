import {
  defaultRangeExtractor,
  useWindowVirtualizer,
  type Range,
} from "@tanstack/react-virtual";
import {
  AlertCircle,
  CircleDashed,
  Plus,
  SearchX,
  Tags as TagsIcon,
  VideoOff,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
} from "react";
import { flushSync } from "react-dom";

import { RequestFailed } from "../api/client";
import {
  batchTags,
  confirmTag,
  createTag,
  currentTags,
  deleteTag,
  forgetRejectedTagName,
  listRejectedTagNamePage,
  maxTagBatch,
  refreshTags,
  rejectTag,
  renameTag,
  subscribeTags,
  type Tag,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { foldForMatch } from "../lib/foldForMatch";
import {
  readTagListPreferences,
  writeTagListPreferences,
} from "../preferences/tagListPreferences";
import Button from "../ui/Button";
import Checkbox from "../ui/Checkbox";
import Skeleton from "../ui/Skeleton";
import { useToast } from "../ui/Toast";
import Tooltip from "../ui/Tooltip";
import { EmptyState } from "../videoList/states";
import BulkTagDialog, { type BulkTagAction } from "./BulkTagDialog";
import CreateTagRow from "./CreateTagRow";
import DeleteTagDialog from "./DeleteTagDialog";
import MergeTagDialog from "./MergeTagDialog";
import RejectedNames from "./RejectedNames";
import RejectTagDialog from "./RejectTagDialog";
import SynonymsDialog from "./SynonymsDialog";
import { sortTags, type TagListSort } from "./tagListOrder";
import { tagFieldError, type TagFieldError } from "./tagNameField";
import TagRow, { type TagRowRefs } from "./TagRow";
import TagSearchBox from "./TagSearchBox";
import TagSelectionBar from "./TagSelectionBar";
import { TagCompactSort, TagSortMenu } from "./TagSortControls";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isTagNotTentative(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_tentative";
}

type FocusTarget = "rename" | "synonyms" | "name" | "menu";

/**
 * ROW_ESTIMATE は、まだ描いていない行の高さの見積り（px）である。行（`py-2` と
 * `size-8` の操作、下の線 1px）の高さで、シノニムの行・改名の失敗の文言を持つ
 * 行は描いたあとに測った高さ（`measureElement`）で置き換わる。
 */
const ROW_ESTIMATE = 49;

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
 * タグの一覧は共有の保持（`web/src/api/tags.ts`）を使う。作成・改名・削除の直後は、サーバーが返した最新の1件を
 * 今の一覧へその場で重ねる（もう1回 `GET /api/tags` を送らない。`createTag`・
 * `renameTag`・`deleteTag` 自体が共有の保持をバックグラウンドで取り直すので、
 * 二重の取得にはならない）。タグがもう無いとき（`tag_not_found`）だけ、
 * ほかのタグも変わっているかもしれないので `reload` で取り直す。
 *
 * 仮のタグ（specs/031-tentative-tags/ui-design.md「Tag management page」）:
 * 「Tentative only」の絞り込み（この画面の状態で URL には載せない）、仮の行の
 * 確定・却下、一覧の下の却下した名前の一覧と取り外しも持つ。却下した名前の
 * 一覧はこの画面だけが読むので、共有の保持ではなくここで取る（research.md R-8）。
 */
export default function TagsPage() {
  const toast = useToast();
  const [tags, setTags] = useState<Tag[] | undefined>(currentTags());
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [search, setSearch] = useState("");
  const [tentativeOnly, setTentativeOnly] = useState(false);
  const [unusedOnly, setUnusedOnly] = useState(false);
  // 並び順だけは端末に残す（specs/036-tag-admin-scale/research.md R-7）。
  const [sort, setSort] = useState<TagListSort>(() => readTagListPreferences().sort);

  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);

  const [renamingId, setRenamingId] = useState<number | null>(null);
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
  const [synonymsTagId, setSynonymsTagId] = useState<number | null>(null);
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
  const barConfirmRef = useRef<HTMLButtonElement | null>(null);
  const barMoreRef = useRef<HTMLButtonElement | null>(null);

  const [rejectedNames, setRejectedNames] = useState<string[] | undefined>(undefined);
  const [rejectedError, setRejectedError] = useState<UiText | null>(null);
  const rejectedGeneration = useRef(0);
  /** 取り直しの応答を待っている世代。待っていなければ null。 */
  const rejectedInFlight = useRef<number | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  const tentativeButtonRef = useRef<HTMLButtonElement | null>(null);
  const unusedButtonRef = useRef<HTMLButtonElement | null>(null);
  const rowRefs = useRef(new Map<number, TagRowRefs>());
  const listRef = useRef<HTMLDivElement | null>(null);
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
   * bandRef は上部バーの下に留める帯（操作の行と件数の行）で、stuckBottom は
   * 留まったときの帯の下端の表示域の中での位置（`top` の上部バーの高さ＋帯の
   * 高さ、px）である。行へスクロールするときに、行が帯の下に隠れないよう
   * これを差し引く（ui-design.md「Band」）。帯の高さは幅で変わるので測り直す。
   */
  const bandRef = useRef<HTMLDivElement | null>(null);
  const [stuckBottom, setStuckBottom] = useState(0);

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
   * filtersRef は今描いている検索と「Tentative only」「Unused only」である。要求の応答を
   * 待つ間に絞り込みが変わることがあるので、応答のあとのフォーカス先は
   * 閉じ込めた（押した時点の）値ではなくこれで決める。
   */
  const filtersRef = useRef({ query: "", tentativeOnly: false, unusedOnly: false });

  /**
   * fallbackFocus は、行へ移せないときの最後の行き先である。ふだんは
   * 「新しいタグ」、「Tentative only」「Unused only」を押している間はそのボタン
   * （次の操作が「絞り込みを外す」だから。specs/031-tentative-tags/ui-design.md
   * 「Toolbar」）。両方を押していれば「Tentative only」へ。
   */
  function fallbackFocus() {
    (filtersRef.current.tentativeOnly
      ? tentativeButtonRef
      : filtersRef.current.unusedOnly
        ? unusedButtonRef
        : createButtonRef
    ).current?.focus();
    // 候補を確かめるために描かせた行（`renderRow`）を Tab の順に残さない。
    releasePinIfFocusOutside();
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
   * reloadRejectedNames は却下した名前の一覧を取り直す。却下・作成・改名・
   * シノニムの追加のあと（どれも一覧を変えうる。要件 15、受け入れ条件 14）と、
   * 画面を開いたときに呼ぶ。追い越された古い取得の結果は捨てる。取り直しの
   * 失敗は、最初の読み込みの失敗と同じ見え方（件数を出さず、「Couldn't load
   * the rejected names」と Retry。ui-design.md「Rejected names」）にする。
   * 古い一覧を黙って残すと、却下や作成で変わったはずの並びを正しいものとして
   * 見せ続けてしまう。
   */
  const reloadRejectedNames = useCallback(() => {
    rejectedGeneration.current += 1;
    const generation = rejectedGeneration.current;
    rejectedInFlight.current = generation;
    setRejectedError(null);
    // 入口の件数を total にし、窓の中で続きを読むのは後の単位（specs/036-tag-admin-scale/research.md R-13）。
    // ここでは先頭のページの items だけを使う。
    listRejectedTagNamePage()
      .then((page) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedInFlight.current = null;
        setRejectedNames(page.items);
      })
      .catch((failure: unknown) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedInFlight.current = null;
        setRejectedNames(undefined);
        setRejectedError(errorText(failure));
      });
  }, []);

  /**
   * forgetRejectedName は × の取り外しである。取り外しの前から待っている
   * 取り直しがあれば、その応答は取り外す前の並び（外した名前を含む）かも
   * しれないので捨て、取り外しのあとで取り直す。
   */
  async function forgetRejectedName(name: string) {
    await forgetRejectedTagName(name);
    setRejectedNames((current) => current?.filter((item) => item !== name));
    if (rejectedInFlight.current !== null) reloadRejectedNames();
  }

  const reload = useCallback(() => {
    setLoadError(null);
    // 一覧への反映は下の mount の subscribeTags に一本化し、ここでは
    // `refreshTags` の戻り値を直接 `setTags` へは使わない。この呼び出しが
    // 別の（後から始まった）取り直しに追い越されると、`refreshTags` の
    // generation ガードはこの呼び出し自身の取得結果ではなく、その時点の
    // `held`（追い越した側がまだ終わっていなければ、さらに古い値）を返す。
    // 直前に作成したタグをその場で重ねた直後にこれが起きると、その重ねを
    // 古い一覧で上書きしてしまう（Devin の指摘4）。`subscribeTags` の通知は
    // 常に「実際に held を更新した、最新の取得」でしか呼ばれないので、
    // そちらだけを信頼する。
    return refreshTags()
      .then(() => undefined)
      .catch((failure: unknown) => {
        // 既に一覧を持っているときは、その一覧を残したまま理由だけを控える
        // （読み込み失敗の空の状態は、一覧をまだ一度も取れていないときだけ
        // 出す。N6: 直前の操作は成功しているので、一覧を空白にしない）。
        setLoadError(errorText(failure));
        return undefined;
      });
  }, []);

  useEffect(() => {
    reloadRejectedNames();
  }, [reloadRejectedNames]);

  useEffect(() => {
    let alive = true;
    void reload();
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, [reload]);

  // 一覧は選んだ並び順（名前・本数・作った日。同値は名前の自然順）で並べる
  // （specs/036-tag-admin-scale/data-model.md §4）。作成・改名でその場に重ねた
  // 1件も、ここで並びの中の位置へ入る。絞り込みと検索は並びを変えないので、
  // 並べてから絞っても「絞ってから並べる」と同じ結果になる。
  const sorted = useMemo(() => {
    if (tags === undefined) return [];
    return sortTags(tags, sort);
  }, [tags, sort]);

  function changeSort(next: TagListSort) {
    setSort(next);
    writeTagListPreferences({ sort: next });
  }

  // 検索はライブラリでタグを探すときと同じ照合形（`foldForMatch`）で照らす
  // （specs/036-tag-admin-scale/research.md R-3）。タグごとの照合形はタグの
  // 配列が変わったときだけ作り直し、打鍵ごとには作らない（data-model.md §4）。
  const normalizedQuery = foldForMatch(search).trim();
  const searchKeys = useMemo(
    () => new Map(sorted.map((tag) => [tag, tagSearchKeys(tag)])),
    [sorted],
  );
  const filtered = useMemo(
    () =>
      sorted.filter((tag) =>
        matchesFilters(
          tag,
          searchKeys.get(tag)!,
          normalizedQuery,
          tentativeOnly,
          unusedOnly,
        ),
      ),
    [sorted, searchKeys, normalizedQuery, tentativeOnly, unusedOnly],
  );

  useLayoutEffect(() => {
    filtersRef.current = { query: normalizedQuery, tentativeOnly, unusedOnly };
  }, [normalizedQuery, tentativeOnly, unusedOnly]);

  /**
   * shown は、その1件が今の検索と絞り込み（「Tentative only」「Unused only」）で
   * 一覧に出るかである。要求の応答のあとで呼ぶので、`filtersRef` の今の値で決める。
   */
  function shown(tag: Tag): boolean {
    const {
      query,
      tentativeOnly: onlyTentative,
      unusedOnly: onlyUnused,
    } = filtersRef.current;
    return matchesFilters(tag, tagSearchKeys(tag), query, onlyTentative, onlyUnused);
  }

  /**
   * visibleRows は実際に並べる行である。改名中の行は、検索・絞り込み・並び順を
   * 変えて一致しなくなっても一覧から外さない（外すとその行が消え、打っている
   * 途中の名前を失う）。`filtered` に無ければ `sorted` の並びのまま差し込む。
   * 件数の行はこれを数えず、実際の一致件数（`filtered`）のまま見せる。
   */
  const visibleRows = useMemo(() => {
    if (renamingId === null) return filtered;
    if (filtered.some((tag) => tag.id === renamingId)) return filtered;
    const matched = new Set(filtered);
    return sorted.filter((tag) => matched.has(tag) || tag.id === renamingId);
  }, [filtered, sorted, renamingId]);

  const visibleRowsRef = useRef<readonly Tag[]>(visibleRows);
  useLayoutEffect(() => {
    visibleRowsRef.current = visibleRows;
  }, [visibleRows]);

  // 選択は見えている行の部分集合に保つ。検索・絞り込みを変えて見えなくなった行と、
  // 改名中の行を外す。並び順だけの変更では行は見えたままなので残る
  // （specs/036-tag-admin-scale/data-model.md §4「選択」）。
  useLayoutEffect(() => {
    setSelected((current) => {
      if (current.size === 0) return current;
      const visible = new Set(visibleRows.map((tag) => tag.id));
      const next = new Set<number>();
      for (const id of current) {
        if (visible.has(id) && id !== renamingId) next.add(id);
      }
      return next.size === current.size ? current : next;
    });
  }, [visibleRows, renamingId]);

  /**
   * selectableCount は「見えているものをすべて選ぶ」の対象の数である。改名中の行は
   * 入らない（ui-design.md「Count line」）。
   */
  const selectableCount =
    visibleRows.length -
    (renamingId !== null && visibleRows.some((tag) => tag.id === renamingId) ? 1 : 0);
  /** overLimit は見えている数がまとめての操作の上限を超えるかである。 */
  const overLimit = visibleRows.length > maxTagBatch;
  const selection = useMemo(() => {
    let tentative = false;
    let confirmed = false;
    if (selected.size > 0) {
      for (const tag of visibleRows) {
        if (!selected.has(tag.id)) continue;
        if (tag.tentative) tentative = true;
        else confirmed = true;
        if (tentative && confirmed) break;
      }
    }
    return { tentative, confirmed };
  }, [selected, visibleRows]);
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
    scrollPaddingStart: stuckBottom,
    rangeExtractor,
    getItemKey,
  });

  // 一覧の上端は作成の行の有無やツールバーの折り返しで動くので、文書の
  // 大きさが変わるたびに測り直す。
  const hasList = tags !== undefined && visibleRows.length > 0;
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

  // 帯の下端（上部バー＋帯の高さ）を測る。帯は幅で 1 行・2 行に折り返す。
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
    root.style.scrollPaddingTop = `${String(stuckBottom)}px`;
    return () => {
      root.style.scrollPaddingTop = previous;
    };
  }, [stuckBottom]);

  /**
   * handleListFocus は、フォーカスを持った行を描き続ける行にする。スクロール
   * で画面の外へ出ても、その行は外れず、フォーカスが `body` へ落ちない。
   */
  function handleListFocus(event: FocusEvent<HTMLDivElement>) {
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
  function handleListKeyDown(event: KeyboardEvent<HTMLDivElement>) {
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

  const searching = normalizedQuery !== "";
  const total = tags?.length ?? 0;
  const countText =
    tags === undefined
      ? t.tags.loading
      : searching || tentativeOnly || unusedOnly
        ? t.tags.filteredCount(filtered.length, total)
        : t.tags.count(total);

  function openCreate() {
    // 改名の送信中は、その応答が届くまで新しく作成を始めない（B2 と同じ規則。
    // 「新しいタグ」自体も createPending・creating では disabled だが、
    // renamePending はボタンの disabled 条件に含めているので、ここは主に
    // キーボード操作などボタンを介さない呼び出しへの保険である）。
    if (renamePending) return;
    // 作成の行はいつも一覧の先頭に入るので、一覧の先頭が帯の下に隠れていれば
    // 先に先頭まで戻す（見えない位置に入力を作らない。ui-design.md「Band」）。
    const listTop = listBoxRef.current?.getBoundingClientRect().top;
    if (listTop !== undefined && listTop < stuckBottom) {
      window.scrollTo({ top: Math.max(0, window.scrollY + listTop - stuckBottom) });
    }
    setCreating(true);
    setCreateError(null);
    setRenamingId(null);
  }

  function clearSearch() {
    setSearch("");
    searchInputRef.current?.focus();
  }

  /**
   * toggleTentativeOnly は「Tentative only」を押す・外す。外した結果タグが
   * 1つも無ければボタンは disabled になるので、フォーカスを「新しいタグ」へ
   * 移す（031 の ui-design.md「Toolbar」）。
   */
  function toggleTentativeOnly() {
    if (tentativeOnly && total === 0) {
      setTimeout(() => createButtonRef.current?.focus(), 0);
    }
    setTentativeOnly((value) => !value);
  }

  /**
   * toggleUnusedOnly は「Unused only」を押す・外す。`toggleTentativeOnly` と同じ
   * 規則（specs/036-tag-admin-scale/ui-design.md「Controls」）。
   */
  function toggleUnusedOnly() {
    if (unusedOnly && total === 0) {
      setTimeout(() => createButtonRef.current?.focus(), 0);
    }
    setUnusedOnly((value) => !value);
  }

  /**
   * showAllFromUnused は「No unused tags」「No unused tentative tags」の
   * 「Show all tags」である。絞り込みを外し、「Unused only」だけなら
   * 「Unused only」へ、両方なら「Tentative only」へ（タグが 0 なら
   * 「新しいタグ」へ）移す（ui-design.md「States」）。
   */
  function showAllFromUnused() {
    const target = tentativeOnly ? tentativeButtonRef : unusedButtonRef;
    setUnusedOnly(false);
    setTentativeOnly(false);
    setTimeout(() => (total === 0 ? createButtonRef : target).current?.focus(), 0);
  }

  /**
   * showAllFromUnusedSearch は「No unused (tentative) tags match」の
   * 「Show all tags」である。絞り込みと検索を外し、検索の入力へ移す。
   */
  function showAllFromUnusedSearch() {
    setUnusedOnly(false);
    setTentativeOnly(false);
    setSearch("");
    setTimeout(
      () => (total === 0 ? createButtonRef : searchInputRef).current?.focus(),
      0,
    );
  }

  /**
   * showAllFromTentative は「No tentative tags」の「Show all tags」である。
   * 絞り込みを外し、「Tentative only」へ（タグが 0 なら「新しいタグ」へ）移す。
   */
  function showAllFromTentative() {
    setTentativeOnly(false);
    setTimeout(
      () => (total === 0 ? createButtonRef : tentativeButtonRef).current?.focus(),
      0,
    );
  }

  /**
   * showAllFromTentativeSearch は「No tentative tags match」の「Show all tags」
   * である。絞り込みと検索の両方を外し、検索の入力へ移す。
   */
  function showAllFromTentativeSearch() {
    setTentativeOnly(false);
    setSearch("");
    setTimeout(
      () => (total === 0 ? createButtonRef : searchInputRef).current?.focus(),
      0,
    );
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTag(name);
      setTags((current) => (current === undefined ? [created] : [...current, created]));
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
      setTags((current) =>
        current?.map((item) => (item.id === updated.id ? updated : item)),
      );
      setRenamingId(null);
      reloadRejectedNames();
      // 改名で確定になった仮のタグは「Tentative only」から外れる（031 の
      // ui-design.md「Toolbar」）。検索に一致しなくなったときも同じ扱い。
      if (shown(updated)) focusRow(tag.id, "rename");
      else focusAfterRemoval(order, tag.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRenamingId(null);
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
      setTags((current) => current?.filter((item) => item.id !== target.id));
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
      setTags((current) =>
        current?.map((item) => (item.id === updated.id ? updated : item)),
      );
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
      setTags((current) => current?.filter((item) => item.id !== target.id));
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
    if (fromSelection) afterCommit(() => barMoreRef.current?.focus());
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
  ) {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    const order = visibleRows;
    const removed = new Set(sourceIds);
    setTags((current) =>
      current
        ?.filter((item) => !removed.has(item.id))
        .map((item) => (item.id === merged.id ? merged : item)),
    );
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
    const order = filtered;
    setMerging(null);
    toast(fromSelection ? t.tags.selection.stale : t.tags.gone);
    void reload().then(() => {
      if (fromSelection) {
        const more = barMoreRef.current;
        if (more?.isConnected === true && !more.disabled) more.focus();
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
    setSynonymsTagId(null);
    if (filtered.some((tag) => tag.id === id)) {
      focusRow(id, "synonyms");
      return;
    }
    const visibleIds = new Set(filtered.map((tag) => tag.id));
    const order = synonymsOrderRef.current;
    const gone = new Set(
      order.filter((tag) => !visibleIds.has(tag.id)).map((tag) => tag.id),
    );
    focusAfterRemoval(order, id, gone);
  }

  /**
   * updateSynonymsTag はシノニムの登録・解除・シノニム登録に伴う統合が
   * 成功したときに、一覧の中のその1件を差し替える（`web/src/api/tags.ts` の
   * 各関数がバックグラウンドで共有の一覧も取り直すが、ここではその結果を
   * 待たずに画面へその場で反映する。作成・改名・削除と同じ扱い）。
   *
   * `removedId` は、シノニム登録に伴う統合（承諾したとき）でだけ渡す。統合元
   * のタグは統合先のシノニムになって一覧から消えるので、そのタグを一覧から
   * 取り除いてから統合先を差し替える。渡さなければ（素のシノニムの登録・
   * 解除）何も取り除かない。取り除かないと、統合元がバックグラウンドの
   * 取り直し（または、それが失敗すれば永久）まで一覧に残ってしまう。
   */
  function updateSynonymsTag(updated: Tag, removedId?: number) {
    setTags((current) => {
      if (current === undefined) return current;
      const withoutRemoved =
        removedId === undefined
          ? current
          : current.filter((item) => item.id !== removedId);
      return withoutRemoved.map((item) => (item.id === updated.id ? updated : item));
    });
    // シノニムに足した名前が却下した名前だったなら、一覧から外れる（要件 15）。
    reloadRejectedNames();
  }

  /**
   * removeSynonymFromTag は、1件のシノニムの解除が成功したときに呼ぶ。
   * `SynonymsDialog` に渡した `tag` の閉じ込め（古いかもしれない）ではなく、
   * `setTags` の関数形で常に最新の一覧からその名前だけを取り除く。複数の
   * シノニムをほぼ同時に解除したとき、それぞれの応答が別々にここへ届いても、
   * 互いの結果を巻き戻さない（N5・並行する解除）。
   */
  function removeSynonymFromTag(tagId: number, name: string) {
    setTags((current) =>
      current?.map((item) =>
        item.id === tagId
          ? { ...item, synonyms: item.synonyms.filter((s) => s !== name) }
          : item,
      ),
    );
  }

  /**
   * staleSynonyms は、シノニムの窓を開いていたタグがもう無かった
   * （tag_not_found）ときに呼ぶ。
   */
  function staleSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    const order = filtered;
    setSynonymsTagId(null);
    toast(t.tags.gone);
    void reload().then(() => focusAfterRemoval(order, id));
  }

  /**
   * toggleSelectAll は件数の行の先頭のチェックである。空・中間なら見えている行の
   * うち選べる行をすべて選び、全部なら選択を解く（要件 9、ui-design.md「Count line」）。
   */
  function toggleSelectAll() {
    if (selectAllState === true) {
      setSelected(new Set());
      return;
    }
    setSelected(
      new Set(visibleRows.filter((tag) => tag.id !== renamingId).map((tag) => tag.id)),
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
      setTags((current) =>
        current?.map((item) =>
          applied.has(item.id) ? { ...item, tentative: false } : item,
        ),
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
   * only」中に一覧が空になればそのボタン、バーが残れば押せる「Confirm」、押せな
   * ければ「More」、バーが消えれば件数の行の先頭のチェックへ（ui-design.md
   * 「Bulk confirm」）。押せないボタンへは置かない。
   */
  function focusAfterBulkConfirm() {
    if (filtersRef.current.tentativeOnly && visibleRowsRef.current.length === 0) {
      tentativeButtonRef.current?.focus();
      return;
    }
    for (const candidate of [barConfirmRef.current, barMoreRef.current]) {
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
      setTags((current) => current?.filter((item) => !applied.has(item.id)));
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
      // 「Tentative only」を押していればそのボタン、押していなければ先頭のチェック。
      const fallback = () => {
        if (filtersRef.current.tentativeOnly) tentativeButtonRef.current?.focus();
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

  /** cancelBulk は確認の窓を何も変えずに閉じ、フォーカスを「More」へ戻す。 */
  function cancelBulk() {
    if (bulkPending !== null) return;
    setBulkDialog(null);
    setBulkError(null);
    afterCommit(() => barMoreRef.current?.focus());
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
        setRenamingId(target.id);
        // 選んでいる行の改名を始めると、その行を選択から外す（ui-design.md
        // 「Row checkbox」）。
        setSelected((current) => withoutIds(current, new Set([target.id])));
      },
      onCancelRename: (target) => {
        if (renamePending) return;
        setRenamingId(null);
        setRenameError(null);
        focusRow(target.id, "rename");
      },
      onSubmitRename: (target, name) => void submitRename(target, name),
      onOpenSynonyms: (target) => {
        synonymsOrderRef.current = visibleRows;
        setSynonymsTagId(target.id);
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

  const showEmptyTags =
    tags !== undefined && tags.length === 0 && !creating && !tentativeOnly && !unusedOnly;
  const nothingShown =
    tags !== undefined && visibleRows.length === 0 && !creating && !showEmptyTags;
  const showNoUnused = nothingShown && unusedOnly && !searching;
  const showNoUnusedMatch = nothingShown && unusedOnly && searching;
  const showNoTentative = nothingShown && !unusedOnly && tentativeOnly && !searching;
  const showNoTentativeMatch = nothingShown && !unusedOnly && tentativeOnly && searching;
  const showNoMatch = nothingShown && !tentativeOnly && !unusedOnly;
  const sortDisabled = tags === undefined || tags.length === 0;
  const showRows =
    tags !== undefined && (visibleRows.length > 0 || creating) && !showEmptyTags;

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6 sm:py-8">
      <h1 className="text-xl font-semibold">{t.tags.title}</h1>
      {/*
        帯（specs/036-tag-admin-scale/ui-design.md「Band」）。操作の行と件数の行を
        上部バーの下に留め、h1 と一覧は本文と一緒に流れる。面は不透明の bg-bg で、
        下を流れる行が透けない。z-20 は選択バー（z-30）と上部バー（z-40）の下。
        h1 との間（pt-3）・操作の行と件数の行の間（mt-2）・件数の行と最初の行の
        間（pb-2）は、帯を入れる前の mt-3・mt-2・mt-2 と同じ。間隔を帯の中の余白に
        するのは、留まったときに操作の行が上部バーに接しないためである。
      */}
      <div
        ref={bandRef}
        className="sticky top-navbar z-20 border-b border-border bg-bg pt-3 pb-2"
      >
        {/*
          操作の行（specs/036-tag-admin-scale/ui-design.md「Controls」「Responsive
          behaviour」）。lg 以上は 1 行で 検索 →「Tentative only」→「Unused only」→
          並び順 → 右端に「新しいタグ」。lg 未満は 2 行で、1 行目が 検索 →「新しい
          タグ」、2 行目が絞り込みと並び順。幅の出し分けは CSS だけで行い、Tab の
          順は DOM の順（lg 以上の見た目の順）のままにする。
        */}
        <div className="flex flex-wrap items-center gap-2 sm:gap-3">
          <TagSearchBox
            value={search}
            onChange={setSearch}
            inputRef={searchInputRef}
            disabled={tags !== undefined && tags.length === 0}
            className="order-1 min-w-0 flex-1 sm:max-w-sm"
          />
          <div className="order-3 flex w-full items-center gap-2 sm:gap-3 lg:order-2 lg:w-auto">
            <Tooltip content={t.tags.tentativeOnlyHint}>
              <Button
                ref={tentativeButtonRef}
                aria-pressed={tentativeOnly}
                className="aria-pressed:border-accent-active aria-pressed:bg-accent-soft aria-pressed:text-link"
                onClick={toggleTentativeOnly}
                // 押している間は、タグが 0 になっても disabled にしない
                // （フォーカスの行き先で、絞り込みを外す唯一の手でもある）。
                disabled={tags === undefined || (!tentativeOnly && tags.length === 0)}
              >
                <CircleDashed className="max-sm:hidden" />
                {t.tags.tentativeOnly}
              </Button>
            </Tooltip>
            <Tooltip content={t.tags.unusedOnlyHint}>
              <Button
                ref={unusedButtonRef}
                aria-pressed={unusedOnly}
                className="aria-pressed:border-accent-active aria-pressed:bg-accent-soft aria-pressed:text-link"
                onClick={toggleUnusedOnly}
                // 「Tentative only」と同じく、押している間は disabled にしない。
                disabled={tags === undefined || (!unusedOnly && tags.length === 0)}
              >
                <VideoOff className="max-sm:hidden" />
                {t.tags.unusedOnly}
              </Button>
            </Tooltip>
            <TagSortMenu
              sort={sort}
              onSortChange={changeSort}
              disabled={sortDisabled}
              className="max-sm:hidden"
            />
            <TagCompactSort
              sort={sort}
              onSortChange={changeSort}
              disabled={sortDisabled}
              className="ml-auto sm:hidden"
            />
          </div>
          <Button
            ref={createButtonRef}
            variant="primary"
            className="order-2 ml-auto lg:order-3"
            onClick={openCreate}
            disabled={tags === undefined || creating || createPending || renamePending}
          >
            <Plus />
            {t.tags.newTag}
          </Button>
        </div>

        {/*
          件数の行（ui-design.md「Count line」）。先頭のチェックは行のチェックと同じ列
          （行の px-2 と size-8 の包み）に置き、行の高さは h-5 のまま。
          狭い幅で件数（「1,000 of 1,000 tags」）と入口（「Rejected names 1,000」）が
          1 行に収まらないときは、件数を折り返さずに入口を次の行の右端へ送る。
          どちらも省略しない（入口は却下した名前への唯一の入口）。行の間の gap-y-3 は、
          チェックの包みと入口の -my-1.5（上下 6px ずつ）が重ならない幅。帯の高さが
          変わっても、ResizeObserver が測り直してスクロール位置に渡す。
        */}
        <div className="group mt-2 flex min-h-5 flex-wrap items-center gap-x-2 gap-y-3 pl-2 sm:gap-x-3">
          <div className="-my-1.5 flex size-8 shrink-0 items-center justify-center">
            <Checkbox
              ref={selectAllRef}
              checked={selectAllState}
              onCheckedChange={toggleSelectAll}
              label={
                selectAllState === true ? t.tags.clearSelection : t.tags.selectAllShown
              }
              disabled={tags === undefined || selectableCount === 0 || overLimit}
              className={cn(
                "transition-opacity",
                selected.size > 0
                  ? "opacity-100"
                  : "opacity-40 group-focus-within:opacity-100 group-hover:opacity-100",
              )}
            />
          </div>
          <p
            role="status"
            aria-live="polite"
            className="text-xs whitespace-nowrap text-fg-muted tabular-nums"
          >
            {countText}
          </p>
          {/*
          却下した名前の入口（ui-design.md「Count line」）。タグの一覧をまだ一度も
          取れていない間（読み込み中・読み込み失敗）は置かない。「タグはまだ
          ありません」のときは置く（031 の ui-design.md「Rejected names」）。
        */}
          {tags !== undefined && (
            <RejectedNames
              names={rejectedNames}
              error={rejectedError}
              onRetry={reloadRejectedNames}
              onForget={forgetRejectedName}
              className="-my-1.5 ml-auto"
            />
          )}
        </div>
      </div>

      <div
        ref={listBoxRef}
        className={cn(!showRows && "mt-2", selected.size > 0 && "pb-16")}
      >
        {tags === undefined && loadError === null && (
          <div className="space-y-2" aria-hidden="true">
            {Array.from({ length: 6 }, (_, index) => (
              <Skeleton key={index} className="h-10" />
            ))}
          </div>
        )}

        {tags === undefined && loadError !== null && (
          <EmptyState
            icon={AlertCircle}
            tone="danger"
            title={t.tags.loadFailed}
            action={<Button onClick={() => void reload()}>{t.common.retry}</Button>}
          />
        )}

        {showEmptyTags && (
          <EmptyState
            icon={TagsIcon}
            title={t.tags.empty.title}
            description={t.tags.empty.description}
            action={
              <Button variant="primary" onClick={openCreate}>
                <Plus />
                {t.tags.newTag}
              </Button>
            }
          />
        )}

        {showNoUnused && (
          <EmptyState
            icon={VideoOff}
            title={tentativeOnly ? t.tags.noUnusedTentative : t.tags.noUnused.title}
            description={tentativeOnly ? undefined : t.tags.noUnused.description}
            action={<Button onClick={showAllFromUnused}>{t.tags.clearSearch}</Button>}
          />
        )}

        {showNoUnusedMatch && (
          <EmptyState
            icon={SearchX}
            title={
              tentativeOnly
                ? t.tags.noUnusedTentativeMatches(search)
                : t.tags.noUnusedMatches(search)
            }
            action={
              <Button onClick={showAllFromUnusedSearch}>{t.tags.clearSearch}</Button>
            }
          />
        )}

        {showNoTentative && (
          <EmptyState
            icon={CircleDashed}
            title={t.tags.noTentative.title}
            description={t.tags.noTentative.description}
            action={<Button onClick={showAllFromTentative}>{t.tags.clearSearch}</Button>}
          />
        )}

        {showNoTentativeMatch && (
          <EmptyState
            icon={SearchX}
            title={t.tags.noTentativeMatches(search)}
            action={
              <Button onClick={showAllFromTentativeSearch}>{t.tags.clearSearch}</Button>
            }
          />
        )}

        {showNoMatch && (
          <EmptyState
            icon={SearchX}
            title={t.tags.noMatches(search)}
            action={<Button onClick={clearSearch}>{t.tags.clearSearch}</Button>}
          />
        )}

        {showRows && (
          <div className="divide-y divide-border">
            {creating && (
              <CreateTagRow
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
            )}
            {visibleRows.length > 0 && (
              // 行は文書の中の位置へ置き（`translateY`）、高さを持つ包みが全件の
              // 高さを保つ。行の間の線は `divide-y` と同じ色・太さを各行に付ける
              // （全件の最後の行には付けない）。
              <div
                ref={listRef}
                className="relative"
                style={{ height: virtualizer.getTotalSize() }}
                onFocus={handleListFocus}
                onBlur={handleListBlur}
                onKeyDown={handleListKeyDown}
              >
                {virtualizer.getVirtualItems().map((item) => {
                  const tag = visibleRows[item.index]!;
                  return (
                    <div
                      key={item.key}
                      data-index={item.index}
                      ref={virtualizer.measureElement}
                      className={cn(
                        "absolute inset-x-0 top-0",
                        item.index < visibleRows.length - 1 &&
                          "*:border-b *:border-border",
                      )}
                      style={{
                        transform: `translateY(${String(item.start - virtualizer.options.scrollMargin)}px)`,
                      }}
                    >
                      <TagRow
                        tag={tag}
                        renaming={renamingId === tag.id}
                        pending={renamingId === tag.id && renamePending}
                        blockStart={
                          createPending || (renamePending && renamingId !== tag.id)
                        }
                        error={renamingId === tag.id ? renameError : null}
                        registerRefs={registerRefs}
                        confirming={confirming.has(tag.id)}
                        selected={selected.has(tag.id)}
                        selectionActive={selected.size > 0}
                        {...rowHandlers}
                      />
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>

      <TagSelectionBar
        count={selected.size}
        hasTentative={selection.tentative}
        hasConfirmed={selection.confirmed}
        overLimit={overLimit}
        busy={bulkPending !== null}
        confirming={bulkPending === "confirm"}
        onConfirm={() => void submitBulkConfirm()}
        onReject={() => openBulkDialog("reject")}
        onDelete={() => openBulkDialog("delete")}
        onMerge={openMergeSelected}
        onClear={clearSelection}
        confirmRef={barConfirmRef}
        moreRef={barMoreRef}
      />

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

      {merging !== null && tags !== undefined && (
        <MergeTagDialog
          sources={merging.sources}
          fromSelection={merging.fromSelection}
          onClose={cancelMerge}
          onMerged={performMerge}
          onStale={staleMerge}
        />
      )}

      {synonymsTagId !== null &&
        (() => {
          const synonymsTag = tags?.find((item) => item.id === synonymsTagId);
          // 別のタブでの削除・統合と、staleSynonyms による一覧の取り直しの
          // 間に、そのタグがもう一覧に無い一瞬がありうる。窓はまだ閉じ切って
          // いないその一瞬だけ何も出さない。
          if (synonymsTag === undefined) return null;
          return (
            <SynonymsDialog
              tag={synonymsTag}
              onClose={cancelSynonyms}
              onTagUpdated={updateSynonymsTag}
              onSynonymRemoved={removeSynonymFromTag}
              onStale={staleSynonyms}
            />
          );
        })()}
    </div>
  );
}

/** TagSearchKeys は、タグの名前とシノニムの照合形（`foldForMatch`）である。 */
interface TagSearchKeys {
  name: string;
  synonyms: string[];
}

function tagSearchKeys(tag: Tag): TagSearchKeys {
  return { name: foldForMatch(tag.name), synonyms: tag.synonyms.map(foldForMatch) };
}

/**
 * matchesFilters は、タグが検索（検索語の照合形が名前かシノニムの照合形に
 * 部分一致する）と絞り込み（「Tentative only」「Unused only」）のすべてに一致する
 * かである。`normalizedQuery` は `foldForMatch` を掛けた検索語、`keys` はその
 * タグの照合形である。
 */
function matchesFilters(
  tag: Tag,
  keys: TagSearchKeys,
  normalizedQuery: string,
  tentativeOnly: boolean,
  unusedOnly: boolean,
): boolean {
  if (tentativeOnly && !tag.tentative) return false;
  if (unusedOnly && tag.videoCount !== 0) return false;
  if (normalizedQuery === "") return true;
  return (
    keys.name.includes(normalizedQuery) ||
    keys.synonyms.some((synonym) => synonym.includes(normalizedQuery))
  );
}

import {
  defaultRangeExtractor,
  useWindowVirtualizer,
  type Range,
} from "@tanstack/react-virtual";
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

import type { Tag } from "../api/tags";
import type { TagRowRefs } from "./TagRow";

export type FocusTarget = "rename" | "synonyms" | "name" | "menu";

/**
 * ROW_ESTIMATE は、まだ描いていない行の高さの見積り（px）である。行（`py-2` と
 * `size-8` の操作、下の線 1px）の高さで、シノニムの行・改名の失敗の文言を持つ
 * 行は描いたあとに測った高さ（`measureElement`）で置き換わる。
 */
const ROW_ESTIMATE = 49;

/** ROW_OVERSCAN は、表示域の前後に余分に描く行の数である。 */
export const ROW_OVERSCAN = 8;

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

/**
 * useVirtualTagRows はタグ管理画面の一覧を、表示域と前後の少数の行だけ描く
 * （specs/036-tag-admin-scale/research.md R-2）。描いていない行へのフォーカスの移動と、
 * 描いている範囲の端での Tab の受け渡しも持つ（ui-design.md「Keyboard across
 * virtualized rows」）。`fallbackFocus` は、行へ移せないときの最後の行き先である。
 */
export function useVirtualTagRows({
  visibleRows,
  renamingId,
  scrollMargin,
  rowsTop,
  fallbackFocus,
}: {
  /** 並べる行（差し込んで残した改名中の行を含む）。 */
  visibleRows: readonly Tag[];
  renamingId: number | null;
  /** 一覧の上端の文書の中での位置（px）。 */
  scrollMargin: number;
  /** 行へスクロールするときに上から差し引く高さ（帯と列の見出し、px）。 */
  rowsTop: number;
  fallbackFocus: () => void;
}) {
  const fallbackRef = useRef(fallbackFocus);
  useLayoutEffect(() => {
    fallbackRef.current = fallbackFocus;
  });
  function focusFallback() {
    fallbackRef.current();
  }

  const rowRefs = useRef(new Map<number, TagRowRefs>());
  const listRef = useRef<HTMLTableSectionElement | null>(null);
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
   * `focusFallback` へ移す。行の差し替えが DOM に反映されたあとで移す必要が
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
        focusFallback();
      } else target.focus();
    });
  }

  /**
   * focusAfterRemoval は、行が一覧から消えた（削除・却下、tag_not_found の
   * 取り直しで消えた、または「Tentative only」の絞り込みから外れた）あとの
   * フォーカス先を決める。次の行の「改名」、無ければ前の行、1つも無ければ
   * `focusFallback`（ui-design.md「Merge and delete」、031 の「Toolbar」）。
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
    fallback: () => void = focusFallback,
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
   * タッチの端末と `sm` 未満では行のアイコンのボタン（`RowIconButton`）が隠れて「Actions」1 つに
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

  const visibleRowsRef = useRef<readonly Tag[]>(visibleRows);
  useLayoutEffect(() => {
    visibleRowsRef.current = visibleRows;
  }, [visibleRows]);

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

  return {
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
  };
}

import { useCallback, useEffect, useState } from "react";

import { type LibraryItem, toLibraryItems } from "../api/client";
import type { GroupRef } from "../api/groups";
import { itemKey } from "../api/libraryItems";
import { readThrough } from "../api/readThrough";
import { subscribeServerEvents } from "../api/serverEvents";
import { type PagedDetailState, usePagedDetail } from "../api/usePagedDetail";
import {
  type LibrarySmartFolder,
  listSmartFolderItems,
  readSmartFolderPage,
  type SmartFolderSettings,
  type SmartRule,
} from "../api/smartFolders";
import type { AncestorRule } from "./pageRules";

/** SmartFolderPageData は読めたスマートフォルダのページの中身である。 */
export interface SmartFolderPageData {
  /** フォルダのカードの値（再生順から）。 */
  folder: LibrarySmartFolder;
  /** 条件・並べ順・欠けた参照。所有者の応答にだけ入る。 */
  settings?: SmartFolderSettings;
  /** 祖先（外側から）。 */
  trail: readonly GroupRef[];
  /** 子フォルダ（名前の順）、続けて動画とグループ（フォルダの並べ順）。 */
  items: readonly LibraryItem[];
  /** 続きがあるときだけ入る。 */
  nextCursor?: string;
}

export type SmartFolderPageState = PagedDetailState<SmartFolderPageData>;

/**
 * appendItems は読んだ続きのページを今の項目の後ろへ足す。同じ項目（itemKey）が前のページに
 * あれば足さない（読む間に一致が変わって、ページの境目で同じ項目が二度返ったとき）。
 */
export function appendItems(
  current: readonly LibraryItem[],
  page: readonly LibraryItem[],
): LibraryItem[] {
  const known = new Set(current.map(itemKey));
  return [...current, ...page.filter((item) => !known.has(itemKey(item)))];
}

/** readItems はフォルダの中身の続きを 1 ページ読み、項目を読み分ける。 */
async function readItems(id: number, cursor: string, signal: AbortSignal) {
  const page = await listSmartFolderItems(id, cursor, signal);
  return { items: toLibraryItems(page.items), nextCursor: page.nextCursor };
}

/**
 * readFolderThrough はフォルダを読み、項目が `atLeast` 件そろうか続きが無くなるまで続きの
 * ページを辿る（読み直しで、並んでいた分を減らさずに入れ替える）。
 */
async function readFolderThrough(
  id: number,
  atLeast: number,
  signal: AbortSignal,
): Promise<SmartFolderPageData> {
  const detail = await readSmartFolderPage(id, signal);
  const { items, nextCursor } = await readThrough(
    { items: toLibraryItems(detail.items), nextCursor: detail.nextCursor },
    (cursor) => readItems(id, cursor, signal),
    appendItems,
    atLeast,
  );
  return {
    folder: detail.smartFolder,
    settings: detail.settings,
    trail: detail.trail,
    items,
    nextCursor,
  };
}

/**
 * useSmartFolderPage はスマートフォルダのページ 1 つの読み込みである
 * （specs/052-smart-folders/ui-design.md「Smart folder page」「Reading again」）。開いたときに
 * 読み、続きは `loadMore` で 1 ページずつ足す。このフォルダの id を運ぶ `smartFolder` の知らせと
 * つなぎ直しで、骨組みを出さずに並んでいる分を保ったまま読み直す。
 *
 * 新しい読み込みを始めると、古い読み込みと続きのページの応答は捨てる。静かな読み直しの失敗
 * （404 のほか）は並んでいる中身を残す。どの読み込みの 404 もフォルダが無くなったこととして
 * 見せる。
 */
export function useSmartFolderPage(id: number | null) {
  const { state, stateRef, more, read, loadMore, stop, markGone } = usePagedDetail(id, {
    read: readFolderThrough,
    more: readItems,
    append: appendItems,
  });
  const reload = useCallback(() => read(false), [read]);
  const refresh = useCallback(() => read(true), [read]);

  useEffect(() => {
    // 知らせは読み込みの前に購読し、読んだ後に起きた変化を取りこぼさない。
    const unsubscribe = subscribeServerEvents({
      smartFolder: (ids) => {
        if (id !== null && ids.includes(id) && stateRef.current.kind !== "gone") {
          refresh();
        }
      },
      open: (reconnected) => {
        if (reconnected && stateRef.current.kind === "ready") refresh();
      },
    });
    reload();
    return () => {
      unsubscribe();
      stop();
    };
  }, [id, refresh, reload, stateRef, stop]);

  return { state, more, reload, refresh, loadMore, markGone };
}

/**
 * useAncestorRules は祖先（近い順に返す）の条件を、それぞれのフォルダを 1 件だけ読んで
 * 引く（`settings` は所有者の応答にだけ入る）。`version` が変わるたびに読み直す。呼び手は
 * ページを読み直したときだけ変わる値（読んだフォルダの値）を渡し、続きのページを足しただけ
 * では読み直さない。読めない祖先の条件は undefined のまま（チップの代わりに骨組み）。
 */
export function useAncestorRules(
  trail: readonly GroupRef[],
  enabled: boolean,
  version: unknown,
): AncestorRule[] {
  const [rules, setRules] = useState<ReadonlyMap<number, SmartRule>>(new Map());
  const ids = trail.map((folder) => folder.id).join(",");
  useEffect(() => {
    if (!enabled || ids === "") return;
    const controller = new AbortController();
    for (const id of ids.split(",").map(Number)) {
      readSmartFolderPage(id, controller.signal, 1).then(
        (detail) => {
          const rule = detail.settings?.rule;
          if (rule === undefined || controller.signal.aborted) return;
          setRules((current) => new Map(current).set(id, rule));
        },
        () => undefined,
      );
    }
    return () => controller.abort();
  }, [enabled, ids, version]);
  return [...trail].reverse().map((folder) => ({ folder, rule: rules.get(folder.id) }));
}

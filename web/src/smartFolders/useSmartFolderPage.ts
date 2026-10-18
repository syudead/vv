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

export interface SmartFolderPageData {
  folder: LibrarySmartFolder;
  /** Owner responses only. */
  settings?: SmartFolderSettings;
  trail: readonly GroupRef[];
  items: readonly LibraryItem[];
  nextCursor?: string;
  /** フォルダがホームの棚か。所有者の応答にだけ入る（docs/design-docs/home.md）。 */
  onHome?: boolean;
}

export type SmartFolderPageState = PagedDetailState<SmartFolderPageData>;

/** Deduplicate itemKey across pages because concurrent matching changes can repeat boundary entries. */
export function appendItems(
  current: readonly LibraryItem[],
  page: readonly LibraryItem[],
): LibraryItem[] {
  const known = new Set(current.map(itemKey));
  return [...current, ...page.filter((item) => !known.has(itemKey(item)))];
}

async function readItems(id: number, cursor: string, signal: AbortSignal) {
  const page = await listSmartFolderItems(id, cursor, signal);
  return { items: toLibraryItems(page.items), nextCursor: page.nextCursor };
}

/** Refresh through the prior loaded count so existing rows are not dropped. */
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
    onHome: detail.onHome,
  };
}

/**
 * Keep rows during event/reconnect refreshes and discard superseded reads. Any 404 becomes not-
 * found; other background failures retain content.
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
    // Subscribe before reading so changes after the snapshot are not lost.
    const unsubscribe = subscribeServerEvents({
      smartFolder: (ids) => {
        if (id !== null && ids.includes(id) && stateRef.current.kind !== "gone") {
          refresh();
        }
      },
      // ホームの棚の集合か並びが変わると、このフォルダの `onHome` も変わったかもしれない。
      home: () => {
        if (stateRef.current.kind === "ready") refresh();
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
 * Return ancestor rules nearest-first; version changes only for folder refreshes, not pagination.
 * Unreadable rules remain undefined for skeletons.
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

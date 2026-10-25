import { useCallback, useEffect, useState } from "react";

import type { CollectionCriteria } from "../api/collectionCriteria";
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

async function readItems(
  id: number,
  cursor: string,
  signal: AbortSignal,
  query: string,
  criteria?: CollectionCriteria,
) {
  const page = await listSmartFolderItems(
    id,
    cursor,
    signal,
    undefined,
    query,
    ...(criteria ? [criteria] : []),
  );
  return { items: toLibraryItems(page.items), nextCursor: page.nextCursor };
}

/** Refresh through the prior loaded count so existing rows are not dropped. */
async function readFolderThrough(
  id: number,
  atLeast: number,
  signal: AbortSignal,
  query: string,
  criteria?: CollectionCriteria,
): Promise<SmartFolderPageData> {
  const detail = await readSmartFolderPage(
    id,
    signal,
    undefined,
    query,
    ...(criteria ? [criteria] : []),
  );
  const { items, nextCursor } = await readThrough(
    { items: toLibraryItems(detail.items), nextCursor: detail.nextCursor },
    (cursor) => readItems(id, cursor, signal, query, criteria),
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
 * Keep rows during event/reconnect refreshes and discard superseded reads. Any 404 becomes not-
 * found; other background failures retain content.
 */
export function useSmartFolderPage(
  id: number | null,
  query = "",
  criteria?: CollectionCriteria,
) {
  const criteriaKey = JSON.stringify([query, criteria]);
  const { state, stateRef, more, read, loadMore, stop, markGone } = usePagedDetail(
    id,
    {
      read: (folderId, atLeast, signal) =>
        readFolderThrough(folderId, atLeast, signal, query, criteria),
      more: (folderId, cursor, signal) =>
        readItems(folderId, cursor, signal, query, criteria),
      append: appendItems,
    },
    criteriaKey,
  );
  const reload = useCallback(() => read(false), [read]);
  const refresh = useCallback(() => read(true), [read]);

  useEffect(() => {
    // Subscribe before reading so changes after the snapshot are not lost.
    const unsubscribe = subscribeServerEvents({
      // The peer's matches enter or leave the folder when it connects or drops.
      peerItems: () => {
        if (stateRef.current.kind === "ready") refresh();
      },
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

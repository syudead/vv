import type { DragEndEvent } from "@dnd-kit/core";
import { type MutableRefObject, useCallback, useEffect, useRef, useState } from "react";

import { type LibraryGroup, RequestFailed } from "../api/client";
import { type GroupItem, moveGroupItem, removeFromGroup } from "../api/groups";
import { errorText, t, type UiText } from "../i18n";
import { dropItem, focusAfterRemove, moveItem } from "../playlists/playlistItems";
import { useToast } from "../ui/Toast";
import type { GroupPageData } from "./useGroupPage";

/** FocusTarget は編集の後にフォーカスを移す先である（項目の `×`、見出し、「More」）。 */
export type FocusTarget = number | "title" | "more";

/**
 * isTargetGone は編集の対象（項目、`after`）がもう無いこと（`404 group_target_not_found`、
 * research.md R-12）を表すかを返す。
 */
function isTargetGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.code === "group_target_not_found"
  );
}

/** isNotFound は応答が 404（対象かグループが無い）かを返す。 */
function isNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/** withoutItems は外した項目を除き、見出しの値を外した後のグループにした中身である。 */
function withoutItems(
  data: GroupPageData,
  removed: ReadonlySet<number>,
  group: LibraryGroup,
): GroupPageData {
  return {
    ...data,
    group,
    items: data.items.filter((item) => !removed.has(item.id)),
  };
}

/** useFocusAfterEdit は窓やメニューが閉じてフォーカスを戻した後（setTimeout 0）に移す。 */
function useFocusAfterEdit() {
  const [focusTarget, setFocusTarget] = useState<FocusTarget | null>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const moreRef = useRef<HTMLButtonElement>(null);
  const removeButtons = useRef(new Map<number, HTMLButtonElement>());

  useEffect(() => {
    if (focusTarget === null) return;
    const timer = setTimeout(() => {
      if (focusTarget === "title") titleRef.current?.focus();
      else if (focusTarget === "more") moreRef.current?.focus();
      else removeButtons.current.get(focusTarget)?.focus();
      setFocusTarget(null);
    }, 0);
    return () => clearTimeout(timer);
  }, [focusTarget]);

  const removeRef = useCallback(
    (itemId: number) => (node: HTMLButtonElement | null) => {
      if (node === null) removeButtons.current.delete(itemId);
      else removeButtons.current.set(itemId, node);
    },
    [],
  );

  return { titleRef, moreRef, removeRef, focus: setFocusTarget };
}

/**
 * useGroupEdits はグループのページの項目の編集である（specs/048-groups/ui-design.md
 * 「Reordering」「Removing one item」「Body」「Edits that fail」）。
 *
 * - 並べ替えは置いたときに番号を振り直してから `move` を 1 回送る。送る要求は置いた順に
 *   1 つずつ流し、失敗したらサーバーが最後に認めた並びへ戻して知らせる。
 * - 行の `×` は確認なしにその項目を外す。外せたら行が消え、フォーカスは次の行の `×`、
 *   無ければ前の行、無ければ見出しへ移る。
 * - 選択バーの「Remove from group」は選んだ項目をまとめて外し、件数を知らせる。
 * - どれも `404 group_target_not_found` なら表示を保ったまま読み直し、変わっていたと知らせる。
 *   ほかの `404`（グループが無い）は読み直しが無くなった状態を見せる。
 */
export function useGroupEdits({
  id,
  update,
  confirmed,
  refresh,
}: {
  id: number | null;
  update: (change: (data: GroupPageData) => GroupPageData) => void;
  confirmed: MutableRefObject<readonly GroupItem[]>;
  refresh: () => void;
}) {
  const toast = useToast();
  const [removing, setRemoving] = useState<ReadonlySet<number>>(new Set());
  const [removingSelected, setRemovingSelected] = useState(false);
  const focus = useFocusAfterEdit();
  /** 送る移動を置いた順に 1 つずつ流す鎖と、失敗で残りを捨てるための世代。 */
  const moves = useRef<Promise<void>>(Promise.resolve());
  const generation = useRef(0);

  useEffect(
    () => () => {
      // 鎖で待っている移動は、画面を離れたら送らない。
      generation.current += 1;
    },
    [],
  );

  /** failed は編集の失敗を見せる。対象が消えていたら読み直す。 */
  const failed = useCallback(
    (error: unknown, message: (reason: string) => UiText) => {
      if (isTargetGone(error)) {
        refresh();
        toast(t.groups.reloaded);
        return;
      }
      if (isNotFound(error)) {
        refresh();
        return;
      }
      toast(message(errorText(error)));
    },
    [refresh, toast],
  );

  const removed = useCallback(
    (itemIds: readonly number[], group: LibraryGroup) => {
      const gone = new Set(itemIds);
      confirmed.current = confirmed.current.filter((item) => !gone.has(item.id));
      update((data) => withoutItems(data, gone, group));
    },
    [confirmed, update],
  );

  const move = useCallback(
    (event: DragEndEvent, items: readonly GroupItem[]) => {
      if (id === null || event.over === null) return;
      const itemId = Number(event.active.id);
      const drop = dropItem(items, itemId, Number(event.over.id));
      if (drop === null) return;
      update((data) => ({ ...data, items: drop.items }));
      const ticket = generation.current;
      const { afterItemId } = drop;
      moves.current = moves.current.then(async () => {
        if (ticket !== generation.current) return;
        try {
          await moveGroupItem(id, itemId, afterItemId);
          if (ticket !== generation.current) return;
          confirmed.current = moveItem(confirmed.current, itemId, afterItemId);
        } catch (error) {
          if (ticket !== generation.current) return;
          generation.current += 1;
          if (!isNotFound(error)) {
            update((data) => ({ ...data, items: confirmed.current }));
          }
          failed(error, t.groups.item.moveFailed);
        }
      });
    },
    [confirmed, failed, id, update],
  );

  const removeOne = useCallback(
    (item: GroupItem, items: readonly GroupItem[]) => {
      if (id === null || removing.has(item.id)) return;
      setRemoving((previous) => new Set(previous).add(item.id));
      const done = () =>
        setRemoving((previous) => {
          const next = new Set(previous);
          next.delete(item.id);
          return next;
        });
      removeFromGroup(id, [item.id]).then(
        (group) => {
          done();
          const target = focusAfterRemove(items, item.id);
          removed([item.id], group);
          focus.focus(target ?? "title");
        },
        (error: unknown) => {
          done();
          failed(error, t.groups.item.removeFailed);
        },
      );
    },
    [failed, focus, id, removed, removing],
  );

  const removeSelected = useCallback(
    (itemIds: readonly number[], name: string, onRemoved: () => void) => {
      if (id === null || itemIds.length === 0 || removingSelected) return;
      setRemovingSelected(true);
      removeFromGroup(id, itemIds).then(
        (group) => {
          setRemovingSelected(false);
          removed(itemIds, group);
          onRemoved();
          toast(t.groups.removed(itemIds.length, name));
        },
        (error: unknown) => {
          setRemovingSelected(false);
          failed(error, t.groups.item.removeFailed);
        },
      );
    },
    [failed, id, removed, removingSelected, toast],
  );

  return {
    removing,
    removingSelected,
    move,
    removeOne,
    removeSelected,
    titleRef: focus.titleRef,
    moreRef: focus.moreRef,
    removeRef: focus.removeRef,
    focus: focus.focus,
  };
}

import { type MutableRefObject, useCallback, useEffect, useRef, useState } from "react";

import { type LibraryGroup, RequestFailed } from "../api/client";
import { type GroupItem, removeFromGroup } from "../api/groups";
import { mergeGroups, moveCards, type OrganizeRef, reorderGroup } from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { focusAfterRemove } from "../playlists/playlistItems";
import { useToast } from "../ui/Toast";
import { moveAfter } from "../videoList/cardDragRules";
import type { UndoToast } from "../videoList/undoToast";
import type { GroupDestination, MoveDestination } from "./GroupDrag";
import type { GroupPageData } from "./useGroupPage";

export type FocusTarget = number | "title" | "more";

/** The server distinguishes target 404s through reason; missing groups have no reason. */
function isTargetGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "group_target_not_found"
  );
}

function isNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

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

function refOf(item: GroupItem): OrganizeRef[] {
  if (item.kind === "group") {
    return item.group === undefined ? [] : [{ kind: "group", id: item.group.id }];
  }
  return item.video === undefined ? [] : [{ kind: "video", id: item.video.id }];
}

/** The optimistic descendant count is approximate; refresh supplies the authoritative value. */
function withoutMoved(data: GroupPageData, moved: readonly GroupItem[]): GroupPageData {
  const gone = new Set(moved.map((item) => item.id));
  const lost = moved.reduce(
    (sum, item) => sum + (item.kind === "group" ? (item.group?.videoCount ?? 0) : 1),
    0,
  );
  return {
    ...data,
    group: { ...data.group, videoCount: Math.max(0, data.group.videoCount - lost) },
    items: data.items.filter((item) => !gone.has(item.id)),
  };
}

/** Defer focus until dialogs and menus finish restoring their own focus. */
export function useFocusAfterEdit() {
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
 * Serialize optimistic edits with page refreshes; restore the last confirmed state on failure and
 * discard queued moves. Removal invalidates positional cursors, while removing the last item
 * dissolves the group; see docs/design-docs/catalog-organization.md.
 */
export function useGroupEdits({
  id,
  update,
  confirmed,
  moves,
  movesSent,
  refresh,
  onDissolved,
  undo,
}: {
  id: number | null;
  update: (change: (data: GroupPageData) => GroupPageData) => void;
  confirmed: MutableRefObject<readonly GroupItem[]>;
  moves: MutableRefObject<Promise<void>>;
  movesSent: MutableRefObject<number>;
  refresh: () => void;
  /** Called when removing the final item dissolves the group. */
  onDissolved: () => void;
  undo: UndoToast;
}) {
  const toast = useToast();
  const [removing, setRemoving] = useState<ReadonlySet<number>>(new Set());
  const [removingSelected, setRemovingSelected] = useState(false);
  const focus = useFocusAfterEdit();
  /** Track queue generation and pending moves so failures can discard the remainder. */
  const generation = useRef(0);
  const queued = useRef(0);

  useEffect(
    () => () => {
      // Do not send queued moves after leaving the page.
      generation.current += 1;
    },
    [],
  );

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
      let paged = false;
      update((data) => {
        paged = data.nextCursor !== undefined;
        return withoutItems(data, gone, group);
      });
      // Removal compacts positions, so an old positional cursor would skip unseen items.
      if (paged) refresh();
    },
    [confirmed, refresh, update],
  );

  /**
   * Append to the move queue; stop updating after failure or unmount. A failure discards the
   * remainder and calls onError.
   */
  const enqueue = useCallback(
    (
      step: (alive: () => boolean) => Promise<void>,
      onError: (error: unknown) => void,
    ) => {
      const ticket = generation.current;
      const alive = () => ticket === generation.current;
      queued.current += 1;
      moves.current = moves.current.then(async () => {
        try {
          if (!alive()) return;
          movesSent.current += 1;
          await step(alive);
        } catch (error) {
          if (!alive()) return;
          generation.current += 1;
          onError(error);
        } finally {
          queued.current -= 1;
        }
      });
    },
    [moves, movesSent],
  );

  /** Restore confirmed items and the current group header unless the group itself is missing. */
  const restore = useCallback(
    (message: (reason: string) => UiText, group: () => LibraryGroup | undefined) =>
      (error: unknown) => {
        if (!isNotFound(error)) {
          const shown = group();
          update((data) => ({
            ...data,
            group: shown ?? data.group,
            items: confirmed.current,
          }));
        }
        failed(error, message);
      },
    [confirmed, failed, update],
  );

  const { offer, carry } = undo;
  const reorder = useCallback(
    (itemIds: readonly number[], after: number | null, name: string) => {
      if (id === null || itemIds.length === 0) return;
      const moved = new Set(itemIds);
      const byId = (item: GroupItem) => item.id;
      update((data) => ({ ...data, items: moveAfter(data.items, moved, after, byId) }));
      enqueue(
        async (alive) => {
          const { undoToken } = await reorderGroup(id, itemIds, after);
          if (!alive()) return;
          confirmed.current = moveAfter(confirmed.current, moved, after, byId);
          if (queued.current === 1)
            update((data) => ({ ...data, items: confirmed.current }));
          offer(t.organize.done.reordered(name), undoToken);
        },
        restore(t.organize.failed.reorder, () => undefined),
      );
    },
    [confirmed, enqueue, id, offer, restore, update],
  );

  const move = useCallback(
    (moved: readonly GroupItem[], to: MoveDestination, name: string) => {
      if (id === null || moved.length === 0) return;
      const gone = new Set(moved.map((item) => item.id));
      const shown: { group?: LibraryGroup; whole: boolean } = { whole: false };
      update((data) => {
        shown.group = data.group;
        shown.whole =
          data.nextCursor === undefined && data.items.every((item) => gone.has(item.id));
        return withoutMoved(data, moved);
      });
      const message =
        to.kind === "group"
          ? t.organize.done.moved(moved.length, to.name)
          : t.organize.done.movedOut(moved.length, name);
      enqueue(
        async (alive) => {
          const { undoToken } = await moveCards(moved.flatMap(refOf), {
            from: id,
            to: to.kind === "group" ? to.id : null,
          });
          if (!alive()) return;
          confirmed.current = confirmed.current.filter((item) => !gone.has(item.id));
          offer(message, undoToken);
          if (!shown.whole) {
            refresh();
            return;
          }
          carry();
          onDissolved();
        },
        restore(t.organize.failed.move, () => shown.group),
      );
    },
    [carry, confirmed, enqueue, id, offer, onDissolved, refresh, restore, update],
  );

  const merge = useCallback(
    (merged: readonly GroupItem[], into: GroupDestination) => {
      if (id === null || merged.length === 0) return;
      const gone = new Set(merged.map((item) => item.id));
      const keep = (item: GroupItem) => !gone.has(item.id);
      const shown: { group?: LibraryGroup } = {};
      update((data) => {
        shown.group = data.group;
        return withoutMoved(data, merged);
      });
      const message = t.organize.done.merged(
        merged.length,
        into.name,
        merged.some((item) => item.kind === "video"),
      );
      enqueue(
        async (alive) => {
          const { undoToken } = await mergeGroups(merged.flatMap(refOf), into.id, id);
          if (!alive()) return;
          confirmed.current = confirmed.current.filter(keep);
          offer(message, undoToken);
          refresh();
        },
        restore(t.organize.failed.merge, () => shown.group),
      );
    },
    [confirmed, enqueue, id, offer, refresh, restore, update],
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
          if (group === null) {
            onDissolved();
            return;
          }
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
    [failed, focus, id, onDissolved, removed, removing],
  );

  const removeSelected = useCallback(
    (itemIds: readonly number[], name: string, onRemoved: () => void) => {
      if (id === null || itemIds.length === 0 || removingSelected) return;
      setRemovingSelected(true);
      removeFromGroup(id, itemIds).then(
        (group) => {
          setRemovingSelected(false);
          if (group === null) {
            onDissolved();
            return;
          }
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
    [failed, id, onDissolved, removed, removingSelected, toast],
  );

  return {
    removing,
    removingSelected,
    reorder,
    move,
    merge,
    removeOne,
    removeSelected,
    titleRef: focus.titleRef,
    moreRef: focus.moreRef,
    removeRef: focus.removeRef,
    focus: focus.focus,
  };
}

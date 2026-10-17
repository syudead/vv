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

/** FocusTarget は編集の後にフォーカスを移す先である（項目の `×`、見出し、「More」）。 */
export type FocusTarget = number | "title" | "more";

/**
 * isTargetGone は編集の対象（項目、`after`）がもう無いこと（`404 group_target_not_found`）を表すかを返す。サーバーは `code` を `not_found` にし、どの 404 かを
 * `reason` に入れる（グループそのものが無いときは `reason` が無い）。
 */
function isTargetGone(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    error.status === 404 &&
    error.reason === "group_target_not_found"
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

/** refOf は項目を移す操作で指す値（動画かグループの id）である。 */
function refOf(item: GroupItem): OrganizeRef[] {
  if (item.kind === "group") {
    return item.group === undefined ? [] : [{ kind: "group", id: item.group.id }];
  }
  return item.video === undefined ? [] : [{ kind: "video", id: item.video.id }];
}

/**
 * withoutMoved は移した項目を除き、見出しの本数をその分だけ減らした中身である。本数は
 * 子孫の動画を数えたものなので目安で、移した後の読み直しが正しい値にする。
 */
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

/**
 * useFocusAfterEdit は窓やメニューが閉じてフォーカスを戻した後（setTimeout 0）に移す。グループの
 * ページとスマートフォルダのページ（直した後の「More」）が共有する。
 */
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
 * useGroupEdits はグループのページの項目の編集である。
 *
 * - 並べ替えは置いたときに番号を振り直してから、持ち上げた項目を画面の順に `reorder` で
 *   1 回送る。送る要求は置いた順に
 *   1 つずつ流し（鎖は useGroupPage と共有し、静かな読み直しは鎖が答えてから読む）、
 *   受け入れられたら「Undo」付きの通知を出し、失敗したらサーバーが最後に認めた並びへ戻して
 *   知らせる。鎖が空になったら、認めた並びを見せる（途中の読み直しが移動の前の並びを置いても、
 *   行を正しい順に戻す）。
 * - 行の `×` は確認なしにその項目を外す。外せたら行が消え、フォーカスは次の行の `×`、
 *   無ければ前の行、無ければ見出しへ移る。続きのページが残っていたら、サーバーが位置を
 *   詰めて手元の続き位置が使えなくなるので、先頭から読み直す。
 * - ドラッグで子グループ・祖先・ライブラリへ移す項目は、置いたときに画面から外してから
 *   `move` で 1 回送る（並べ替えと同じ鎖）。受け入れられたら「Undo」付きの通知を出して
 *   読み直し、失敗したら外す前に見せていた項目と本数に戻して知らせる。すべての項目を
 *   移すとサーバーがグループを解散するので、通知とトークンを持ち越して `onDissolved` へ渡す。
 * - 子グループを含む項目を子グループへマージすると、持ち上げた項目（動画と子グループの両方）を
 *   画面から外してから、開いているグループを `from` に付けて `merge` で 1 回送る（並べ替えと
 *   同じ鎖）。サーバーは持ち上げた動画とグループを `from` から外す。受け入れられたら「Undo」
 *   付きの通知を出して読み直し、失敗したら外す前の項目に戻して知らせる。
 * - 選択バーの「Remove from group」は選んだ項目をまとめて外し、件数を知らせる。
 * - 最後の項目を外すとサーバーがグループを解散する（`204`）。外した行は
 *   見せず、`onDissolved` で画面を解散と同じ結末（道筋の親へ移ってトースト）へ渡す。
 * - どれも `404 group_target_not_found` なら表示を保ったまま読み直し、変わっていたと知らせる。
 *   ほかの `404`（グループが無い）は読み直しが無くなった状態を見せる。
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
  /** 最後の項目を外してグループが解散されたときに呼ぶ。 */
  onDissolved: () => void;
  /** 受け入れられた並べ替えの取り消しの通知。 */
  undo: UndoToast;
}) {
  const toast = useToast();
  const [removing, setRemoving] = useState<ReadonlySet<number>>(new Set());
  const [removingSelected, setRemovingSelected] = useState(false);
  const focus = useFocusAfterEdit();
  /** 失敗で鎖の残りを捨てるための世代と、鎖で待っているか送っている移動の数。 */
  const generation = useRef(0);
  const queued = useRef(0);

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
      let paged = false;
      update((data) => {
        paged = data.nextCursor !== undefined;
        return withoutItems(data, gone, group);
      });
      // サーバーは外した後に位置を詰める。位置で決まる続き位置は、読んでいない項目を飛ばす。
      if (paged) refresh();
    },
    [confirmed, refresh, update],
  );

  /**
   * enqueue は移動 1 つを鎖の最後に足す。`step` は送って結果を見せ、`alive` が false に
   * なった（失敗か画面を離れて鎖の残りを捨てた）後は何も見せない。失敗したら鎖の残りを
   * 捨てて `onError` を呼ぶ。
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

  /**
   * restore は送った編集の失敗を見せる。グループが無い（`404`）のでなければ、項目をサーバーが
   * 最後に認めたものに、見出しを `group()`（無ければ今のまま）に戻す。
   */
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

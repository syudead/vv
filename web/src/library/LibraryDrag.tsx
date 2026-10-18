import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { LibraryGroup, LibraryItem } from "../api/client";
import {
  groupKey,
  isOrganizable,
  isSmartFolderKey,
  itemKey,
  type OrganizableItem,
  videoKey,
} from "../api/libraryItems";
import {
  mergeGroups,
  moveCards,
  type OrganizeRef,
  type OrganizeResult,
} from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import {
  type CardAttachment,
  type CardDrop,
  CardDragLayer,
  type HeldDrop,
  useCardDrag,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer } from "../videoList/cardDragRules";
import { useGroupDrops } from "../videoList/dropMenu";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { useUndoToast } from "../videoList/undoToast";
import { VideoRow, type VideoCardProps } from "../videoList/VideoCard";
import { type GroupCardProps, GroupRow } from "./GroupCard";
import { favoriteTargets, type LibrarySelection } from "./selection";

// Library cards have only onto drops, not reorder zones; group-containing drops require a move/merge choice.

/** Lift selected group cards plus videos not already represented by a selected group. */
export function selectedKeys(selection: LibrarySelection): string[] {
  const { videoIds, groupIds } = favoriteTargets(selection);
  return [...groupIds.map(groupKey), ...videoIds.map(videoKey)];
}

function isLibraryGroup(item: LibraryItem | undefined): boolean {
  return item?.kind === "group";
}

function libraryGroupOf(item: LibraryItem | undefined): LibraryGroup | null {
  return item?.kind === "group" ? item.group : null;
}

function refOf(item: OrganizableItem): OrganizeRef {
  return item.kind === "video"
    ? { kind: "video", id: item.video.id }
    : { kind: "group", id: item.group.id };
}

/** Lift selected loaded cards in display order; group-containing drops offer move or merge. */
export function LibraryDragLayer({
  items,
  list,
  selection,
  onto: ontoVideo,
  onDrop,
  onMove,
  onMerge,
  onLift,
  held,
  attached,
  children,
}: {
  items: readonly LibraryItem[];
  list: boolean;
  selection: LibrarySelection;
  onto: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  onDrop: (drop: CardDrop) => void;
  onMove: (moved: readonly OrganizableItem[], to: LibraryGroup) => void;
  onMerge: (merged: readonly OrganizableItem[], into: LibraryGroup) => void;
  onLift: () => void;
  held: HeldDrop | null;
  attached: CardAttachment | null;
  children: ReactNode;
}) {
  // Smart folders are neither draggable nor valid drop targets.
  const organizable = useMemo(() => items.filter(isOrganizable), [items]);
  const { order, byKey, nameOf, preview } = useOrderedItemDrag(
    organizable,
    itemKey,
    list,
  );
  const selected = useMemo(() => selectedKeys(selection), [selection]);
  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selected),
    [order, selected],
  );
  const groups = useGroupDrops({
    byKey,
    groupOf: libraryGroupOf,
    isGroup: isLibraryGroup,
    nameOf,
    onMove,
    onMerge,
  });
  const { onto: ontoGroup, drop: dropOnGroup } = groups;
  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> => {
      if (isSmartFolderKey(target)) return "refused";
      return isLibraryGroup(byKey.get(target))
        ? ontoGroup(target, lifted)
        : ontoVideo(target, lifted);
    },
    [byKey, ontoGroup, ontoVideo],
  );
  const drop = useCallback(
    (drop: CardDrop) => {
      const onGroup =
        drop.sign.kind === "onto" && isLibraryGroup(byKey.get(drop.sign.key));
      if (onGroup) dropOnGroup(drop);
      else onDrop(drop);
    },
    [byKey, dropOnGroup, onDrop],
  );

  return (
    <CardDragLayer
      order={order}
      liftedFor={liftedFor}
      reorder={false}
      onto={onto}
      nameOf={nameOf}
      preview={preview}
      list={list}
      onDrop={drop}
      onLift={onLift}
      instructions={t.organize.instructions}
      held={held ?? groups.held}
      attached={attached ?? groups.attached}
    >
      {children}
    </CardDragLayer>
  );
}

export function DraggableVideoRow(props: Omit<VideoCardProps, "drag">) {
  const drag = useCardDrag(videoKey(props.video.id));
  return <VideoRow {...props} drag={drag} />;
}

export function DraggableGroupRow(props: Omit<GroupCardProps, "drag">) {
  const drag = useCardDrag(groupKey(props.group.id));
  return <GroupRow {...props} drag={drag} />;
}

const noKeys: ReadonlySet<string> = new Set();

/** Release accepted or absent hidden items; a refreshed server list is authoritative even when moved videos still appear. */
function pruned(
  hidden: ReadonlySet<string>,
  items: readonly LibraryItem[],
  released: ReadonlySet<string>,
) {
  if (hidden.size === 0) return hidden;
  const present = new Set(items.map(itemKey));
  const kept = new Set(
    Array.from(hidden).filter((key) => present.has(key) && !released.has(key)),
  );
  return kept.size === hidden.size ? hidden : kept;
}

function without(hidden: ReadonlySet<string>, keys: readonly string[]) {
  return new Set(Array.from(hidden).filter((key) => !keys.includes(key)));
}

interface LibraryOperation {
  send: () => Promise<OrganizeResult>;
  done: UiText;
  failed: (reason: string) => UiText;
}

/** Hide cards immediately and serialize moves so Undo belongs to the latest drop. Restore failures, trust refreshed membership, and suppress notifications after leaving. */
export function useLibraryMoves(
  items: readonly LibraryItem[],
  refreshInPlace: () => void,
  clearSelection: () => void,
) {
  const toast = useToast();
  const [hidden, setHidden] = useState<ReadonlySet<string>>(noKeys);
  const shown = useRef(true);
  const chain = useRef<Promise<void>>(Promise.resolve());
  const pending = useRef(new Set<string>());
  const released = useRef(new Set<string>());
  useEffect(() => {
    shown.current = true;
    return () => {
      shown.current = false;
    };
  }, []);

  useEffect(() => {
    const done = released.current;
    released.current = new Set();
    setHidden((current) => pruned(current, items, done));
  }, [items]);

  const afterUndo = useCallback(() => {
    setHidden(pending.current.size === 0 ? noKeys : new Set(pending.current));
    refreshInPlace();
  }, [refreshInPlace]);
  const { offer } = useUndoToast(afterUndo);

  const run = useCallback(
    (moved: readonly OrganizableItem[], operation: LibraryOperation) => {
      if (moved.length === 0) return;
      const keys = moved.map(itemKey);
      for (const key of keys) pending.current.add(key);
      const settle = () => {
        for (const key of keys) pending.current.delete(key);
      };
      setHidden((current) => new Set([...current, ...keys]));
      clearSelection();
      chain.current = chain.current.then(() =>
        operation.send().then(
          ({ undoToken }) => {
            settle();
            for (const key of keys) released.current.add(key);
            if (!shown.current) return;
            offer(operation.done, undoToken);
            refreshInPlace();
          },
          (error: unknown) => {
            settle();
            if (!shown.current) return;
            setHidden((current) => without(current, keys));
            toast(operation.failed(errorText(error)));
          },
        ),
      );
    },
    [clearSelection, offer, refreshInPlace, toast],
  );

  const move = useCallback(
    (moved: readonly OrganizableItem[], to: LibraryGroup) =>
      run(moved, {
        send: () => moveCards(moved.map(refOf), { from: null, to: to.id }),
        done: t.organize.done.moved(moved.length, to.name),
        failed: t.organize.failed.move,
      }),
    [run],
  );
  const merge = useCallback(
    (merged: readonly OrganizableItem[], into: LibraryGroup) =>
      run(merged, {
        send: () => mergeGroups(merged.map(refOf), into.id, null),
        done: t.organize.done.merged(
          merged.length,
          into.name,
          merged.some((item) => item.kind === "video"),
        ),
        failed: t.organize.failed.merge,
      }),
    [run],
  );

  const visible = useMemo(
    () =>
      hidden.size === 0 ? items : items.filter((item) => !hidden.has(itemKey(item))),
    [hidden, items],
  );
  return { visible, move, merge };
}

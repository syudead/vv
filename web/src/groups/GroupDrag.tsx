import { type ReactNode, useCallback, useMemo } from "react";

import type { GroupItem, GroupRef } from "../api/groups";
import { t, type UiText } from "../i18n";
import {
  type CardAttachment,
  type CardDrop,
  CardDragLayer,
  crumbIndexOf,
  type HeldDrop,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer, reorderDrop } from "../videoList/cardDragRules";
import { orderedItemTitle } from "../videoList/ItemRowCells";
import { useGroupDrops } from "../videoList/dropMenu";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { reorderAnnouncements } from "../videoList/reorder";

export function itemKey(item: GroupItem): string {
  return String(item.id);
}

export interface GroupDropOnto {
  onto: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  onDrop: (drop: CardDrop) => void;
  held: HeldDrop | null;
  attached: CardAttachment | null;
}

export type MoveDestination = GroupDestination | { kind: "library" };

export interface GroupDestination {
  kind: "group";
  id: number;
  name: string;
}

/** Crumb zero is Library; ancestors follow. Reject the current group and absent crumbs. */
export function destinationOfCrumb(
  index: number,
  trail: readonly GroupRef[],
): MoveDestination | null {
  if (index === 0) return { kind: "library" };
  const ancestor = trail[index - 1];
  return ancestor === undefined ? null : { kind: "group", ...ancestor };
}

function crumbName(index: number | null, trail: readonly GroupRef[], name: string) {
  if (index === null) return "";
  if (index === 0) return t.library.title;
  return trail[index - 1]?.name ?? name;
}

function isChildGroup(item: GroupItem | undefined): boolean {
  return item?.kind === "group";
}

function groupDestination(item: GroupItem | undefined): GroupDestination | null {
  const group = item?.group;
  return group === undefined ? null : { kind: "group", id: group.id, name: group.name };
}

/**
 * Preserve display order when dragging selected items; exclude direct videos covered by selected
 * child groups. Gaps reorder, cards move or merge, and ancestor crumbs move outward; reject the
 * current crumb and collapsed ellipsis.
 */
export function GroupDragLayer({
  items,
  list,
  trail,
  name,
  selectedItems,
  onReorder,
  onMove,
  onMerge,
  onLift,
  dropOnto,
  children,
}: {
  items: readonly GroupItem[];
  list: boolean;
  /** Ancestors from outermost to immediate parent. */
  trail: readonly GroupRef[];
  name: string;
  /** Selected item IDs in display order, partitioned like Remove from group. */
  selectedItems: readonly number[];
  /** Move itemIds in display order after after; null means the beginning. */
  onReorder: (itemIds: readonly number[], after: number | null) => void;
  onMove: (moved: readonly GroupItem[], to: MoveDestination) => void;
  /** Merge ordered items, including child groups, into the target child group. */
  onMerge: (merged: readonly GroupItem[], into: GroupDestination) => void;
  onLift: () => void;
  dropOnto?: GroupDropOnto;
  children: ReactNode;
}) {
  const {
    order,
    byKey,
    nameOf: itemName,
    preview,
  } = useOrderedItemDrag(items, itemKey, list);

  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selectedItems.map(String)),
    [order, selectedItems],
  );
  const nameOf = useCallback(
    (key: string) =>
      byKey.has(key) ? itemName(key) : crumbName(crumbIndexOf(key), trail, name),
    [byKey, itemName, name, trail],
  );
  const groups = useGroupDrops({
    byKey,
    groupOf: groupDestination,
    isGroup: isChildGroup,
    nameOf,
    onMove,
    onMerge,
  });
  const { onto: ontoGroup, drop: dropOnGroup } = groups;
  const { onto: ontoVideo } = dropOnto ?? {};
  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> =>
      groupDestination(byKey.get(target)) === null
        ? (ontoVideo?.(target, lifted) ?? null)
        : ontoGroup(target, lifted),
    [byKey, ontoGroup, ontoVideo],
  );
  const drop = useCallback(
    ({ lifted, sign }: CardDrop) => {
      const moved = lifted.flatMap((key) => byKey.get(key) ?? []);
      if (sign.kind === "onto") {
        if (groupDestination(byKey.get(sign.key)) !== null) dropOnGroup({ lifted, sign });
        else dropOnto?.onDrop({ lifted, sign });
        return;
      }
      if (sign.kind === "crumb") {
        const index = crumbIndexOf(sign.key);
        const to = index === null ? null : destinationOfCrumb(index, trail);
        if (to !== null) onMove(moved, to);
        return;
      }
      const result = reorderDrop(order, new Set(lifted), sign.key, sign.side);
      if (result === null) return;
      onReorder(
        result.moved.map(Number),
        result.after === null ? null : Number(result.after),
      );
    },
    [byKey, dropOnGroup, dropOnto, onMove, onReorder, order, trail],
  );
  const handle = useMemo(
    () =>
      list
        ? {
            announcements: reorderAnnouncements(items, orderedItemTitle, t.groups.item),
          }
        : undefined,
    [items, list],
  );

  return (
    <CardDragLayer
      order={order}
      liftedFor={liftedFor}
      reorder
      onto={onto}
      nameOf={nameOf}
      preview={preview}
      list={list}
      onDrop={drop}
      onLift={onLift}
      handle={handle}
      instructions={t.groups.item.reorderInstructions}
      held={dropOnto?.held ?? groups.held}
      attached={dropOnto?.attached ?? groups.attached}
    >
      {children}
    </CardDragLayer>
  );
}

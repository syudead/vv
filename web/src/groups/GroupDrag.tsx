import { type ReactNode, useCallback, useMemo } from "react";

import type { GroupItem } from "../api/groups";
import { t, type UiText } from "../i18n";
import {
  type CardAttachment,
  type CardDrop,
  CardDragLayer,
  type HeldDrop,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer, reorderDrop } from "../videoList/cardDragRules";
import { orderedItemTitle } from "../videoList/ItemRowCells";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { reorderAnnouncements } from "../videoList/reorder";

/** itemKey は項目のドラッグの鍵である（項目の id）。 */
export function itemKey(item: GroupItem): string {
  return String(item.id);
}

/** GroupDropOnto はカードの上に落とす規則と、そこへ落としたときの画面の扱いである。 */
export interface GroupDropOnto {
  onto: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  onDrop: (drop: CardDrop) => void;
  held: HeldDrop | null;
  attached: CardAttachment | null;
}

/**
 * GroupDragLayer はグループのページの所有者の格子とリストを包むドラッグの層である
 * （specs/050-drag-organize/ui-design.md「Group page」）。カードか行の間に落とすと、持ち上げた
 * 項目を画面の順のまま、その間の前の項目（先頭なら無し）の後ろへ並べ替える。カードの上に
 * 落とせるか、落としたら何をするかは `dropOnto` が決める。選択した項目
 * （「Remove from group」と同じ分け方。選んだ子グループのメンバーの動画の直接の項目は、
 * チェックが見えていても入らない）を持ち上げると、選択したすべてがいっしょに動く。
 */
export function GroupDragLayer({
  items,
  list,
  selectedItems,
  onReorder,
  onLift,
  dropOnto,
  children,
}: {
  items: readonly GroupItem[];
  list: boolean;
  /** 選択した項目の id（画面の順、「Remove from group」と同じ分け方）。 */
  selectedItems: readonly number[];
  /** 項目 `itemIds` を画面の順のまま項目 `after`（null は先頭）の後ろへ移す。 */
  onReorder: (itemIds: readonly number[], after: number | null) => void;
  onLift: () => void;
  dropOnto?: GroupDropOnto;
  children: ReactNode;
}) {
  const { order, nameOf, preview } = useOrderedItemDrag(items, itemKey, list);

  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selectedItems.map(String)),
    [order, selectedItems],
  );
  const drop = useCallback(
    ({ lifted, sign }: CardDrop) => {
      if (sign.kind !== "between") {
        dropOnto?.onDrop({ lifted, sign });
        return;
      }
      const result = reorderDrop(order, new Set(lifted), sign.key, sign.side);
      if (result === null) return;
      onReorder(
        result.moved.map(Number),
        result.after === null ? null : Number(result.after),
      );
    },
    [dropOnto, onReorder, order],
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
      nameOf={nameOf}
      preview={preview}
      list={list}
      onDrop={drop}
      onLift={onLift}
      handle={handle}
      instructions={t.groups.item.reorderInstructions}
      onto={dropOnto?.onto}
      held={dropOnto?.held}
      attached={dropOnto?.attached}
    >
      {children}
    </CardDragLayer>
  );
}

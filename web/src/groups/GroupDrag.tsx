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

/** itemKey は項目のドラッグの鍵である（項目の id）。 */
export function itemKey(item: GroupItem): string {
  return String(item.id);
}

/** GroupDropOnto は動画のカードの上に落とす規則と、そこへ落としたときの画面の扱いである。 */
export interface GroupDropOnto {
  onto: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  onDrop: (drop: CardDrop) => void;
  held: HeldDrop | null;
  attached: CardAttachment | null;
}

/** MoveDestination はグループのページから項目を移す先である。 */
export type MoveDestination = GroupDestination | { kind: "library" };

/** GroupDestination は移す先かマージ先のグループである。 */
export interface GroupDestination {
  kind: "group";
  id: number;
  name: string;
}

/**
 * destinationOfCrumb は道筋の段 `index`（0 は「Library」、続いて道筋の祖先、最後は開いている
 * グループ）に落としたときの移す先を返す。開いているグループの段と、無い段は null。
 */
export function destinationOfCrumb(
  index: number,
  trail: readonly GroupRef[],
): MoveDestination | null {
  if (index === 0) return { kind: "library" };
  const ancestor = trail[index - 1];
  return ancestor === undefined ? null : { kind: "group", ...ancestor };
}

/** crumbName は道筋の段 `index` の名前である（読み上げ用）。 */
function crumbName(index: number | null, trail: readonly GroupRef[], name: string) {
  if (index === null) return "";
  if (index === 0) return t.library.title;
  return trail[index - 1]?.name ?? name;
}

/** isChildGroup は項目が子グループかを返す。 */
function isChildGroup(item: GroupItem | undefined): boolean {
  return item?.kind === "group";
}

/** groupDestination は子グループの項目 `item` を移す先にした値である。子グループでなければ null。 */
function groupDestination(item: GroupItem | undefined): GroupDestination | null {
  const group = item?.group;
  return group === undefined ? null : { kind: "group", id: group.id, name: group.name };
}

/**
 * GroupDragLayer はグループのページの所有者の画面（道筋の帯、格子、リスト）を包むドラッグの
 * 層である。
 *
 * - カードか行の間に落とすと、持ち上げた項目を画面の順のまま、その間の前の項目（先頭なら
 *   無し）の後ろへ並べ替える。
 * - 動画だけを子グループのカードの上に落とすと、その子グループへ移す（「Put in」）。
 * - 子グループを含む項目を子グループのカードの上に落とすと、「Put in as a child group」
 *   （その子グループへ移す）と「Merge」（その子グループへマージする）のメニューを開く。
 * - 動画のカードの上に落とせるか、落としたら何をするかは `dropOnto` が決める
 *   （「Make a group」「Bundle as versions」のメニュー）。
 * - 道筋の「Library」の段に落とすとライブラリへ出し、祖先の段に落とすとその祖先へ移す。
 *   開いているグループの段と畳んだ「…」は拒否。
 *
 * 選択した項目（「Remove from group」と同じ分け方。選んだ子グループのメンバーの動画の直接の
 * 項目は、チェックが見えていても入らない）を持ち上げると、選択したすべてがいっしょに動く。
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
  /** 開いているグループの道筋（外側から、最後は親）。 */
  trail: readonly GroupRef[];
  /** 開いているグループの名前。 */
  name: string;
  /** 選択した項目の id（画面の順、「Remove from group」と同じ分け方）。 */
  selectedItems: readonly number[];
  /** 項目 `itemIds` を画面の順のまま項目 `after`（null は先頭）の後ろへ移す。 */
  onReorder: (itemIds: readonly number[], after: number | null) => void;
  /** 項目 `moved`（画面の順）を `to` へ移す。 */
  onMove: (moved: readonly GroupItem[], to: MoveDestination) => void;
  /** 項目 `merged`（画面の順、子グループを含む）を子グループ `into` へマージする。 */
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

import { type ReactNode, useCallback, useMemo } from "react";

import type { GroupItem } from "../api/groups";
import { t } from "../i18n";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import { CardDragFace, type CardDrop, CardDragLayer } from "../videoList/cardDrag";
import { liftedKeys, reorderDrop } from "../videoList/cardDragRules";
import FolderArt from "../videoList/FolderArt";
import { orderedItemTitle, rowFacts } from "../videoList/ItemRowCells";
import { reorderAnnouncements } from "../videoList/reorder";

/** itemKey は項目のドラッグの鍵である（項目の id）。 */
export function itemKey(item: GroupItem): string {
  return String(item.id);
}

/**
 * ItemFace は持ち上げた項目の写しの中身である。格子では動画のサムネイルか子グループの
 * フォルダの絵柄、リストでは行のサムネイル（ui-design.md「Lifting」）。
 */
function ItemFace({ item, list }: { item: GroupItem; list: boolean }) {
  const title = orderedItemTitle(item);
  if (!list && item.kind === "group" && item.group !== undefined) {
    return (
      <CardDragFace
        title={title}
        thumbnail={
          <VideoThumbnail>
            <FolderArt previews={item.group.previews} />
          </VideoThumbnail>
        }
      />
    );
  }
  const { thumbnailUrl } = rowFacts(item);
  return (
    <CardDragFace
      title={title}
      thumbnail={
        <VideoThumbnail>
          {thumbnailUrl !== undefined && <VideoThumbnailImage src={thumbnailUrl} />}
        </VideoThumbnail>
      }
    />
  );
}

/**
 * GroupDragLayer はグループのページの所有者の格子とリストを包むドラッグの層である
 * （specs/050-drag-organize/ui-design.md「Group page」）。カードか行の間に落とすと、持ち上げた
 * 項目を画面の順のまま、その間の前の項目（先頭なら無し）の後ろへ並べ替える。選択した項目
 * （「Remove from group」と同じ分け方。選んだ子グループのメンバーの動画の直接の項目は、
 * チェックが見えていても入らない）を持ち上げると、選択したすべてがいっしょに動く。
 */
export function GroupDragLayer({
  items,
  list,
  selectedItems,
  onReorder,
  onLift,
  children,
}: {
  items: readonly GroupItem[];
  list: boolean;
  /** 選択した項目の id（画面の順、「Remove from group」と同じ分け方）。 */
  selectedItems: readonly number[];
  /** 項目 `itemIds` を画面の順のまま項目 `after`（null は先頭）の後ろへ移す。 */
  onReorder: (itemIds: readonly number[], after: number | null) => void;
  onLift: () => void;
  children: ReactNode;
}) {
  const order = useMemo(() => items.map(itemKey), [items]);
  const byKey = useMemo(
    () => new Map(items.map((item) => [itemKey(item), item])),
    [items],
  );

  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selectedItems.map(String)),
    [order, selectedItems],
  );
  const nameOf = useCallback(
    (key: string) => {
      const item = byKey.get(key);
      return item === undefined ? "" : orderedItemTitle(item);
    },
    [byKey],
  );
  const preview = useCallback(
    (key: string) => {
      const item = byKey.get(key);
      return item === undefined ? null : <ItemFace item={item} list={list} />;
    },
    [byKey, list],
  );
  const drop = useCallback(
    ({ lifted, sign }: CardDrop) => {
      if (sign.kind !== "between") return;
      const result = reorderDrop(order, new Set(lifted), sign.key, sign.side);
      if (result === null) return;
      onReorder(
        result.moved.map(Number),
        result.after === null ? null : Number(result.after),
      );
    },
    [onReorder, order],
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
    >
      {children}
    </CardDragLayer>
  );
}

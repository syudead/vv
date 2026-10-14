import { type ReactNode, useCallback, useMemo } from "react";

import type { GroupItem, GroupRef } from "../api/groups";
import { t, type UiText } from "../i18n";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import {
  CardDragFace,
  type CardDrop,
  CardDragLayer,
  crumbIndexOf,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer, reorderDrop } from "../videoList/cardDragRules";
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

/** MoveDestination はグループのページから項目を移す先である。 */
export type MoveDestination =
  { kind: "group"; id: number; name: string } | { kind: "library" };

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

/** allVideos は項目がどれも動画かを返す。 */
function allVideos(items: readonly (GroupItem | undefined)[]): boolean {
  return items.every((item) => item?.kind === "video");
}

/**
 * GroupDragLayer はグループのページの所有者の画面（道筋の帯、格子、リスト）を包むドラッグの
 * 層である（specs/050-drag-organize/ui-design.md「Group page」）。
 *
 * - カードか行の間に落とすと、持ち上げた項目を画面の順のまま、その間の前の項目（先頭なら
 *   無し）の後ろへ並べ替える。
 * - 動画だけを子グループのカードの上に落とすと、その子グループへ移す（「Put in」）。
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
  onLift,
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
      if (item === undefined) return crumbName(crumbIndexOf(key), trail, name);
      return orderedItemTitle(item);
    },
    [byKey, name, trail],
  );
  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> => {
      const item = byKey.get(target);
      if (item?.kind !== "group" || item.group === undefined) return null;
      if (!allVideos(lifted.map((key) => byKey.get(key)))) return null;
      return { label: t.organize.drop.putIn };
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
      const moved = lifted.flatMap((key) => byKey.get(key) ?? []);
      if (sign.kind === "onto") {
        const target = byKey.get(sign.key)?.group;
        if (target !== undefined) {
          onMove(moved, { kind: "group", id: target.id, name: target.name });
        }
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
    [byKey, onMove, onReorder, order, trail],
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
    >
      {children}
    </CardDragLayer>
  );
}

import { type ReactNode, useCallback, useMemo } from "react";

import type { LibraryItem } from "../api/client";
import { groupKey, itemKey, videoKey } from "../api/libraryItems";
import { t, type UiText } from "../i18n";
import {
  type CardAttachment,
  type CardDrop,
  CardDragLayer,
  type HeldDrop,
  useCardDrag,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer } from "../videoList/cardDragRules";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { VideoRow, type VideoCardProps } from "../videoList/VideoCard";
import { type GroupCardProps, GroupRow } from "./GroupCard";
import { favoriteTargets, type LibrarySelection } from "./selection";

/**
 * selectedKeys は選択から持ち上げるカードの鍵を返す。「Favorite」と同じ分け方で、選んだ
 * グループはグループのカード、選んだグループのメンバーでない動画は動画のカードになる
 * （specs/050-drag-organize/ui-design.md「Lifting」）。
 */
export function selectedKeys(selection: LibrarySelection): string[] {
  const { videoIds, groupIds } = favoriteTargets(selection);
  return [...groupIds.map(groupKey), ...videoIds.map(videoKey)];
}

/**
 * LibraryDragLayer はライブラリの所有者の格子とリストを包むドラッグの層である
 * （specs/050-drag-organize/ui-design.md「Library」）。並べ替えは無く、カード全体が「上」になる。
 * 落とした先で何をするかは画面が決める（`onto`、`onDrop`）。
 */
export function LibraryDragLayer({
  items,
  list,
  selection,
  onto,
  onDrop,
  onLift,
  held,
  attached,
  children,
}: {
  /** 画面に出している項目（画面の順）。 */
  items: readonly LibraryItem[];
  list: boolean;
  selection: LibrarySelection;
  onto: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  onDrop: (drop: CardDrop) => void;
  onLift: () => void;
  held: HeldDrop | null;
  attached: CardAttachment | null;
  children: ReactNode;
}) {
  const { order, nameOf, preview } = useOrderedItemDrag(items, itemKey, list);
  const selected = useMemo(() => selectedKeys(selection), [selection]);
  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selected),
    [order, selected],
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
      onDrop={onDrop}
      onLift={onLift}
      instructions={t.organize.instructions}
      held={held}
      attached={attached}
    >
      {children}
    </CardDragLayer>
  );
}

/** DraggableVideoRow はライブラリのリスト表示の、持ち上げられる動画の行である。 */
export function DraggableVideoRow(props: VideoCardProps) {
  const drag = useCardDrag(videoKey(props.video.id));
  return <VideoRow {...props} drag={drag} />;
}

/** DraggableGroupRow はライブラリのリスト表示の、持ち上げられるグループの行である。 */
export function DraggableGroupRow(props: GroupCardProps) {
  const drag = useCardDrag(groupKey(props.group.id));
  return <GroupRow {...props} drag={drag} />;
}

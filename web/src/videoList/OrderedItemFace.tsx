import { useCallback, useMemo } from "react";

import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import { CardDragFace } from "./cardDrag";
import FolderArt from "./FolderArt";
import { type OrderedItem, orderedItemTitle, rowFacts } from "./ItemRowCells";

export function OrderedItemFace({ item, list }: { item: OrderedItem; list: boolean }) {
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

export function useOrderedItemDrag<T extends OrderedItem>(
  items: readonly T[],
  keyOf: (item: T) => string,
  list: boolean,
) {
  const order = useMemo(() => items.map(keyOf), [items, keyOf]);
  const byKey = useMemo(
    () => new Map(items.map((item) => [keyOf(item), item])),
    [items, keyOf],
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
      return item === undefined ? null : <OrderedItemFace item={item} list={list} />;
    },
    [byKey, list],
  );
  return { order, byKey, nameOf, preview };
}

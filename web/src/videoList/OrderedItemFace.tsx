import { useCallback, useMemo } from "react";

import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import { CardDragFace } from "./cardDrag";
import FolderArt from "./FolderArt";
import { type OrderedItem, orderedItemTitle, rowFacts } from "./ItemRowCells";

/**
 * OrderedItemFace は持ち上げた動画かグループの写しの中身である。格子では動画のサムネイルか
 * グループのフォルダの絵柄、リストでは行のサムネイル（specs/050-drag-organize/ui-design.md
 * 「Lifting」）。ライブラリの項目とグループの項目が同じ形を持つ。
 */
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

/**
 * useOrderedItemDrag はドラッグの層に渡す、項目の鍵の並び（`order`）、鍵から項目（`byKey`）、
 * 読み上げの名前（`nameOf`）、写しの中身（`preview`）である。ライブラリ、グループのページ、
 * フォルダ画面が共有する。
 */
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

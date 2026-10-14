import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { LibraryGroup, LibraryItem } from "../api/client";
import { itemKey } from "../api/libraryItems";
import { moveCards, type OrganizeRef } from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import { VideoThumbnail, VideoThumbnailImage } from "../ui/VideoThumbnail";
import {
  CardDragFace,
  type CardDrop,
  CardDragLayer,
  useCardDrag,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer } from "../videoList/cardDragRules";
import FolderArt from "../videoList/FolderArt";
import { useUndoToast } from "../videoList/undoToast";
import { VideoRow, type VideoCardProps } from "../videoList/VideoCard";
import { type GroupCardProps, GroupRow } from "./GroupCard";
import { favoriteTargets, type LibrarySelection } from "./selection";

// ライブラリのドラッグである（specs/050-drag-organize/ui-design.md「Library」）。所有者の格子と
// リストを包み、動画だけをグループのカードの上に落とすと、そのグループへ移す。ライブラリは
// 並べ替えないので、カード全体が「上」になる。

/** itemName は項目の名前である（読み上げと写しの題名）。 */
function itemName(item: LibraryItem): string {
  return item.kind === "video" ? item.video.title : item.group.name;
}

/** refOf は項目を移す操作で指す値である。 */
function refOf(item: LibraryItem): OrganizeRef {
  return item.kind === "video"
    ? { kind: "video", id: item.video.id }
    : { kind: "group", id: item.group.id };
}

/**
 * ItemFace は持ち上げた項目の写しの中身である。格子では動画のサムネイルかグループの
 * フォルダの絵柄、リストでは行のサムネイル（グループは最初のメンバーの 1 枚）。
 */
function ItemFace({ item, list }: { item: LibraryItem; list: boolean }) {
  if (item.kind === "group" && !list) {
    return (
      <CardDragFace
        title={item.group.name}
        thumbnail={
          <VideoThumbnail>
            <FolderArt previews={item.group.previews} />
          </VideoThumbnail>
        }
      />
    );
  }
  const src =
    item.kind === "group"
      ? item.group.previews[0]?.thumbnailUrl
      : item.video.thumbnailUrl;
  return (
    <CardDragFace
      title={itemName(item)}
      thumbnail={
        <VideoThumbnail>
          {src !== undefined && <VideoThumbnailImage src={src} />}
        </VideoThumbnail>
      }
    />
  );
}

/**
 * LibraryDragLayer はライブラリの所有者の格子とリストを包むドラッグの層である。選択した
 * カード（「Favorite」と同じ分け方。選んだグループのメンバーの動画は入らない）を持ち上げると、
 * 選択した読み込み済みのカードがすべて画面の順でいっしょに動く。
 */
export function LibraryDragLayer({
  items,
  list,
  selection,
  onMove,
  onLift,
  children,
}: {
  items: readonly LibraryItem[];
  list: boolean;
  selection: LibrarySelection;
  /** 項目 `moved`（画面の順）をグループ `to` へ移す。 */
  onMove: (moved: readonly LibraryItem[], to: LibraryGroup) => void;
  onLift: () => void;
  children: ReactNode;
}) {
  const order = useMemo(() => items.map(itemKey), [items]);
  const byKey = useMemo(
    () => new Map(items.map((item) => [itemKey(item), item])),
    [items],
  );
  const selectedKeys = useMemo(() => {
    const { videoIds, groupIds } = favoriteTargets(selection);
    const videos = new Set(videoIds);
    const groups = new Set(groupIds);
    return items
      .filter((item) =>
        item.kind === "video" ? videos.has(item.video.id) : groups.has(item.group.id),
      )
      .map(itemKey);
  }, [items, selection]);

  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selectedKeys),
    [order, selectedKeys],
  );
  const nameOf = useCallback(
    (key: string) => {
      const item = byKey.get(key);
      return item === undefined ? "" : itemName(item);
    },
    [byKey],
  );
  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> => {
      if (byKey.get(target)?.kind !== "group") return null;
      if (!lifted.every((key) => byKey.get(key)?.kind === "video")) return null;
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
      if (sign.kind !== "onto") return;
      const target = byKey.get(sign.key);
      if (target?.kind !== "group") return;
      onMove(
        lifted.flatMap((key) => byKey.get(key) ?? []),
        target.group,
      );
    },
    [byKey, onMove],
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
    >
      {children}
    </CardDragLayer>
  );
}

/** DraggableVideoRow はライブラリのリスト表示の、持ち上げられる動画の行である。 */
export function DraggableVideoRow(props: Omit<VideoCardProps, "drag">) {
  const drag = useCardDrag(itemKey({ kind: "video", video: props.video }));
  return <VideoRow {...props} drag={drag} />;
}

/** DraggableGroupRow はライブラリのリスト表示の、持ち上げられるグループの行である。 */
export function DraggableGroupRow(props: Omit<GroupCardProps, "drag">) {
  const drag = useCardDrag(itemKey({ kind: "group", group: props.group }));
  return <GroupRow {...props} drag={drag} />;
}

const noKeys: ReadonlySet<string> = new Set();

/** pruned は `hidden` のうち、もう一覧に無い項目の鍵を除く（見せるときに隠す必要が無い）。 */
function pruned(hidden: ReadonlySet<string>, items: readonly LibraryItem[]) {
  if (hidden.size === 0) return hidden;
  const present = new Set(items.map(itemKey));
  const kept = new Set(Array.from(hidden).filter((key) => present.has(key)));
  return kept.size === hidden.size ? hidden : kept;
}

/**
 * useLibraryMoves はライブラリでグループへ落とした項目を移す（research.md R-8）。落とした
 * 項目はすぐ一覧から隠し、選択を解き、`move` を送る。受け入れられたら「Undo」付きの通知を
 * 出して一覧をその場で読み直し、失敗したら隠した項目を戻して知らせる。隠した項目は、
 * 読み直した一覧から消えたら覚えるのをやめ、取り消しの後はすべて見せる。
 */
export function useLibraryMoves(
  items: readonly LibraryItem[],
  refreshInPlace: () => void,
  clearSelection: () => void,
) {
  const toast = useToast();
  const [hidden, setHidden] = useState<ReadonlySet<string>>(noKeys);
  const shown = useRef(true);
  useEffect(() => {
    shown.current = true;
    return () => {
      shown.current = false;
    };
  }, []);

  useEffect(() => {
    setHidden((current) => pruned(current, items));
  }, [items]);

  const afterUndo = useCallback(() => {
    setHidden(noKeys);
    refreshInPlace();
  }, [refreshInPlace]);
  const { offer } = useUndoToast(afterUndo);

  const move = useCallback(
    (moved: readonly LibraryItem[], to: LibraryGroup) => {
      if (moved.length === 0) return;
      const keys = moved.map(itemKey);
      setHidden((current) => new Set([...current, ...keys]));
      clearSelection();
      moveCards(moved.map(refOf), { from: null, to: to.id }).then(
        ({ undoToken }) => {
          if (!shown.current) return;
          offer(t.organize.done.moved(moved.length, to.name), undoToken);
          refreshInPlace();
        },
        (error: unknown) => {
          setHidden(
            (current) =>
              new Set(Array.from(current).filter((key) => !keys.includes(key))),
          );
          toast(t.organize.failed.move(errorText(error)));
        },
      );
    },
    [clearSelection, offer, refreshInPlace, toast],
  );

  const visible = useMemo(
    () =>
      hidden.size === 0 ? items : items.filter((item) => !hidden.has(itemKey(item))),
    [hidden, items],
  );
  return { visible, move };
}

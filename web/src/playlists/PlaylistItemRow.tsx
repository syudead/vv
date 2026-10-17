import type { Ref } from "react";
import { Link } from "react-router";

import type { PlaylistItem } from "../api/playlists";
import { formatNumber, t } from "../i18n";
import { cn } from "../lib/cn";
import { TableCell, TableRow } from "../ui/shadcn/table";
import {
  ItemThumbnailCell,
  ItemTitleExtras,
  ItemWatchedAndDurationCells,
  rowFacts,
} from "../videoList/ItemRowCells";
import { RemoveItemButton, ReorderHandle, useSortableRow } from "../videoList/reorder";
import { itemTitle, playTarget } from "./playlistItems";

/**
 * PlaylistItemRow はプレイリストの画面の 1 行である。
 * ライブラリのリスト表示の行と同じ列と高さに、前へハンドルと番号、後ろへ外す `×` を足す。
 *
 * 行のリンク（サムネイルと題名）はその項目から再生を始める。行はハンドルからだけ持ち上がり
 * 、持ち上げている間はリンクを押せない。
 */
export default function PlaylistItemRow({
  playlistId,
  item,
  position,
  removing,
  onRemove,
  removeRef,
}: {
  playlistId: number;
  item: PlaylistItem;
  /** 1 始まりの番号。 */
  position: number;
  /** 外す要求を送っている間 true。 */
  removing: boolean;
  onRemove: (item: PlaylistItem) => void;
  removeRef: Ref<HTMLButtonElement>;
}) {
  const { rowRef, style, isDragging, handle } = useSortableRow(item.id);
  const title = itemTitle(item);
  const facts = rowFacts(item);
  const target = playTarget(playlistId, item);

  return (
    <TableRow
      ref={rowRef}
      data-item-id={item.id}
      style={style}
      className={cn(
        "relative motion-reduce:transition-none",
        isDragging && "z-10 bg-popover shadow-elevated hover:bg-popover",
      )}
    >
      <TableCell className="w-0 pr-0 pl-1">
        <span className="flex items-center gap-1">
          <ReorderHandle handle={handle} label={t.playlists.item.reorder(title)} />
          <span className="min-w-4 text-right text-xs text-muted-foreground tabular-nums">
            {formatNumber(position)}
          </span>
        </span>
      </TableCell>
      <ItemThumbnailCell item={item} facts={facts} />
      <TableCell className="w-full max-w-0 pr-2 whitespace-normal sm:pr-4">
        {target === null ? (
          <span
            title={title}
            className={cn(
              "line-clamp-2 text-sm font-medium break-all",
              facts.empty && "text-muted-foreground",
            )}
          >
            {title}
          </span>
        ) : (
          // リンクの after: を行（relative）いっぱいに広げ、行のどこを押しても再生を始める。
          // ハンドルと × は z-10 でその上に置く。
          <Link
            to={target.to}
            state={target.state}
            title={title}
            aria-label={t.playlists.item.playFrom(title)}
            className={cn(
              "line-clamp-2 rounded-sm text-sm font-medium break-all after:absolute after:inset-0 hover:text-primary",
              facts.state === "watched" ? "text-muted-foreground" : "text-foreground",
              isDragging && "pointer-events-none",
            )}
          >
            {title}
          </Link>
        )}
        <ItemTitleExtras facts={facts} />
      </TableCell>
      <ItemWatchedAndDurationCells item={item} facts={facts} />
      <TableCell className="w-0 pr-1 pl-0 sm:pr-3">
        <RemoveItemButton
          removing={removing}
          label={t.playlists.item.removeFor(title)}
          tooltip={t.playlists.item.remove}
          onRemove={() => onRemove(item)}
          buttonRef={removeRef}
        />
      </TableCell>
    </TableRow>
  );
}

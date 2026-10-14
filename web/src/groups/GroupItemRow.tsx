import { memo, type Ref } from "react";
import { Link } from "react-router";

import type { GroupItem } from "../api/groups";
import { formatNumber, t } from "../i18n";
import { cn } from "../lib/cn";
import { Checkbox } from "../ui/shadcn/checkbox";
import { TableCell, TableRow } from "../ui/shadcn/table";
import {
  ItemThumbnailCell,
  ItemTitleExtras,
  ItemWatchedAndDurationCells,
  orderedItemTitle,
  rowFacts,
} from "../videoList/ItemRowCells";
import {
  RemoveItemButton,
  ReorderHandle,
  type SortableRow,
  useSortableRow,
} from "../videoList/reorder";

export interface GroupItemRowProps {
  item: GroupItem;
  /** 1 始まりの番号。 */
  position: number;
  /** 行が開く場所（動画は `?group` 付きの再生画面、子グループは道筋を伸ばしたページ）。 */
  to: string;
  /** 開いた先の「戻る」が帰るこのページの URL。 */
  backTo: string;
  /** 動画なら選択に入っているとき、子グループなら全メンバーが入っているとき true。 */
  selected: boolean;
  selectionMode: boolean;
  /** 選択を入れる・外す。選択を持たない見る人（ゲスト）では省き、チェックを描かない。 */
  onSelect?: (item: GroupItem, selected: boolean) => void;
  /** 持ち上げられる行の値。所有者のリスト表示だけが渡し、ハンドルを描く。 */
  sortable?: SortableRow;
  /** 行の `×`。所有者だけが渡す。 */
  remove?: GroupItemRemove;
}

/** GroupItemRemove は行の `×` に渡す値である。 */
export interface GroupItemRemove {
  /** 外す要求を送っている間 true。 */
  removing: boolean;
  onRemove: (item: GroupItem) => void;
  buttonRef: Ref<HTMLButtonElement>;
}

/** selectLabel はチェックの読み上げ名である。ライブラリの行と同じ文言。 */
function selectLabel(item: GroupItem, title: string) {
  return item.kind === "group"
    ? t.library.group.select(title)
    : t.list.card.select(title);
}

/** linkLabel は子グループの行のリンクの読み上げ名である。動画は題名をそのまま読む。 */
function linkLabel(item: GroupItem) {
  const group = item.group;
  if (item.kind !== "group" || group === undefined) return undefined;
  return t.library.group.label(group.name, group.videoCount, group.watchedCount ?? 0);
}

/**
 * GroupItemRow はグループのページのリスト表示の 1 行である（specs/048-groups/ui-design.md
 * 「Body」）。プレイリストの画面の行に、ライブラリの選択のチェックを前に足す: チェック、番号、
 * サムネイル、題名（子グループは「12 videos」の行）、`sm` から視聴済み、長さ。所有者には番号の
 * 前にハンドル、後ろに `×`「Remove from group」を描く（「Reordering」「Removing one item」）。
 */
export const GroupItemRow = memo(function GroupItemRow({
  item,
  position,
  to,
  backTo,
  selected,
  selectionMode,
  onSelect,
  sortable,
  remove,
}: GroupItemRowProps) {
  const title = orderedItemTitle(item);
  const facts = rowFacts(item);
  const dragging = sortable?.isDragging === true;

  return (
    <TableRow
      ref={sortable?.rowRef}
      data-item-id={item.id}
      data-state={selected ? "selected" : undefined}
      style={sortable?.style}
      className={cn(
        "group relative motion-reduce:transition-none",
        dragging && "z-10 bg-popover shadow-elevated hover:bg-popover",
      )}
    >
      {onSelect !== undefined && (
        <TableCell className="w-10 pl-3">
          <Checkbox
            checked={selected}
            onCheckedChange={(next) => onSelect(item, next === true)}
            aria-label={selectLabel(item, title)}
            className={cn(
              "relative z-10 transition-opacity",
              selectionMode || selected
                ? "opacity-100"
                : "opacity-50 group-focus-within:opacity-100 group-hover:opacity-100",
            )}
          />
        </TableCell>
      )}
      <TableCell className={cn("w-0 pr-0", onSelect === undefined ? "pl-3" : "pl-0")}>
        <span className="flex items-center gap-1">
          {sortable !== undefined && (
            <ReorderHandle
              handle={sortable.handle}
              label={t.groups.item.reorder(title)}
            />
          )}
          <span className="block min-w-4 text-right text-xs text-muted-foreground tabular-nums">
            {formatNumber(position)}
          </span>
        </span>
      </TableCell>
      <ItemThumbnailCell item={item} facts={facts} />
      <TableCell className="w-full max-w-0 pr-2 whitespace-normal sm:pr-4">
        {/* リンクの after: を行（relative）いっぱいに広げ、行のどこを押しても開く。チェックは
            z-10 でその上に置く。 */}
        <Link
          to={to}
          state={{ from: backTo }}
          title={title}
          aria-label={linkLabel(item)}
          onClick={(event) => {
            if (selectionMode && onSelect !== undefined) {
              event.preventDefault();
              onSelect(item, !selected);
            }
          }}
          className={cn(
            "line-clamp-2 rounded-sm text-sm font-medium break-all after:absolute after:inset-0 hover:text-primary",
            facts.state === "watched" ? "text-muted-foreground" : "text-foreground",
            dragging && "pointer-events-none",
          )}
        >
          {title}
        </Link>
        <ItemTitleExtras facts={facts} />
      </TableCell>
      <ItemWatchedAndDurationCells item={item} facts={facts} />
      {remove !== undefined && (
        <TableCell className="w-0 pr-1 pl-0 sm:pr-3">
          <RemoveItemButton
            removing={remove.removing}
            label={t.groups.item.removeFor(title)}
            tooltip={t.groups.item.remove}
            onRemove={() => remove.onRemove(item)}
            buttonRef={remove.buttonRef}
          />
        </TableCell>
      )}
    </TableRow>
  );
});

/**
 * SortableGroupItemRow は所有者のリスト表示の、持ち上げられる 1 行である。DndContext と
 * SortableContext の中で描く。
 */
export function SortableGroupItemRow(props: Omit<GroupItemRowProps, "sortable">) {
  const sortable = useSortableRow(props.item.id);
  return <GroupItemRow {...props} sortable={sortable} />;
}

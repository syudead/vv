import { memo, type ReactNode, type Ref } from "react";
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
import { type CardDragBinding, CardDropMark, useCardDrag } from "../videoList/cardDrag";
import { RemoveItemButton, ReorderHandle } from "../videoList/reorder";

export interface GroupItemRowProps {
  item: GroupItem;
  position: number;
  /** Videos retain group playback context; child groups extend the trail. */
  to: string;
  backTo: string;
  /** Child groups are selected only when every member is selected. */
  selected: boolean;
  selectionMode: boolean;
  /** Omit for guests to hide selection controls. */
  onSelect?: (item: GroupItem, selected: boolean) => void;
  /** Owner list rows supply drag metadata to show a handle. */
  drag?: CardDragBinding;
  remove?: GroupItemRemove;
  /** Only newly created child groups have an inline naming field. */
  nameField?: ReactNode;
}

export interface GroupItemRemove {
  removing: boolean;
  onRemove: (item: GroupItem) => void;
  buttonRef: Ref<HTMLButtonElement>;
}

function selectLabel(item: GroupItem, title: string) {
  return item.kind === "group"
    ? t.library.group.select(title)
    : t.list.card.select(title);
}

function linkLabel(item: GroupItem) {
  const group = item.group;
  if (item.kind !== "group" || group === undefined) return undefined;
  return t.library.group.label(group.name, group.videoCount, group.watchedCount ?? 0);
}

export const GroupItemRow = memo(function GroupItemRow({
  item,
  position,
  to,
  backTo,
  selected,
  selectionMode,
  onSelect,
  drag,
  remove,
  nameField,
}: GroupItemRowProps) {
  const title = orderedItemTitle(item);
  const facts = rowFacts(item);
  const dragging = drag?.sorting === true;

  return (
    <TableRow
      ref={drag?.ref}
      {...drag?.listeners}
      data-item-id={item.id}
      data-state={selected ? "selected" : undefined}
      style={drag?.style}
      className={cn(
        "group relative motion-reduce:transition-none",
        dragging && "z-10 bg-popover shadow-elevated hover:bg-popover",
        drag?.lifted === true && "opacity-50",
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
          {drag !== undefined && (
            <ReorderHandle handle={drag.handle} label={t.groups.item.reorder(title)} />
          )}
          <span className="block min-w-4 text-right text-xs text-muted-foreground tabular-nums">
            {formatNumber(position)}
          </span>
        </span>
      </TableCell>
      <ItemThumbnailCell item={item} facts={facts} />
      <TableCell className="w-full max-w-0 pr-2 whitespace-normal sm:pr-4">
        {/* Extend the link across the row; keep the checkbox above it with z-10. */}
        {nameField ?? (
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
        )}
        <ItemTitleExtras facts={facts} />
        {drag !== undefined && <CardDropMark sign={drag.sign} axis="y" list />}
        {drag?.attachment}
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

/** Render within GroupDragLayer. */
export function DraggableGroupItemRow(props: Omit<GroupItemRowProps, "drag">) {
  const drag = useCardDrag(String(props.item.id));
  return <GroupItemRow {...props} drag={drag} />;
}

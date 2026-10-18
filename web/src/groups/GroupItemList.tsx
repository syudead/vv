import type { GroupItem } from "../api/groups";
import { t } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { TableBody } from "../ui/shadcn/table";
import {
  DraggableGroupItemRow,
  GroupItemRow,
  type GroupItemRowProps,
} from "./GroupItemRow";

type ListRow = Omit<GroupItemRowProps, "drag" | "remove">;

export interface GroupItemEditing {
  removing: ReadonlySet<number>;
  onRemove: (item: GroupItem, items: readonly GroupItem[]) => void;
  removeRef: (itemId: number) => (node: HTMLButtonElement | null) => void;
}

/** Owner rows require an enclosing GroupDragLayer; guests have numbered rows only. */
export function GroupItemList({
  items,
  row,
  editing,
}: {
  items: readonly GroupItem[];
  row: (item: GroupItem, index: number) => ListRow;
  editing?: GroupItemEditing;
}) {
  return (
    <DataTable label={t.groups.detail.table}>
      <TableBody>
        {items.map((item, index) =>
          editing === undefined ? (
            <GroupItemRow key={item.id} {...row(item, index)} />
          ) : (
            <DraggableGroupItemRow
              key={item.id}
              {...row(item, index)}
              remove={{
                removing: editing.removing.has(item.id),
                onRemove: (target) => editing.onRemove(target, items),
                buttonRef: editing.removeRef(item.id),
              }}
            />
          ),
        )}
      </TableBody>
    </DataTable>
  );
}

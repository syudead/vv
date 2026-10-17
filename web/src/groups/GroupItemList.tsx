import type { GroupItem } from "../api/groups";
import { t } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { TableBody } from "../ui/shadcn/table";
import {
  DraggableGroupItemRow,
  GroupItemRow,
  type GroupItemRowProps,
} from "./GroupItemRow";

/** ListRow は行ごとに決まる値である（項目、番号、開く場所、選択）。 */
type ListRow = Omit<GroupItemRowProps, "drag" | "remove">;

/** GroupItemEditing は所有者のリスト表示の行の `×` である。 */
export interface GroupItemEditing {
  removing: ReadonlySet<number>;
  onRemove: (item: GroupItem, items: readonly GroupItem[]) => void;
  removeRef: (itemId: number) => (node: HTMLButtonElement | null) => void;
}

/**
 * GroupItemList はグループのページのリスト表示である。所有者には行をドラッグの層（GroupDragLayer、呼び出し側が包む）に加え、
 * ハンドルと行の `×` を足す。ゲストには番号つきの行だけを描く。
 */
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

import { closestCenter, DndContext, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";

import type { GroupItem } from "../api/groups";
import { t } from "../i18n";
import { DataTable } from "../ui/patterns/data-table";
import { TableBody } from "../ui/shadcn/table";
import { orderedItemTitle } from "../videoList/ItemRowCells";
import {
  reorderAnnouncements,
  useReorderSensors,
  verticalOnly,
} from "../videoList/reorder";
import {
  GroupItemRow,
  type GroupItemRowProps,
  SortableGroupItemRow,
} from "./GroupItemRow";

/** ListRow は行ごとに決まる値である（項目、番号、開く場所、選択）。 */
type ListRow = Omit<GroupItemRowProps, "sortable" | "remove">;

/** GroupItemEditing は所有者のリスト表示の編集（並べ替えと `×`）である。 */
export interface GroupItemEditing {
  removing: ReadonlySet<number>;
  onMove: (event: DragEndEvent, items: readonly GroupItem[]) => void;
  onRemove: (item: GroupItem, items: readonly GroupItem[]) => void;
  removeRef: (itemId: number) => (node: HTMLButtonElement | null) => void;
}

/**
 * GroupItemList はグループのページのリスト表示である（specs/048-groups/ui-design.md「Body」
 * 「Reordering」）。所有者には行をハンドルで持ち上げて並べ替える dnd-kit の文脈と行の `×` を
 * 足し、ゲストには番号つきの行だけを描く。持ち上げ方と読み上げはプレイリストの画面と同じ。
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
  const sensors = useReorderSensors();
  if (editing === undefined) {
    return (
      <DataTable label={t.groups.detail.table}>
        <TableBody>
          {items.map((item, index) => (
            <GroupItemRow key={item.id} {...row(item, index)} />
          ))}
        </TableBody>
      </DataTable>
    );
  }
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      modifiers={[verticalOnly]}
      onDragEnd={(event) => editing.onMove(event, items)}
      accessibility={{
        announcements: reorderAnnouncements(items, orderedItemTitle, t.groups.item),
        screenReaderInstructions: { draggable: t.groups.item.reorderInstructions },
      }}
    >
      <SortableContext
        items={items.map((item) => item.id)}
        strategy={verticalListSortingStrategy}
      >
        <DataTable label={t.groups.detail.table}>
          <TableBody>
            {items.map((item, index) => (
              <SortableGroupItemRow
                key={item.id}
                {...row(item, index)}
                remove={{
                  removing: editing.removing.has(item.id),
                  onRemove: (target) => editing.onRemove(target, items),
                  buttonRef: editing.removeRef(item.id),
                }}
              />
            ))}
          </TableBody>
        </DataTable>
      </SortableContext>
    </DndContext>
  );
}

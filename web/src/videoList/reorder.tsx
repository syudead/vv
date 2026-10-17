import {
  type Announcements,
  KeyboardSensor,
  type Modifier,
  PointerSensor,
  TouchSensor,
  type UniqueIdentifier,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { sortableKeyboardCoordinates, useSortable } from "@dnd-kit/sortable";
import { GripVertical, X } from "lucide-react";
import type { CSSProperties, Ref } from "react";

import type { UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

// 順に並ぶ項目（動画かグループ）を持ち上げて並べ替え、`×` で外す行の共通の部品である。
// プレイリストの画面と
// グループのページのリスト表示が同じ持ち上げ方・読み上げ・ハンドル・`×` を使う。

/** verticalOnly は持ち上げた行を縦にだけ動かす。 */
export const verticalOnly: Modifier = ({ transform }) => ({ ...transform, x: 0 });

/** useReorderSensors はポインタ・タッチ・キーボードの持ち上げ方である。 */
export function useReorderSensors() {
  return useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
}

/** ReorderTexts は dnd-kit の読み上げの文言である（en.ts の reorderTexts）。 */
export interface ReorderTexts {
  pickedUp: (title: string, position: number, count: number) => UiText;
  movedTo: (title: string, position: number, count: number) => UiText;
  dropped: (title: string, position: number, count: number) => UiText;
  cancelled: (title: string) => UiText;
}

/**
 * reorderAnnouncements は dnd-kit の読み上げである。位置は今の並び `items` での 1 始まりで、
 * 名前は `titleOf` が項目から決める。
 */
export function reorderAnnouncements<T extends { id: number }>(
  items: readonly T[],
  titleOf: (item: T) => string,
  texts: ReorderTexts,
): Announcements {
  const title = (dragId: UniqueIdentifier): string => {
    const item = items.find((each) => each.id === Number(dragId));
    return item === undefined ? "" : titleOf(item);
  };
  const position = (dragId: UniqueIdentifier | undefined): number =>
    items.findIndex((each) => each.id === Number(dragId)) + 1;
  const count = items.length;
  return {
    onDragStart: ({ active }) =>
      texts.pickedUp(title(active.id), position(active.id), count),
    onDragOver: ({ active, over }) =>
      over === null
        ? undefined
        : texts.movedTo(title(active.id), position(over.id), count),
    onDragEnd: ({ active, over }) =>
      texts.dropped(title(active.id), position(over?.id ?? active.id), count),
    onDragCancel: ({ active }) => texts.cancelled(title(active.id)),
  };
}

/** SortableRow は持ち上げられる行に渡す値である（useSortableRow の戻り値）。 */
export interface SortableRow {
  rowRef: (node: HTMLElement | null) => void;
  style: CSSProperties;
  isDragging: boolean;
  handle: {
    ref: (node: HTMLElement | null) => void;
    props: Record<string, unknown>;
  };
}

/**
 * useSortableRow は行 `id` を持ち上げられるようにする。位置は持ち上げている間だけ実行時に
 * 決まる値なので style で渡し、横には動かさない。行はハンドルからだけ持ち上がる。
 */
export function useSortableRow(id: number): SortableRow {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id });
  return {
    rowRef: setNodeRef,
    style:
      transform === null
        ? { transition }
        : { transform: `translate3d(0, ${String(transform.y)}px, 0)`, transition },
    isDragging,
    handle: { ref: setActivatorNodeRef, props: { ...attributes, ...listeners } },
  };
}

/** ReorderHandle は行を持ち上げるハンドルである。行のリンクより上（z-10）に置く。 */
export function ReorderHandle({
  handle,
  label,
}: {
  handle: SortableRow["handle"];
  label: UiText;
}) {
  return (
    <Button
      ref={handle.ref}
      type="button"
      variant="ghost"
      size="icon-sm"
      className="relative z-10 cursor-grab touch-none text-muted-foreground"
      {...handle.props}
      aria-label={label}
    >
      <GripVertical aria-hidden="true" />
    </Button>
  );
}

/**
 * RemoveItemButton は行の `×` である。確認なしで外し、送っている間は `disabled` にして
 * `Spinner` を出す。行のリンクより上（z-10）に置く。
 */
export function RemoveItemButton({
  removing,
  label,
  tooltip,
  onRemove,
  buttonRef,
}: {
  removing: boolean;
  label: UiText;
  tooltip: UiText;
  onRemove: () => void;
  buttonRef: Ref<HTMLButtonElement>;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          ref={buttonRef}
          type="button"
          variant="ghost"
          size="icon-sm"
          className="relative z-10 text-muted-foreground"
          disabled={removing}
          aria-busy={removing || undefined}
          aria-label={label}
          onClick={onRemove}
        >
          {removing ? <Spinner aria-hidden="true" /> : <X aria-hidden="true" />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

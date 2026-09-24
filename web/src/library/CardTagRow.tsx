import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

import type { TagRef } from "../api/client";
import { cn } from "../lib/cn";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { computeVisibleTagCount } from "./tagRowOverflow";

/** gap-1（0.25rem、16px 基準）と同じ値。 */
const GAP_PX = 4;

export interface CardTagRowProps {
  tags: readonly TagRef[];
  /**
   * 選択中（1件以上選んでいる）ときは、チップをボタンとして描かず Tab の順からも
   * 外し、押すとカードの選択を切り替える（ui-design.md「Card structure and
   * pressing」、Edge Case「選択中にタグを押したとき」）。
   */
  selectionMode: boolean;
  /** チップを押したとき、そのタグで絞り込む。 */
  onPress: (tag: TagRef) => void;
  /** 選択中にチップを押したとき、カードの選択を切り替える。 */
  onToggleSelection: () => void;
}

function chipClassName(pressable: boolean): string {
  return cn(
    "inline-flex h-5 max-w-full shrink-0 items-center rounded-sm bg-elevated px-1.5 text-xs text-fg-muted",
    pressable &&
      "hover:text-fg hover:ring-1 hover:ring-inset hover:ring-border-strong focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-link",
  );
}

function TagChip({
  tag,
  pressable,
  onPress,
}: {
  tag: TagRef;
  pressable: boolean;
  onPress: () => void;
}) {
  if (!pressable) {
    return (
      <span title={tag.name} className={chipClassName(false)}>
        <span className="min-w-0 truncate">{tag.name}</span>
      </span>
    );
  }
  return (
    <button
      type="button"
      title={tag.name}
      aria-label={`${tag.name}で絞り込む`}
      onClick={onPress}
      className={chipClassName(true)}
    >
      <span className="min-w-0 truncate">{tag.name}</span>
    </button>
  );
}

/**
 * CardTagRow はライブラリの格子カードの題名の下に出すタグの行である
 * （specs/014-video-tags/ui-design.md「Tag row」「Overflow」）。1行に収まらない
 * 分は末尾の「+N」にまとめ、押すと Popover で残りを縦に見せる。
 */
export default function CardTagRow({
  tags,
  selectionMode,
  onPress,
  onToggleSelection,
}: CardTagRowProps) {
  const rowRef = useRef<HTMLUListElement | null>(null);
  const measureRowRef = useRef<HTMLDivElement | null>(null);
  const overflowMeasureRef = useRef<HTMLSpanElement | null>(null);
  const [visibleCount, setVisibleCount] = useState(tags.length);
  const [open, setOpen] = useState(false);

  const recompute = useCallback(() => {
    const row = rowRef.current;
    const measureRow = measureRowRef.current;
    const overflowEl = overflowMeasureRef.current;
    if (row === null || measureRow === null || overflowEl === null) return;
    const available = row.clientWidth;
    const widths = Array.from(measureRow.children).map(
      (child) => (child as HTMLElement).getBoundingClientRect().width,
    );
    const overflowWidth = overflowEl.getBoundingClientRect().width;
    setVisibleCount(computeVisibleTagCount(widths, overflowWidth, GAP_PX, available));
  }, []);

  // 描画の前（layout effect）に決める。測る前の1フレームで行が伸び縮みしない
  // ようにするためである（ui-design.md「Overflow」）。
  // tags の中身が変わったときも測り直す（依存に含めるだけで、効果の中では
  // 直接参照しない。measure 用の子要素を再描画させる役目は React の再描画が担う）。
  useLayoutEffect(() => {
    recompute();
  }, [recompute, tags]);

  useEffect(() => {
    const row = rowRef.current;
    if (row === null || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => recompute());
    observer.observe(row);
    return () => observer.disconnect();
  }, [recompute]);

  const pressable = !selectionMode;
  const clampedVisible = Math.min(visibleCount, tags.length);
  const hidden = tags.slice(clampedVisible);
  const visible = tags.slice(0, clampedVisible);

  return (
    <div
      onClick={selectionMode ? onToggleSelection : undefined}
      className={selectionMode ? "cursor-pointer" : undefined}
    >
      <ul
        ref={rowRef}
        aria-label="タグ"
        className="flex flex-nowrap items-center gap-1 overflow-hidden"
      >
        {visible.map((tag) => (
          <li key={tag.id} className="min-w-0 max-w-full">
            <TagChip tag={tag} pressable={pressable} onPress={() => onPress(tag)} />
          </li>
        ))}
        {hidden.length > 0 &&
          (pressable ? (
            <li className="shrink-0">
              <PopoverRoot open={open} onOpenChange={setOpen}>
                <PopoverTrigger asChild>
                  <button
                    type="button"
                    aria-label={`ほかのタグ ${String(hidden.length)} 個を表示`}
                    className={chipClassName(true)}
                  >
                    <span className="tabular-nums">+{hidden.length}</span>
                  </button>
                </PopoverTrigger>
                <PopoverContent align="start" className="w-auto max-w-64 p-1.5">
                  <ul className="flex flex-col gap-1">
                    {hidden.map((tag) => (
                      <li key={tag.id}>
                        <TagChip
                          tag={tag}
                          pressable
                          onPress={() => {
                            setOpen(false);
                            onPress(tag);
                          }}
                        />
                      </li>
                    ))}
                  </ul>
                </PopoverContent>
              </PopoverRoot>
            </li>
          ) : (
            <li className="shrink-0">
              <span className={chipClassName(false)}>
                <span className="tabular-nums">+{hidden.length}</span>
              </span>
            </li>
          ))}
      </ul>

      {/* 測るためだけの、見えない全タグの並び（Overflow の算出。ui-design.md「Overflow」）。 */}
      <div
        aria-hidden="true"
        className="pointer-events-none invisible absolute flex flex-nowrap items-center gap-1"
      >
        <div ref={measureRowRef} className="flex flex-nowrap items-center gap-1">
          {tags.map((tag) => (
            <span key={tag.id} className={chipClassName(true)}>
              <span>{tag.name}</span>
            </span>
          ))}
        </div>
        <span ref={overflowMeasureRef} className={chipClassName(true)}>
          <span className="tabular-nums">+{String(tags.length)}</span>
        </span>
      </div>
    </div>
  );
}

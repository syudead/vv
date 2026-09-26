import { Folder } from "lucide-react";
import { useCallback, useLayoutEffect, useRef, useState } from "react";

import type { VideoTag } from "../api/client";
import { isFolderOnly } from "../api/tagOrder";
import { cn } from "../lib/cn";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { useTagRowMeasure } from "./TagRowMeasure";
import { computeVisibleTagCount } from "./tagRowOverflow";

/** gap-1（0.25rem、16px 基準）と同じ値。 */
const GAP_PX = 4;

export interface CardTagRowProps {
  tags: readonly VideoTag[];
  /**
   * 選択中（1件以上選んでいる）ときは、チップをボタンとして描かず Tab の順からも
   * 外し、押すとカードの選択を切り替える（ui-design.md「Card structure and
   * pressing」、Edge Case「選択中にタグを押したとき」）。
   */
  selectionMode: boolean;
  /** チップを押したとき、そのタグで絞り込む。 */
  onPress: (tag: VideoTag) => void;
  /** 選択中にチップを押したとき、カードの選択を切り替える。 */
  onToggleSelection: () => void;
}

/**
 * chipClassName の shrink は、行に収まらないときに幅を縮めて省略してよいかを
 * 決める。「+N」と、選ぶ余地の無い唯一の可視タグ（B4）以外は `shrink-0` にし、
 * 計測した幅のまま出す。
 *
 * surface は面の色。既定はカードの `bg-elevated`。ポップオーバーの中
 * （`PopoverContent` も `bg-elevated`）では `bg-field` にし、チップの面が
 * 窓の面へ溶けて見えなくならないようにする（N5、Synonym の窓の `bg-bg` と
 * 同じ理由）。
 *
 * folderOnly のチップは面を持たず、破線の枠と Folder の目印で区別する。
 * 大きさ（`h-5`・`text-xs`）と文字の色は面のあるチップと同じにし、hover では
 * `ring` を重ねず枠を実線にする（017 の ui-design.md「Folder-derived tag chip」
 * 「Interaction states」）。
 */
function chipClassName(
  pressable: boolean,
  shrink = false,
  surface: "elevated" | "field" = "elevated",
  folderOnly = false,
): string {
  return cn(
    "inline-flex h-5 max-w-full min-w-0 items-center rounded-sm px-1.5 text-xs text-fg-muted",
    folderOnly
      ? "gap-1 border border-dashed border-border-strong"
      : surface === "elevated"
        ? "bg-elevated"
        : "bg-field",
    shrink ? "shrink" : "shrink-0",
    pressable &&
      (folderOnly
        ? "hover:border-solid hover:text-fg"
        : "hover:text-fg hover:ring-1 hover:ring-inset hover:ring-border-strong"),
    pressable &&
      "focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-link",
  );
}

/** FolderMark は破線のチップの名前の前に置く目印である。 */
function FolderMark() {
  return <Folder className="size-3 shrink-0 text-fg-subtle" aria-hidden="true" />;
}

function TagChip({
  tag,
  pressable,
  shrink,
  surface,
  onPress,
}: {
  tag: VideoTag;
  pressable: boolean;
  shrink?: boolean;
  surface?: "elevated" | "field";
  onPress: () => void;
}) {
  const folderOnly = isFolderOnly(tag);
  const content = (
    <>
      {folderOnly && <FolderMark />}
      <span className="min-w-0 truncate">{tag.name}</span>
    </>
  );
  if (!pressable) {
    return (
      <span
        title={tag.name}
        className={chipClassName(false, shrink, surface, folderOnly)}
      >
        {content}
        {folderOnly && <span className="sr-only">（フォルダ名から）</span>}
      </span>
    );
  }
  return (
    <button
      type="button"
      title={tag.name}
      aria-label={
        folderOnly ? `${tag.name}で絞り込む（フォルダ名から）` : `${tag.name}で絞り込む`
      }
      onClick={onPress}
      className={chipClassName(true, shrink, surface, folderOnly)}
    >
      {content}
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

  // カードごとに見張りを作らず、一覧に1つの ResizeObserver へ登録する
  // （ui-design.md「Overflow」、B2）。Provider の外（単体テストなど）では
  // useTagRowMeasure が null を返し、初回の layout effect の計測だけになる。
  const observeRow = useTagRowMeasure();
  useLayoutEffect(() => {
    const row = rowRef.current;
    if (row === null || observeRow === null) return;
    return observeRow(row, recompute);
  }, [observeRow, recompute]);

  const pressable = !selectionMode;
  const clampedVisible = Math.min(visibleCount, tags.length);
  const hidden = tags.slice(clampedVisible);
  const visible = tags.slice(0, clampedVisible);
  // 先頭の1つすら自然な幅では収まらないのに1つは出しているとき（B4）は、
  // その1つだけ縮めて省略してよいことにする。
  const forcedShrink = visible.length === 1 && hidden.length > 0;

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
          <li
            key={tag.id}
            className={cn("min-w-0 max-w-full", !forcedShrink && "shrink-0")}
          >
            <TagChip
              tag={tag}
              pressable={pressable}
              shrink={forcedShrink}
              onPress={() => onPress(tag)}
            />
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
                          surface="field"
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
            <span
              key={tag.id}
              className={chipClassName(true, false, "elevated", isFolderOnly(tag))}
            >
              {isFolderOnly(tag) && <FolderMark />}
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

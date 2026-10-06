import { Folder } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

import type { VideoTag } from "../api/client";
import { isFolderOnly } from "../api/tagOrder";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { badgeVariants } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import TentativeMark from "../ui/TentativeMark";
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
 * chipClassName はタグのチップの見た目で、Badge の secondary（web/registry/rules/
 * components.md「Badge」）である。押せるチップは Badge の形の Button にする。
 *
 * shrink は、行に収まらないときに幅を縮めて省略してよいかを決める。「+N」と、選ぶ
 * 余地の無い唯一の可視タグ（B4）以外は `shrink-0` にし、計測した幅のまま出す。
 *
 * folderOnly のチップは面を持たず（Badge の outline）、破線の枠と Folder の目印で区別する。
 * hover では枠を実線にする（017 の ui-design.md「Folder-derived tag chip」
 * 「Interaction states」）。
 */
function chipClassName(pressable: boolean, shrink = false, folderOnly = false): string {
  return cn(
    badgeVariants({ variant: folderOnly ? "outline" : "secondary" }),
    // Button の高さと余白を Badge の段に揃える（押せるチップも同じ大きさにする）。
    "max-w-full min-w-0 py-0 font-normal text-muted-foreground has-[>svg]:px-2 [&_svg]:size-3",
    folderOnly && "border-dashed border-input",
    shrink ? "shrink" : "shrink-0",
    pressable &&
      (folderOnly
        ? "hover:border-solid hover:bg-transparent hover:text-foreground"
        : "hover:bg-accent hover:text-foreground"),
  );
}

/**
 * chipLabel は押せるチップの読み上げ名である。仮のタグは「(tentative)」を添える
 * （specs/031-tentative-tags/ui-design.md「Tentative mark」「Words」）。
 */
function chipLabel(tag: VideoTag, folderOnly: boolean): string {
  const row = t.library.tagRow;
  if (folderOnly) {
    return tag.tentative
      ? row.filterByFromFolderTentative(tag.name)
      : row.filterByFromFolder(tag.name);
  }
  return tag.tentative ? row.filterByTentative(tag.name) : row.filterBy(tag.name);
}

/**
 * listChipClassName は「+N」の一覧のチップに足す形である（specs/041-tag-overflow-list/
 * ui-design.md「List chip」、research.md R-3）。一覧では名前を省略せず、一覧の内幅
 * より広い名前はチップの中で左揃えに折り返し、その行の分だけ高くなる。1行に
 * 収まる名前は h-6 と同じ高さ（py-1 と text-xs の1行）で、行のチップと変わらない。
 * 目印は items-start で1行目に置き、mt-0.5 で1行目の文字の中央に揃える。
 */
const listChipClassName =
  "h-auto min-h-6 items-start justify-start py-1 text-left whitespace-normal [&_svg]:mt-0.5";

/** FolderMark は破線のチップの名前の前に置く目印である。 */
function FolderMark() {
  return <Folder className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />;
}

function TagChip({
  tag,
  pressable,
  shrink,
  inList = false,
  onPress,
}: {
  tag: VideoTag;
  pressable: boolean;
  shrink?: boolean;
  /** 「+N」の一覧の中のチップか。一覧では省略せず折り返す（R-3）。 */
  inList?: boolean;
  onPress: () => void;
}) {
  const folderOnly = isFolderOnly(tag);
  const content = (
    <>
      {folderOnly && <FolderMark />}
      <span className={cn("min-w-0", inList ? "break-all" : "truncate")}>{tag.name}</span>
      {tag.tentative && <TentativeMark />}
    </>
  );
  if (!pressable) {
    return (
      <span title={tag.name} className={chipClassName(false, shrink, folderOnly)}>
        {content}
        {folderOnly && <span className="sr-only"> {t.library.tagRow.fromFolder}</span>}
        {tag.tentative && <span className="sr-only"> {t.library.tagRow.tentative}</span>}
      </span>
    );
  }
  return (
    <Button
      variant="ghost"
      title={tag.name}
      aria-label={chipLabel(tag, folderOnly)}
      onClick={onPress}
      className={cn(chipClassName(true, shrink, folderOnly), inList && listChipClassName)}
    >
      {content}
    </Button>
  );
}

/**
 * CardTagRow はライブラリの格子カードの題名の下に出すタグの行である
 * （specs/014-video-tags/ui-design.md「Tag row」「Overflow」）。1行に収まらない
 * 分は末尾の「+N」にまとめ、押すと Popover で残りを折り返すチップの並びで見せる
 * （specs/041-tag-overflow-list/ui-design.md「The list」）。
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
  // 一覧は開いたときに写さず、行と同じ tags と測った数から毎回導く（041 の
  // research.md R-4）。隠れるタグが無くなったとき、または選択が始まったときは
  // 「+N」と一覧が一緒に消える。開いた状態も落とし、あとで隠れるタグが戻っても
  // 勝手に開かないようにする。
  const hasList = pressable && hidden.length > 0;
  useEffect(() => {
    if (!hasList) setOpen(false);
  }, [hasList]);
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
        aria-label={t.library.tagRow.label}
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
              <Popover open={open} onOpenChange={setOpen}>
                <PopoverTrigger asChild>
                  <Button
                    variant="ghost"
                    aria-label={t.library.tagRow.showMore(hidden.length)}
                    className={chipClassName(true)}
                  >
                    <span className="tabular-nums">
                      {t.library.tagRow.more(hidden.length)}
                    </span>
                  </Button>
                </PopoverTrigger>
                {/* 幅はチップに合わせて max-w-popover まで、高さは PopoverContent が
                    画面に残る高さで抑える。余白 p-2 はスクロールする並びの外に置く
                    （041 の ui-design.md「The list」）。 */}
                <PopoverContent align="start" className="w-auto max-w-popover p-2">
                  <ul className="flex min-h-0 flex-wrap gap-1 overflow-y-auto">
                    {hidden.map((tag) => (
                      <li key={tag.id} className="max-w-full min-w-0">
                        <TagChip
                          tag={tag}
                          pressable
                          inList
                          onPress={() => {
                            setOpen(false);
                            onPress(tag);
                          }}
                        />
                      </li>
                    ))}
                  </ul>
                </PopoverContent>
              </Popover>
            </li>
          ) : (
            <li className="shrink-0">
              <span className={chipClassName(false)}>
                <span className="tabular-nums">
                  {t.library.tagRow.more(hidden.length)}
                </span>
              </span>
            </li>
          ))}
      </ul>

      {/* 測るためだけの、見えない全タグの並び（Overflow の算出。ui-design.md「Overflow」）。 */}
      <div
        aria-hidden="true"
        // 幅 0 で切り、測る並びがカードの外（ページの右端の外）へはみ出して横スクロールを
        // 作らないようにする。中の並びは内容の幅のまま測れる。
        className="pointer-events-none invisible absolute flex w-0 flex-nowrap items-center gap-1 overflow-hidden"
      >
        <div ref={measureRowRef} className="flex flex-nowrap items-center gap-1">
          {tags.map((tag) => (
            <span key={tag.id} className={chipClassName(true, false, isFolderOnly(tag))}>
              {isFolderOnly(tag) && <FolderMark />}
              <span>{tag.name}</span>
              {tag.tentative && <TentativeMark />}
            </span>
          ))}
        </div>
        <span ref={overflowMeasureRef} className={chipClassName(true)}>
          <span className="tabular-nums">{t.library.tagRow.more(tags.length)}</span>
        </span>
      </div>
    </div>
  );
}

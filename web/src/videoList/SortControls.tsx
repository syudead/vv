import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarArrowDown,
  CalendarClock,
  CalendarHeart,
  ChevronDown,
  Dices,
  FileClock,
  HardDrive,
  History,
  type LucideIcon,
  Shuffle,
  Timer,
} from "lucide-react";

import { type ReactNode, useId } from "react";

import type { VideoSort } from "../api/client";
import { useAudience } from "../auth/audience";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { FieldLegend, FieldSet } from "../ui/shadcn/field";
import { Label } from "../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../ui/shadcn/radio-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import IconToggleGroup from "./IconToggleGroup";
import {
  directionLabel,
  directionToggleLabel,
  ownerOnlySortKinds,
  type SortDirection,
  type SortKind,
  sortDirection,
  sortKindLabel,
  sortKindOf,
  sortKinds,
  withDirection,
} from "./listCriteria";

/** sortIcons は種類ごとのアイコンである（ui-design.md「Sort and direction」）。 */
export const sortIcons: Record<SortKind, LucideIcon> = {
  added: CalendarArrowDown,
  modified: CalendarClock,
  created: FileClock,
  title: ArrowDownAZ,
  duration: Timer,
  size: HardDrive,
  played: History,
  favorited: CalendarHeart,
  random: Shuffle,
};

/**
 * sortOptions は並べ替えの9種を、選んだときの並び順とともに並べる。
 * value は種類を選んだときの値（向きは種類ごとの既定）である。
 */
export const sortOptions = sortKinds.map((info) => ({
  kind: info.kind,
  value: info.initial,
  icon: sortIcons[info.kind],
}));

/**
 * useSortOptions は今描いている相手に出す並べ替えの種類である。ゲストには
 * 「最近再生した順」と「お気に入りにした日時」を出さない（再生位置とお気に入りは
 * 所有者のもの。specs/016-single-account-auth/ui-design.md「Guest degradation」、
 * specs/035-favorites/ui-design.md「Guest degradation」）。ゲストは7種になる。
 */
function useSortOptions(): typeof sortOptions {
  const owner = useAudience() === "owner";
  return owner
    ? sortOptions
    : sortOptions.filter((option) => !ownerOnlySortKinds.includes(option.kind));
}

export interface SortControlProps {
  sort: VideoSort;
  /** 種類や向きを変えた。random を選んだときは呼び出し側が新しい seed を作る。 */
  onSortChange: (value: VideoSort) => void;
  /** ランダムの並びを作り直す（「並べ直す」）。 */
  onShuffle: () => void;
  disabled?: boolean;
  /** トップバーの中に置く（一覧ページ）。SortMenuView の inTopBar。 */
  inTopBar?: boolean;
}

/**
 * SortMenu はツールバーの並べ替えのメニューボタンと、そのすぐ右に接する
 * 向きの切り替え（ランダムのときは「並べ直す」）である。`md` 以上で出す。
 */
export function SortMenu({
  sort,
  onSortChange,
  onShuffle,
  disabled,
  inTopBar,
}: SortControlProps) {
  const options = useSortOptions();
  const active = sortKindOf(sort);
  const direction = sortDirection(sort);
  const toggleLabel =
    direction === undefined ? t.list.sort.shuffle : directionToggleLabel(sort);

  return (
    <SortMenuView
      label={sortKindLabel(active.kind)}
      currentLabel={t.list.sort.current(sortKindLabel(active.kind))}
      heading={t.list.sort.heading}
      value={active.initial}
      options={options.map((option) => ({
        value: option.value,
        label: sortKindLabel(option.kind),
        icon: option.icon,
      }))}
      onValueChange={(value) => {
        const next = sortKindOf(value);
        if (next.kind !== active.kind) onSortChange(next.initial);
      }}
      toggle={{
        label: toggleLabel,
        icon:
          direction === undefined ? (
            <Dices />
          ) : direction === "desc" ? (
            <ArrowDownWideNarrow />
          ) : (
            <ArrowUpNarrowWide />
          ),
        onClick: () => {
          if (direction === undefined) onShuffle();
          else onSortChange(withDirection(sort, direction === "asc" ? "desc" : "asc"));
        },
      }}
      disabled={disabled}
      inTopBar={inTopBar}
    />
  );
}

/**
 * CompactSortControls は `md` 未満の「表示と並び順」のまとめの中に置く並べ替えである。
 * 種類を2列のラジオで並べ、その下に向き（ランダムのときは「並べ直す」）を置く。
 */
export function CompactSortControls({
  sort,
  onSortChange,
  onShuffle,
  disabled,
  name,
}: SortControlProps & { /** ラジオの name（画面に1つ）。 */ name: string }) {
  const options = useSortOptions();
  const active = sortKindOf(sort);
  const direction = sortDirection(sort);

  return (
    <CompactSortView
      name={name}
      heading={t.list.sort.heading}
      value={active.initial}
      options={options.map((option) => ({
        value: option.value,
        label: sortKindLabel(option.kind),
        icon: option.icon,
      }))}
      onValueChange={(value) => onSortChange(value)}
      disabled={disabled}
    >
      {direction === undefined ? (
        <Button variant="outline" size="sm" onClick={onShuffle} disabled={disabled}>
          <Dices aria-hidden="true" />
          {t.list.sort.shuffle}
        </Button>
      ) : (
        <IconToggleGroup<SortDirection>
          label={t.list.sort.direction}
          value={direction}
          onValueChange={(next) => onSortChange(withDirection(sort, next))}
          options={[
            {
              value: "desc",
              label: directionLabel(sort, "desc"),
              icon: <ArrowDownWideNarrow aria-hidden="true" />,
            },
            {
              value: "asc",
              label: directionLabel(sort, "asc"),
              icon: <ArrowUpNarrowWide aria-hidden="true" />,
            },
          ]}
        />
      )}
    </CompactSortView>
  );
}

/** SortOption は並べ替えのメニューとまとめの 1 項目である。 */
export interface SortOption<T extends string> {
  /** 選んだときの値。 */
  value: T;
  label: UiText;
  icon: LucideIcon;
}

/**
 * SortMenuView は並べ替えのメニューボタンと、そのすぐ右に接する向きの切り替えの
 * 見た目である。ライブラリの `SortMenu` とタグ管理画面が同じ形を使う。`toggle` を
 * 渡さなければ（向きの無い並び順）メニューボタンだけを角丸で描く。
 */
export function SortMenuView<T extends string>({
  label,
  currentLabel,
  heading,
  value,
  options,
  onValueChange,
  toggle,
  disabled,
  inTopBar = false,
}: {
  /** ボタンに出す今の種類の名前。 */
  label: UiText;
  /** ボタンの読み上げ名（「Sort by: 〈種類〉」）。 */
  currentLabel: UiText;
  /** メニューの見出し。 */
  heading: UiText;
  /** メニューで選んでいる項目の値。 */
  value: T;
  options: readonly SortOption<T>[];
  onValueChange: (value: T) => void;
  toggle?: { label: UiText; icon: ReactNode; onClick: () => void };
  disabled?: boolean;
  /**
   * トップバーの中に置く（一覧ページ）。ボタンはトップバーの操作と同じ h-9 の
   * secondary に input の縁を付ける。
   */
  inTopBar?: boolean;
}) {
  return (
    <div className="flex">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant={inTopBar ? "secondary" : "outline"}
            size={inTopBar ? "default" : "sm"}
            aria-label={currentLabel}
            disabled={disabled}
            className={cn(
              inTopBar && "border border-input pr-3",
              toggle !== undefined && "rounded-r-none",
            )}
          >
            {label}
            <ChevronDown aria-hidden="true" className="text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>{heading}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={value}
            onValueChange={(next) => onValueChange(next as T)}
          >
            {options.map((option) => (
              <DropdownMenuRadioItem key={option.value} value={option.value}>
                <option.icon aria-hidden="true" />
                {option.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {toggle !== undefined && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={inTopBar ? "secondary" : "outline"}
              size={inTopBar ? "icon" : "icon-sm"}
              aria-label={toggle.label}
              disabled={disabled}
              // メニューのボタンに接して 1 つの組に見せる（間の縁は 1 本にする）。
              className={cn(
                inTopBar && "border border-input",
                "rounded-l-none border-l-0",
              )}
              onClick={toggle.onClick}
            >
              {toggle.icon}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{toggle.label}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

/**
 * CompactSortView は狭い幅のまとめの中の並べ替えの見た目である。種類を 2 列の
 * ラジオで並べ、その下に `children`（向きの切り替えなど）を置く。
 */
export function CompactSortView<T extends string>({
  name,
  heading,
  value,
  options,
  onValueChange,
  disabled,
  children,
}: {
  /** ラジオの name（画面に1つ）。 */
  name: string;
  heading: UiText;
  value: T;
  options: readonly SortOption<T>[];
  onValueChange: (value: T) => void;
  disabled?: boolean;
  children?: ReactNode;
}) {
  const idPrefix = useId();
  return (
    <FieldSet disabled={disabled} className="gap-3">
      <FieldLegend variant="label">{heading}</FieldLegend>
      <RadioGroup
        name={name}
        value={value}
        onValueChange={(next) => onValueChange(next as T)}
        disabled={disabled}
        className="grid-cols-2 gap-2"
      >
        {options.map((option) => {
          const id = `${idPrefix}-${option.value}`;
          return (
            <div key={option.value} className="flex min-w-0 items-center gap-2">
              <RadioGroupItem id={id} value={option.value} />
              <Label htmlFor={id} className="min-w-0 font-normal">
                <option.icon aria-hidden="true" className="size-4 shrink-0" />
                <span className="truncate">{option.label}</span>
              </Label>
            </div>
          );
        })}
      </RadioGroup>
      {children}
    </FieldSet>
  );
}

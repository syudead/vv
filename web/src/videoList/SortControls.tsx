import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarArrowDown,
  CalendarClock,
  ChevronDown,
  Dices,
  HardDrive,
  History,
  type LucideIcon,
  Shuffle,
  Timer,
} from "lucide-react";

import type { VideoSort } from "../api/client";
import { useAudience } from "../auth/audience";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import {
  MenuContent,
  MenuLabel,
  MenuRadioGroup,
  MenuRadioItem,
  MenuRoot,
  MenuTrigger,
} from "../ui/Menu";
import SegmentedControl from "../ui/SegmentedControl";
import Tooltip from "../ui/Tooltip";
import {
  directionLabel,
  directionToggleLabel,
  type SortDirection,
  type SortKind,
  sortDirection,
  sortKindOf,
  sortKinds,
  withDirection,
} from "./listCriteria";

/** sortIcons は種類ごとのアイコンである（ui-design.md「Sort and direction」）。 */
export const sortIcons: Record<SortKind, LucideIcon> = {
  added: CalendarArrowDown,
  modified: CalendarClock,
  title: ArrowDownAZ,
  duration: Timer,
  size: HardDrive,
  played: History,
  random: Shuffle,
};

/**
 * sortOptions は並べ替えの7種を、選んだときの並び順とともに並べる。
 * value は種類を選んだときの値（向きは種類ごとの既定）である。
 */
export const sortOptions = sortKinds.map((info) => ({
  kind: info.kind,
  value: info.initial,
  label: info.label,
  icon: sortIcons[info.kind],
}));

/**
 * useSortOptions は今描いている相手に出す並べ替えの種類である。ゲストには
 * 「最近再生した順」を出さない（再生位置は所有者のもの。
 * specs/016-single-account-auth/ui-design.md「Guest degradation」）。
 */
function useSortOptions(): typeof sortOptions {
  const owner = useAudience() === "owner";
  return owner ? sortOptions : sortOptions.filter((option) => option.kind !== "played");
}

export interface SortControlProps {
  sort: VideoSort;
  /** 種類や向きを変えた。random を選んだときは呼び出し側が新しい seed を作る。 */
  onSortChange: (value: VideoSort) => void;
  /** ランダムの並びを作り直す（「並べ直す」）。 */
  onShuffle: () => void;
  disabled?: boolean;
}

/**
 * SortMenu はツールバーの並べ替えのメニューボタンと、そのすぐ右に接する
 * 向きの切り替え（ランダムのときは「並べ直す」）である。`md` 以上で出す。
 */
export function SortMenu({ sort, onSortChange, onShuffle, disabled }: SortControlProps) {
  const options = useSortOptions();
  const active = sortKindOf(sort);
  const direction = sortDirection(sort);
  const toggleLabel = direction === undefined ? "並べ直す" : directionToggleLabel(sort);

  return (
    <div className="flex">
      <MenuRoot>
        <MenuTrigger asChild>
          <Button
            variant="secondary"
            aria-label={`並び順: ${active.label}`}
            disabled={disabled}
            className="rounded-r-none"
          >
            {active.label}
            <ChevronDown className="-mr-1 text-fg-muted" />
          </Button>
        </MenuTrigger>
        <MenuContent align="start">
          <MenuLabel>並び順</MenuLabel>
          <MenuRadioGroup
            value={active.initial}
            onValueChange={(value) => {
              const next = sortKindOf(value);
              if (next.kind !== active.kind) onSortChange(next.initial);
            }}
          >
            {options.map((option) => (
              <MenuRadioItem key={option.kind} value={option.value}>
                <option.icon />
                {option.label}
              </MenuRadioItem>
            ))}
          </MenuRadioGroup>
        </MenuContent>
      </MenuRoot>
      <Tooltip content={toggleLabel}>
        <Button
          variant="secondary"
          aria-label={toggleLabel}
          disabled={disabled}
          className="rounded-l-none px-2.5"
          onClick={() => {
            if (direction === undefined) onShuffle();
            else onSortChange(withDirection(sort, direction === "asc" ? "desc" : "asc"));
          }}
        >
          {direction === undefined ? (
            <Dices />
          ) : direction === "desc" ? (
            <ArrowDownWideNarrow />
          ) : (
            <ArrowUpNarrowWide />
          )}
        </Button>
      </Tooltip>
    </div>
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
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
        並び順
      </legend>
      <div className="grid grid-cols-2 gap-1">
        {options.map((option) => (
          <label
            key={option.kind}
            className={cn(
              "flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md px-1 text-sm transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-link",
              active.kind === option.kind
                ? "bg-accent text-accent-fg"
                : "text-fg hover:bg-hover-wash",
              disabled && "pointer-events-none opacity-50",
            )}
          >
            <input
              type="radio"
              name={name}
              value={option.value}
              checked={active.kind === option.kind}
              onChange={() => onSortChange(option.value)}
              className="sr-only"
            />
            <option.icon className="size-4 shrink-0" />
            <span className="truncate">{option.label}</span>
          </label>
        ))}
      </div>
      {direction === undefined ? (
        <Button variant="ghost" size="sm" onClick={onShuffle} disabled={disabled}>
          <Dices />
          並べ直す
        </Button>
      ) : (
        <SegmentedControl<SortDirection>
          label="並び順の向き"
          value={direction}
          onValueChange={(next) => onSortChange(withDirection(sort, next))}
          options={[
            {
              value: "desc",
              label: directionLabel(sort, "desc"),
              icon: <ArrowDownWideNarrow />,
            },
            {
              value: "asc",
              label: directionLabel(sort, "asc"),
              icon: <ArrowUpNarrowWide />,
            },
          ]}
        />
      )}
    </fieldset>
  );
}

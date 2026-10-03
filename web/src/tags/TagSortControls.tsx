import {
  ArrowDownAZ,
  ArrowDownWideNarrow,
  ArrowUpNarrowWide,
  CalendarPlus,
  ChevronDown,
  Hash,
  type LucideIcon,
  SlidersHorizontal,
} from "lucide-react";

import { t } from "../i18n";
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
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import SegmentedControl from "../ui/SegmentedControl";
import Tooltip from "../ui/Tooltip";
import {
  tagSortDirection,
  tagSortKind,
  tagSortKinds,
  withTagSortDirection,
  type TagListSort,
  type TagSortDirection,
  type TagSortKind,
} from "./tagListOrder";

/**
 * tagSortIcons は種類ごとのアイコンである（specs/036-tag-admin-scale/ui-design.md
 * 「Controls」）。「Date created」はライブラリのファイルの作成日（`FileClock`）と
 * 別のものなので、同じ絵にしない。
 */
const tagSortIcons: Record<TagSortKind, LucideIcon> = {
  name: ArrowDownAZ,
  count: Hash,
  created: CalendarPlus,
};

export interface TagSortControlProps {
  sort: TagListSort;
  onSortChange: (sort: TagListSort) => void;
  disabled?: boolean;
}

/**
 * TagSortMenu は操作の行の並び順のメニューボタンと、そのすぐ右に接する向きの
 * 切り替えである（ライブラリの `SortMenu` と同じ形）。「Name」に向きは無いので、
 * そのときは向きのボタンを出さない。`sm` 以上で出す。
 */
export function TagSortMenu({
  sort,
  onSortChange,
  disabled,
  className,
}: TagSortControlProps & { className?: string }) {
  const kind = tagSortKind(sort);
  const direction = tagSortDirection(sort);
  const label = t.tags.sort.kinds[kind];

  return (
    <div className={cn("flex", className)}>
      <MenuRoot>
        <MenuTrigger asChild>
          <Button
            variant="secondary"
            aria-label={t.tags.sort.current(label)}
            disabled={disabled}
            className={cn(direction !== undefined && "rounded-r-none")}
          >
            {label}
            <ChevronDown className="-mr-1 text-fg-muted" />
          </Button>
        </MenuTrigger>
        <MenuContent align="start">
          <MenuLabel>{t.tags.sort.heading}</MenuLabel>
          <MenuRadioGroup
            value={kind}
            onValueChange={(value) => {
              const next = tagSortKinds.find((option) => option.kind === value);
              if (next !== undefined && next.kind !== kind) onSortChange(next.initial);
            }}
          >
            {tagSortKinds.map((option) => {
              const Icon = tagSortIcons[option.kind];
              return (
                <MenuRadioItem key={option.kind} value={option.kind}>
                  <Icon />
                  {t.tags.sort.kinds[option.kind]}
                </MenuRadioItem>
              );
            })}
          </MenuRadioGroup>
        </MenuContent>
      </MenuRoot>
      {direction !== undefined && sort !== "name" && (
        <Tooltip content={t.tags.sort.toggle[sort]}>
          <Button
            variant="secondary"
            aria-label={t.tags.sort.toggle[sort]}
            disabled={disabled}
            className="rounded-l-none px-2.5"
            onClick={() =>
              onSortChange(
                withTagSortDirection(sort, direction === "asc" ? "desc" : "asc"),
              )
            }
          >
            {direction === "desc" ? <ArrowDownWideNarrow /> : <ArrowUpNarrowWide />}
          </Button>
        </Tooltip>
      )}
    </div>
  );
}

/**
 * TagCompactSort は `sm` 未満の並び順のまとめである。押すと吹き出しに、
 * ライブラリの `CompactSortControls` と同じ形（2 列のラジオと向きの
 * `SegmentedControl`）を出す。並び順が「Name」以外のときはボタンを
 * 効いている形にし、閉じていても既定の並びではないと分かるようにする。
 */
export function TagCompactSort({
  sort,
  onSortChange,
  disabled,
  className,
}: TagSortControlProps & { className?: string }) {
  const kind = tagSortKind(sort);
  const direction = tagSortDirection(sort);

  return (
    <PopoverRoot>
      <PopoverTrigger asChild>
        <Button
          variant="secondary"
          aria-label={t.tags.sort.compact}
          disabled={disabled}
          className={cn(
            "px-2.5",
            sort !== "name" && "bg-accent-soft text-link",
            className,
          )}
        >
          <SlidersHorizontal />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <fieldset className="space-y-2" disabled={disabled}>
          <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
            {t.tags.sort.heading}
          </legend>
          <div className="grid grid-cols-2 gap-1">
            {tagSortKinds.map((option) => {
              const Icon = tagSortIcons[option.kind];
              return (
                <label
                  key={option.kind}
                  className={cn(
                    "flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md px-1 text-sm transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-link",
                    kind === option.kind
                      ? "bg-accent text-accent-fg"
                      : "text-fg hover:bg-hover-wash",
                  )}
                >
                  <input
                    type="radio"
                    name="tag-compact-sort"
                    value={option.kind}
                    checked={kind === option.kind}
                    onChange={() => onSortChange(option.initial)}
                    className="sr-only"
                  />
                  <Icon className="size-4 shrink-0" />
                  <span className="truncate">{t.tags.sort.kinds[option.kind]}</span>
                </label>
              );
            })}
          </div>
          {direction !== undefined && (
            <SegmentedControl<TagSortDirection>
              label={t.tags.sort.direction}
              value={direction}
              onValueChange={(next) => onSortChange(withTagSortDirection(sort, next))}
              options={[
                {
                  value: "desc",
                  label:
                    kind === "count"
                      ? t.tags.sort.segments.countDesc
                      : t.tags.sort.segments.createdDesc,
                  icon: <ArrowDownWideNarrow />,
                },
                {
                  value: "asc",
                  label:
                    kind === "count"
                      ? t.tags.sort.segments.countAsc
                      : t.tags.sort.segments.createdAsc,
                  icon: <ArrowUpNarrowWide />,
                },
              ]}
            />
          )}
        </fieldset>
      </PopoverContent>
    </PopoverRoot>
  );
}

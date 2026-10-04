import { ListFilter } from "lucide-react";
import { type ReactNode, type Ref, useId, useState } from "react";

import type { WatchFilter } from "../api/client";
import { useAudience } from "../auth/audience";
import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { Label } from "../ui/label";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { RadioGroup, RadioGroupItem } from "../ui/radio-group";
import { watchLabel, watchValues } from "./listSummary";

export interface FilterMenuProps {
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  /** お気に入りのみ（所有者だけ。specs/035-favorites/ui-design.md「Filter menu」）。 */
  favorite: boolean;
  onFavoriteChange: (value: boolean) => void;
  /**
   * 検索語・視聴状態・再生可否・お気に入りのみのどれかが効いているか
   * （「条件を解除」を出す）。
   */
  canClear: boolean;
  /** 検索語・視聴状態・再生可否・お気に入りのみを外す。並べ替えは残す。 */
  onClear: () => void;
  disabled?: boolean;
}

/**
 * FilterMenu は絞り込みのボタンとポップオーバーである（ui-design.md「Filter menu」）。
 * ボタンの数字は視聴状態・お気に入りのみ・再生可否の数だけを数える。検索語は検索欄に
 * 見えている。
 *
 * ゲストには「視聴状態」と「お気に入りのみ」を出さない（再生位置とお気に入りは
 * 所有者のもの。specs/016-single-account-auth/ui-design.md「Guest degradation」、
 * specs/035-favorites/ui-design.md「Guest degradation」）。そのときボタンの数字は
 * 再生可否だけを数える。
 */
export default function FilterMenu({
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  favorite,
  onFavoriteChange,
  canClear,
  onClear,
  disabled,
}: FilterMenuProps) {
  const owner = useAudience() === "owner";
  const watchCount = owner && watch !== "all" ? 1 : 0;
  const favoriteCount = owner && favorite ? 1 : 0;
  const filterCount = watchCount + favoriteCount + (playable ? 1 : 0);
  const idBase = useId();

  return (
    <FilterPopover
      count={filterCount}
      disabled={disabled}
      canClear={canClear}
      onClear={onClear}
    >
      {owner && (
        <fieldset className="mb-4">
          <legend className="mb-2 text-xs font-semibold text-muted-foreground uppercase">
            {t.list.filter.watch}
          </legend>
          <RadioGroup
            name="watch"
            value={watch}
            onValueChange={(next) => onWatchChange(next as WatchFilter)}
            className="grid-cols-2 gap-x-3 gap-y-2"
          >
            {watchValues.map((value) => (
              <Label
                key={value}
                htmlFor={`${idBase}-${value}`}
                className="h-8 cursor-pointer font-normal"
              >
                <RadioGroupItem id={`${idBase}-${value}`} value={value} />
                {watchLabel(value)}
              </Label>
            ))}
          </RadioGroup>
        </fieldset>
      )}

      {/* 2 つのチェックは同じ性質の行なので、視聴状態の下の mb-4 より詰めた gap-2 で並べる。 */}
      <div className="flex flex-col gap-2">
        {owner && (
          <FilterCheckbox
            checked={favorite}
            onCheckedChange={onFavoriteChange}
            label={t.list.filter.favoritesOnly}
          />
        )}
        <FilterCheckbox
          checked={playable}
          onCheckedChange={onPlayableChange}
          label={t.list.filter.playableOnly}
        />
      </div>
    </FilterPopover>
  );
}

export interface FilterPopoverProps {
  /** 効いている絞り込みの数。1 以上ならボタンを効いている形にし、数を添える。 */
  count: number;
  disabled?: boolean;
  /** 「条件を解除」を出すか。 */
  canClear: boolean;
  /** 「条件を解除」を押した。押すと吹き出しを閉じる。 */
  onClear: () => void;
  /** ボタン（絞り込みを外したあとのフォーカスの行き先に使う）。 */
  triggerRef?: Ref<HTMLButtonElement>;
  children: ReactNode;
}

/**
 * FilterPopover はトップバーの「Filter」のボタンと吹き出しの枠である。ライブラリの
 * `FilterMenu` とタグ管理画面の絞り込みが同じ見た目を使う（ボタンの形・効いている
 * 間の色と数・吹き出しの位置・末尾の「Clear filters」）。中身の項目は呼び手が渡す。
 */
export function FilterPopover({
  count,
  disabled,
  canClear,
  onClear,
  triggerRef,
  children,
}: FilterPopoverProps) {
  const [open, setOpen] = useState(false);
  const shown = disabled ? 0 : count;

  return (
    <PopoverRoot open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          ref={triggerRef}
          variant="outline"
          size="sm"
          disabled={disabled}
          aria-label={shown > 0 ? t.list.filter.applied(shown) : t.list.filter.label}
          className={cn(
            shown > 0 && "border-primary-active bg-primary-soft text-primary",
          )}
        >
          <ListFilter />
          <span className="hidden xl:inline">{t.list.filter.label}</span>
          {shown > 0 && <span className="tabular-nums">{formatNumber(shown)}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start">
        {children}

        {canClear && (
          <Button
            variant="ghost"
            size="sm"
            className="mt-4 w-full"
            onClick={() => {
              onClear();
              // 閉じるとフォーカスは絞り込みのボタンへ戻る（Radix の既定）。
              setOpen(false);
            }}
          >
            {t.list.filter.clear}
          </Button>
        )}
      </PopoverContent>
    </PopoverRoot>
  );
}

/** FilterCheckbox は絞り込みの吹き出しの中のチェックの 1 行である。 */
export function FilterCheckbox({
  checked,
  onCheckedChange,
  label,
  hint,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: UiText;
  /** 行の下に添える説明。 */
  hint?: UiText;
}) {
  const id = useId();
  return (
    <div className="flex items-start gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(next) => onCheckedChange(next === true)}
        className="mt-0.5"
      />
      <Label
        htmlFor={id}
        className="cursor-pointer flex-col items-start gap-0.5 font-normal"
      >
        {label}
        {hint !== undefined && (
          <span className="text-xs text-muted-foreground">{hint}</span>
        )}
      </Label>
    </div>
  );
}

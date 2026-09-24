import { ListFilter } from "lucide-react";
import { useState } from "react";

import type { WatchFilter } from "../api/client";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { watchOptions } from "./listSummary";

export interface FilterMenuProps {
  watch: WatchFilter;
  onWatchChange: (value: WatchFilter) => void;
  playable: boolean;
  onPlayableChange: (value: boolean) => void;
  /** 検索語・視聴状態・再生可否のどれかが効いているか（「条件を解除」を出す）。 */
  canClear: boolean;
  /** 検索語・視聴状態・再生可否を外す。並べ替えは残す。 */
  onClear: () => void;
  disabled?: boolean;
}

/**
 * FilterMenu は絞り込みのボタンとポップオーバーである（ui-design.md「Filter menu」）。
 * ボタンの数字は視聴状態と再生可否の数だけを数える。検索語は検索欄に見えている。
 */
export default function FilterMenu({
  watch,
  onWatchChange,
  playable,
  onPlayableChange,
  canClear,
  onClear,
  disabled,
}: FilterMenuProps) {
  const [open, setOpen] = useState(false);
  const filterCount = disabled ? 0 : (watch === "all" ? 0 : 1) + (playable ? 1 : 0);

  return (
    <PopoverRoot open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="secondary"
          disabled={disabled}
          aria-label={
            filterCount > 0 ? `絞り込み（${String(filterCount)} 件適用中）` : "絞り込み"
          }
          className={cn("px-2.5", filterCount > 0 && "bg-accent-soft text-link")}
        >
          <ListFilter />
          {filterCount > 0 && <span className="tabular-nums">{filterCount}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start">
        <fieldset>
          <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
            視聴状態
          </legend>
          <div className="grid grid-cols-2 gap-1">
            {watchOptions.map((option) => (
              <label
                key={option.value}
                className={cn(
                  "flex h-8 cursor-pointer items-center justify-center rounded-md text-sm transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-link",
                  watch === option.value
                    ? "bg-accent text-accent-fg"
                    : "text-fg hover:bg-hover-wash",
                )}
              >
                <input
                  type="radio"
                  name="watch"
                  value={option.value}
                  checked={watch === option.value}
                  onChange={() => onWatchChange(option.value)}
                  className="sr-only"
                />
                {option.label}
              </label>
            ))}
          </div>
        </fieldset>

        <label className="mt-4 flex cursor-pointer items-center gap-2.5 text-sm text-fg">
          <input
            type="checkbox"
            checked={playable}
            onChange={(event) => onPlayableChange(event.target.checked)}
            className="size-4 accent-accent"
          />
          再生できるものだけ
        </label>

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
            条件を解除
          </Button>
        )}
      </PopoverContent>
    </PopoverRoot>
  );
}

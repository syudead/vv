import { Checkbox as RadixCheckbox } from "radix-ui";
import { Check, Minus } from "lucide-react";
import type { MouseEvent, Ref } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";

/**
 * Checkbox は選択のチェックである。`checked` に `"indeterminate"` を渡すと中間の
 * 状態（lucide `Minus`、`aria-checked="mixed"`）を描く。中間・空のときに押すと
 * `onCheckedChange(true)`、選択のときは `false` を渡す。
 */
export default function Checkbox({
  checked,
  onCheckedChange,
  label,
  className,
  onClick,
  disabled,
  describedBy,
  ref,
}: {
  checked: boolean | "indeterminate";
  onCheckedChange: (checked: boolean) => void;
  label: UiText;
  className?: string;
  onClick?: (event: MouseEvent) => void;
  disabled?: boolean;
  /** 押せない理由などを添える要素の id（`aria-describedby`）。 */
  describedBy?: string;
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <RadixCheckbox.Root
      ref={ref}
      checked={checked}
      onCheckedChange={(value) => onCheckedChange(value === true)}
      aria-label={label}
      aria-describedby={describedBy}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        "inline-flex size-5 shrink-0 items-center justify-center rounded-sm border transition-colors",
        "border-control-border bg-navbar enabled:hover:border-accent disabled:cursor-not-allowed",
        "data-[state=checked]:border-accent-active data-[state=checked]:bg-accent-active",
        "data-[state=indeterminate]:border-accent-active data-[state=indeterminate]:bg-accent-active",
        className,
      )}
    >
      <RadixCheckbox.Indicator className="text-accent-fg">
        {checked === "indeterminate" ? (
          <Minus className="size-3.5" strokeWidth={3} />
        ) : (
          <Check className="size-3.5" strokeWidth={3} />
        )}
      </RadixCheckbox.Indicator>
    </RadixCheckbox.Root>
  );
}

import * as RadixCheckbox from "@radix-ui/react-checkbox";
import { Check } from "lucide-react";
import type { MouseEvent } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";

export default function Checkbox({
  checked,
  onCheckedChange,
  label,
  className,
  onClick,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: UiText;
  className?: string;
  onClick?: (event: MouseEvent) => void;
}) {
  return (
    <RadixCheckbox.Root
      checked={checked}
      onCheckedChange={(value) => onCheckedChange(value === true)}
      aria-label={label}
      onClick={onClick}
      className={cn(
        "inline-flex size-5 shrink-0 items-center justify-center rounded-sm border transition-colors",
        "border-control-border bg-navbar hover:border-accent",
        "data-[state=checked]:border-accent-active data-[state=checked]:bg-accent-active",
        className,
      )}
    >
      <RadixCheckbox.Indicator className="text-accent-fg">
        <Check className="size-3.5" strokeWidth={3} />
      </RadixCheckbox.Indicator>
    </RadixCheckbox.Root>
  );
}

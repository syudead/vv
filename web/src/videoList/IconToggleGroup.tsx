import type { ReactNode } from "react";

import type { UiText } from "../i18n";
import { ToggleGroup, ToggleGroupItem } from "../ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

export interface IconToggleOption<T extends string> {
  value: T;
  label: UiText;
  icon: ReactNode;
}

export default function IconToggleGroup<T extends string>({
  value,
  onValueChange,
  options,
  label,
}: {
  value: T;
  onValueChange: (value: T) => void;
  options: readonly IconToggleOption<T>[];
  label: UiText;
}) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      value={value}
      onValueChange={(next) => {
        if (next !== "") onValueChange(next as T);
      }}
      aria-label={label}
    >
      {options.map((option) => (
        <Tooltip key={option.value}>
          {/*
           * Keep the item outside the tooltip trigger; aria-checked preserves selection styling
           * while tooltip state is active.
           */}
          <ToggleGroupItem value={option.value} asChild>
            <TooltipTrigger aria-label={option.label}>{option.icon}</TooltipTrigger>
          </ToggleGroupItem>
          <TooltipContent>{option.label}</TooltipContent>
        </Tooltip>
      ))}
    </ToggleGroup>
  );
}

import * as ToggleGroup from "@radix-ui/react-toggle-group";
import type { ReactNode } from "react";

import Tooltip from "./Tooltip";

export interface SegmentOption<T extends string> {
  value: T;
  label: string;
  icon: ReactNode;
}

/** SegmentedControl は排他的な少数の選択肢をひとつの枠に並べる。 */
export default function SegmentedControl<T extends string>({
  value,
  onValueChange,
  options,
  label,
}: {
  value: T;
  onValueChange: (value: T) => void;
  options: readonly SegmentOption<T>[];
  label: string;
}) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      onValueChange={(next) => {
        if (next !== "") onValueChange(next as T);
      }}
      aria-label={label}
      className="inline-flex h-9 items-center gap-0.5 rounded-md border border-border bg-surface p-0.5"
    >
      {options.map((option) => (
        <Tooltip key={option.value} content={option.label}>
          <ToggleGroup.Item
            value={option.value}
            aria-label={option.label}
            className="inline-flex h-full w-8 items-center justify-center rounded-sm text-fg-muted transition-colors hover:text-fg data-[state=on]:bg-surface-hover data-[state=on]:text-fg [&>svg]:size-4"
          >
            {option.icon}
          </ToggleGroup.Item>
        </Tooltip>
      ))}
    </ToggleGroup.Root>
  );
}

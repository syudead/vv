import { ToggleGroup } from "radix-ui";
import type { ReactNode } from "react";

import type { UiText } from "../../i18n";
import Tooltip from "../Tooltip";

export interface SegmentOption<T extends string> {
  value: T;
  label: UiText;
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
  label: UiText;
}) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      onValueChange={(next) => {
        if (next !== "") onValueChange(next as T);
      }}
      aria-label={label}
      className="inline-flex h-9 items-center rounded-md bg-card shadow-card"
    >
      {options.map((option) => (
        <Tooltip key={option.value} content={option.label}>
          <ToggleGroup.Item
            value={option.value}
            aria-label={option.label}
            className="inline-flex h-full w-9 items-center justify-center rounded-md border border-transparent text-muted-foreground transition-colors first:rounded-r-none last:rounded-l-none hover:text-foreground data-[state=on]:border-primary-active data-[state=on]:bg-primary-soft data-[state=on]:text-primary disabled:pointer-events-none disabled:opacity-50 [&>svg]:size-4"
          >
            {option.icon}
          </ToggleGroup.Item>
        </Tooltip>
      ))}
    </ToggleGroup.Root>
  );
}

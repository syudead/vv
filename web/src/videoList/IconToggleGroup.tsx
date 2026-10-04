import type { ReactNode } from "react";

import type { UiText } from "../i18n";
import { ToggleGroup, ToggleGroupItem } from "../ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

export interface IconToggleOption<T extends string> {
  value: T;
  label: UiText;
  icon: ReactNode;
}

/**
 * IconToggleGroup はアイコンだけの排他の選択（表示形式・並び順の向き）である。
 * shadcn/ui の ToggleGroup（type="single"、outline、sm）で、各項目に名前と同じ
 * ツールチップを添える（web/registry/rules/components.md「Toggle and ToggleGroup」）。
 * 項目は支援技術に radio として読まれる。
 */
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
          {/* 項目を外に置き、ツールチップの引き金を asChild で重ねる。ツールチップが開いている
              間は引き金の data-state（delayed-open）が選んだ状態の data-state="on" と
              ぶつかるので、選んだ見た目は aria-checked でも付ける。 */}
          <ToggleGroupItem
            value={option.value}
            asChild
            className="aria-checked:border-primary-active aria-checked:bg-primary-soft aria-checked:text-primary"
          >
            <TooltipTrigger aria-label={option.label}>{option.icon}</TooltipTrigger>
          </ToggleGroupItem>
          <TooltipContent>{option.label}</TooltipContent>
        </Tooltip>
      ))}
    </ToggleGroup>
  );
}

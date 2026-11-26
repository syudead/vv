import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { ToggleGroup, ToggleGroupItem } from "@/ui/shadcn/toggle-group";

import { type KindColour, KindMark, kindColour } from "./KindMark";

// See web/registry/rules/components.md, KindToggleGroup.

export interface KindToggleOption {
  value: string;
  label: ReactNode;
  /** The kind's id; leave it out for a plain choice such as `All` or `None`. */
  kind?: number;
  /** A count after the label, such as how many records have the kind, or a `Skeleton` while it loads. */
  count?: ReactNode;
}

export interface KindToggleGroupProps {
  /** Names the group for screen readers. */
  label: string;
  /** In the caller's order: plain choices and kinds may come in any order. */
  options: readonly KindToggleOption[];
  value: string;
  /** Called with the newly pressed choice; pressing the pressed one again reports nothing. */
  onValueChange: (value: string) => void;
  /** `scroll`: one line below md that wraps from md, for `ListPage`'s `filter`; `wrap`: wraps, for a kind picker. */
  layout?: "scroll" | "wrap";
  disabled?: boolean;
}

// Literal class names, so Tailwind generates every colour.
const pressed: Record<KindColour, string> = {
  1: "data-[state=on]:border-kind-1 data-[state=on]:bg-kind-1-soft aria-checked:border-kind-1 aria-checked:bg-kind-1-soft",
  2: "data-[state=on]:border-kind-2 data-[state=on]:bg-kind-2-soft aria-checked:border-kind-2 aria-checked:bg-kind-2-soft",
  3: "data-[state=on]:border-kind-3 data-[state=on]:bg-kind-3-soft aria-checked:border-kind-3 aria-checked:bg-kind-3-soft",
  4: "data-[state=on]:border-kind-4 data-[state=on]:bg-kind-4-soft aria-checked:border-kind-4 aria-checked:bg-kind-4-soft",
  5: "data-[state=on]:border-kind-5 data-[state=on]:bg-kind-5-soft aria-checked:border-kind-5 aria-checked:bg-kind-5-soft",
  6: "data-[state=on]:border-kind-6 data-[state=on]:bg-kind-6-soft aria-checked:border-kind-6 aria-checked:bg-kind-6-soft",
  7: "data-[state=on]:border-kind-7 data-[state=on]:bg-kind-7-soft aria-checked:border-kind-7 aria-checked:bg-kind-7-soft",
  8: "data-[state=on]:border-kind-8 data-[state=on]:bg-kind-8-soft aria-checked:border-kind-8 aria-checked:bg-kind-8-soft",
};

/** A single choice among kinds, each with its dot, and the caller's plain choices. */
export function KindToggleGroup({
  label,
  options,
  value,
  onValueChange,
  layout = "scroll",
  disabled = false,
}: KindToggleGroupProps) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      aria-label={label}
      value={value}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== "") onValueChange(next);
      }}
      data-slot="kind-toggle-group"
      className={cn(
        "gap-2",
        layout === "wrap" ? "flex-wrap" : "w-max flex-nowrap md:w-auto md:flex-wrap",
      )}
    >
      {options.map((option) => (
        <ToggleGroupItem
          key={option.value}
          value={option.value}
          className={cn(
            "flex-none rounded-full px-3 font-normal first:rounded-full last:rounded-full data-[variant=outline]:border-l",
            option.kind !== undefined &&
              cn(
                pressed[kindColour(option.kind)],
                "data-[state=on]:text-foreground aria-checked:text-foreground",
              ),
          )}
        >
          {option.kind !== undefined && <KindMark kind={option.kind} variant="dot" />}
          {option.label}
          {option.count !== undefined && (
            <span className="text-xs text-muted-foreground tabular-nums">
              {option.count}
            </span>
          )}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}

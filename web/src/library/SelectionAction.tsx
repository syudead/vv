import type { ComponentProps, ReactNode } from "react";

import type { UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * SelectionAction は選択バーの一括の操作のボタンである（ghost・sm。
 * web/registry/rules/patterns.md「Sections」の SelectionBar）。帯の幅（max-w-2xl）に収める
 * ため、名前を出すのはタグの 2 つだけで、それも sm より狭い幅では隠す（`iconOnly` の操作は
 * どの幅でもアイコンだけ）。名前は aria-label にも持ち、ツールチップで見せる。
 * Popover や DropdownMenu の引き金に asChild で渡せるよう、残りの props はボタンへ渡す。
 */
export function SelectionAction({
  icon,
  label,
  after,
  iconOnly = false,
  ...props
}: Omit<ComponentProps<typeof Button>, "children"> & {
  icon: ReactNode;
  label: UiText;
  /** どの幅でも名前を出さない。 */
  iconOnly?: boolean;
  /** アイコンの後ろの印（メニューを開く ⌄ など）。 */
  after?: ReactNode;
}) {
  return (
    <Button variant="ghost" size="sm" aria-label={label} {...props}>
      {icon}
      {!iconOnly && <span className="hidden sm:inline">{label}</span>}
      {after}
    </Button>
  );
}

/** WithTooltip は子（asChild の引き金）に名前と同じツールチップを添える。 */
export function WithTooltip({ label, children }: { label: UiText; children: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

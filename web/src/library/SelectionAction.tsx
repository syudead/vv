import type { ComponentProps, ReactNode } from "react";

import type { UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * SelectionAction は選択バーの一括の操作のボタンである（ghost・sm。
 * web/registry/rules/patterns.md「Sections」の SelectionBar）。名前はどの幅でも出し、
 * ホバーの無い機器でも読めるようにする（docs/design-docs/library-ui.md「Selection bar」）。
 * 帯に収まらなければ帯が折り返す。
 * Popover や DropdownMenu の引き金に asChild で渡せるよう、残りの props はボタンへ渡す。
 */
export function SelectionAction({
  icon,
  label,
  after,
  ...props
}: Omit<ComponentProps<typeof Button>, "children"> & {
  icon: ReactNode;
  label: UiText;
  /** アイコンの後ろの印（メニューを開く ⌄ など）。 */
  after?: ReactNode;
}) {
  return (
    <Button variant="ghost" size="sm" aria-label={label} {...props}>
      {icon}
      <span>{label}</span>
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

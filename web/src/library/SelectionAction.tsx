import type { ComponentProps, ReactNode } from "react";

import type { UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * SelectionAction は選択バーの一括の操作のボタンである（ghost・sm。
 * web/registry/rules/patterns.md「Sections」の SelectionBar）。名前はタグの 2 つは sm から、
 * ほかの操作（`labelFromLg`）は帯の幅に名前が全部収まる lg から出し、それより狭い幅では
 * アイコンだけにする。名前は aria-label にも持ち、ツールチップで見せる。
 * Popover や DropdownMenu の引き金に asChild で渡せるよう、残りの props はボタンへ渡す。
 */
export function SelectionAction({
  icon,
  label,
  after,
  labelFromLg = false,
  ...props
}: Omit<ComponentProps<typeof Button>, "children"> & {
  icon: ReactNode;
  label: UiText;
  /** 名前を lg から出す（sm から出すのはタグの 2 つ）。 */
  labelFromLg?: boolean;
  /** アイコンの後ろの印（メニューを開く ⌄ など）。 */
  after?: ReactNode;
}) {
  return (
    <Button variant="ghost" size="sm" aria-label={label} {...props}>
      {icon}
      <span className={labelFromLg ? "hidden lg:inline" : "hidden sm:inline"}>
        {label}
      </span>
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

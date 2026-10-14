import type { LucideIcon } from "lucide-react";
import { type ReactNode, useState } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverTrigger } from "../ui/shadcn/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

/**
 * PickerButton はカード・リスト表示の行・動画ページの「Add to playlist」「Add to group」の
 * 引き金である（specs/044-playlists/ui-design.md「Add to playlist」、specs/048-groups/ui-design.md
 * 「Add to group」の置き場所の表）。所有者の画面で、お気に入りのハートが描かれるところにだけ
 * 置く（呼び出し側が決める）。中身のピッカーは `picker` が開閉の状態から描く。
 *
 * - `card` はサムネイルの右上の印の置き場の形。面も枠も無い `size-8` の押せる範囲に `size-5`
 *   のアイコンを `drop-shadow-mark` で置く。
 * - `row` はリスト表示の列に置く形（`size-6`、アイコン 16px）。
 * - どちらもオフのハートと同じ条件（hover・中へのフォーカス・`hover:none`）で見え、
 *   ピッカーを開いている間は見えたままにする。押してもカードは開かず、選択も変わらない。
 * - `page` は動画ページの情報の行の操作の一群に置くアイコンだけの `ghost` のボタンで、
 *   ツールチップ（`tooltip`、無ければ `label`）を添える。常に見える。
 */
export default function PickerButton({
  variant,
  label,
  tooltip,
  icon: Icon,
  picker,
}: {
  variant: "card" | "row" | "page";
  /** 読み上げ名。 */
  label: UiText;
  /** `page` のツールチップ。省けば読み上げ名。 */
  tooltip?: UiText;
  icon: LucideIcon;
  picker: (open: boolean, onOpenChange: (open: boolean) => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const content = picker(open, setOpen);

  if (variant === "page") {
    return (
      <Popover open={open} onOpenChange={setOpen}>
        <Tooltip>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={label}
                className="text-muted-foreground"
              >
                <Icon aria-hidden="true" />
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent>{tooltip ?? label}</TooltipContent>
        </Tooltip>
        {content}
      </Popover>
    );
  }

  const card = variant === "card";
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          aria-label={label}
          // カードのリンクにも選択にも渡さない（ハートと同じ）。preventDefault は Radix の
          // 開閉を止めるので使わない。
          onClick={(event) => event.stopPropagation()}
          className={cn(
            "rounded-sm p-0 transition-opacity duration-150 hover:bg-transparent active:bg-transparent",
            card
              ? "size-8 text-foreground hover:text-foreground [&_svg]:size-5"
              : "size-6 text-muted-foreground hover:text-foreground",
            open
              ? "opacity-100"
              : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100",
          )}
        >
          <Icon aria-hidden="true" className={cn(card && "drop-shadow-mark")} />
        </Button>
      </PopoverTrigger>
      {content}
    </Popover>
  );
}

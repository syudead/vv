import { ListPlus } from "lucide-react";
import { useState } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverTrigger } from "../ui/shadcn/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import PlaylistPicker from "./PlaylistPicker";

const noIds: readonly number[] = [];

/**
 * AddToPlaylistButton はカード・リスト表示の行・動画ページの「Add to playlist」の引き金と
 * ピッカーである（specs/044-playlists/ui-design.md「Add to playlist」の置き場所の表）。
 * 所有者の画面で、お気に入りのハートが描かれるところにだけ置く（呼び出し側が決める）。
 *
 * - `card` はサムネイルの右上の印の置き場で、ハートの左に置く形。面も枠も無い `size-8` の
 *   押せる範囲に `size-5` の ListPlus を `drop-shadow-mark` で置く。
 * - `row` はリスト表示のお気に入りの列の直後の列に置く形（`size-6`、アイコン 16px）。
 * - どちらもオフのハートと同じ条件（hover・中へのフォーカス・`hover:none`）で見え、
 *   ピッカーを開いている間は見えたままにする。押してもカードは開かず、選択も変わらない。
 * - `page` は動画ページの情報の行の操作の一群の、お気に入りの直後に置くアイコンだけの
 *   `ghost` のボタンで、ツールチップ「Add to playlist」を添える。常に見える。
 */
export default function AddToPlaylistButton({
  variant,
  label,
  videoIds = noIds,
  groupIds = noIds,
}: {
  variant: "card" | "row" | "page";
  /** 読み上げ名。`page` ではツールチップの文言でもある。 */
  label: UiText;
  videoIds?: readonly number[];
  groupIds?: readonly number[];
}) {
  const [open, setOpen] = useState(false);
  const picker = (
    <PlaylistPicker
      open={open}
      onOpenChange={setOpen}
      videoIds={videoIds}
      groupIds={groupIds}
    />
  );

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
                <ListPlus aria-hidden="true" />
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent>{label}</TooltipContent>
        </Tooltip>
        {picker}
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
          <ListPlus aria-hidden="true" className={cn(card && "drop-shadow-mark")} />
        </Button>
      </PopoverTrigger>
      {picker}
    </Popover>
  );
}

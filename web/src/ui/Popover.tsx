import * as RadixPopover from "@radix-ui/react-popover";
import type { ReactNode } from "react";

import { cn } from "../lib/cn";

export const PopoverRoot = RadixPopover.Root;
export const PopoverTrigger = RadixPopover.Trigger;
export const PopoverClose = RadixPopover.Close;

export function PopoverContent({
  children,
  align = "end",
  side = "bottom",
  className,
  container,
  onOpenAutoFocus,
  onCloseAutoFocus,
  onEscapeKeyDown,
  onPointerEnter,
  onPointerLeave,
  "aria-labelledby": labelledBy,
}: {
  children: ReactNode;
  align?: "start" | "center" | "end";
  /** 吹き出しを開く向き。既定は下（トリガの下に開く）。選択バーは上に開く。 */
  side?: "top" | "bottom" | "left" | "right";
  className?: string;
  /** 吹き出しを描く先。全画面の要素の中で開くときに、その要素を渡す。既定は body。 */
  container?: HTMLElement | null;
  onOpenAutoFocus?: (event: Event) => void;
  onCloseAutoFocus?: (event: Event) => void;
  /**
   * Esc を、この吹き出しが閉じる既定の動作より前に受ける。Radix の
   * DismissableLayer は document の capture 段階で Esc を拾い、既定では
   * そのまま閉じる（`event.preventDefault()` してから）。中の部品（combobox の
   * 候補の一覧など）が Esc を自分の操作として先に使いたいときは、ここで
   * `event.preventDefault()` を呼んで既定の「閉じる」を止める
   * （ui-design.md「Combobox」）。
   */
  onEscapeKeyDown?: (event: KeyboardEvent) => void;
  onPointerEnter?: () => void;
  onPointerLeave?: () => void;
  /** 吹き出し（dialog）の読み上げ名にする見出しの id。 */
  "aria-labelledby"?: string;
}) {
  return (
    <RadixPopover.Portal container={container ?? undefined}>
      <RadixPopover.Content
        align={align}
        side={side}
        sideOffset={6}
        collisionPadding={8}
        onOpenAutoFocus={onOpenAutoFocus}
        onCloseAutoFocus={onCloseAutoFocus}
        onEscapeKeyDown={onEscapeKeyDown}
        onPointerEnter={onPointerEnter}
        onPointerLeave={onPointerLeave}
        aria-labelledby={labelledBy}
        className={cn(
          "z-50 w-72 rounded-md bg-elevated p-4 shadow-elevated animate-pop-in origin-(--radix-popover-content-transform-origin) outline-none",
          className,
        )}
      >
        {children}
      </RadixPopover.Content>
    </RadixPopover.Portal>
  );
}

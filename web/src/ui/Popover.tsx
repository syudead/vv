import * as RadixPopover from "@radix-ui/react-popover";
import type { ReactNode } from "react";

import { cn } from "../lib/cn";

export const PopoverRoot = RadixPopover.Root;
export const PopoverTrigger = RadixPopover.Trigger;
export const PopoverClose = RadixPopover.Close;

export function PopoverContent({
  children,
  align = "end",
  className,
  container,
  onOpenAutoFocus,
  onCloseAutoFocus,
  onPointerEnter,
  onPointerLeave,
  "aria-labelledby": labelledBy,
}: {
  children: ReactNode;
  align?: "start" | "center" | "end";
  className?: string;
  /** 吹き出しを描く先。全画面の要素の中で開くときに、その要素を渡す。既定は body。 */
  container?: HTMLElement | null;
  onOpenAutoFocus?: (event: Event) => void;
  onCloseAutoFocus?: (event: Event) => void;
  onPointerEnter?: () => void;
  onPointerLeave?: () => void;
  /** 吹き出し（dialog）の読み上げ名にする見出しの id。 */
  "aria-labelledby"?: string;
}) {
  return (
    <RadixPopover.Portal container={container ?? undefined}>
      <RadixPopover.Content
        align={align}
        sideOffset={6}
        collisionPadding={8}
        onOpenAutoFocus={onOpenAutoFocus}
        onCloseAutoFocus={onCloseAutoFocus}
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

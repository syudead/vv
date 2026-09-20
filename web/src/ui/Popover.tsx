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
}: {
  children: ReactNode;
  align?: "start" | "center" | "end";
  className?: string;
}) {
  return (
    <RadixPopover.Portal>
      <RadixPopover.Content
        align={align}
        sideOffset={6}
        collisionPadding={8}
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

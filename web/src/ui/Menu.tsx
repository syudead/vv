import * as Dropdown from "@radix-ui/react-dropdown-menu";
import { Check } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "../lib/cn";

/** 見た目を統一したドロップダウン。挙動（位置・キーボード・フォーカス）は Radix。 */

export const MenuRoot = Dropdown.Root;
export const MenuTrigger = Dropdown.Trigger;

export function MenuContent({
  children,
  align = "end",
  className,
}: {
  children: ReactNode;
  align?: "start" | "center" | "end";
  className?: string;
}) {
  return (
    <Dropdown.Portal>
      <Dropdown.Content
        align={align}
        sideOffset={6}
        collisionPadding={8}
        className={cn(
          "z-50 min-w-48 rounded-lg bg-elevated p-1.5 shadow-elevated animate-pop-in origin-(--radix-dropdown-menu-content-transform-origin)",
          className,
        )}
      >
        {children}
      </Dropdown.Content>
    </Dropdown.Portal>
  );
}

export function MenuLabel({ children }: { children: ReactNode }) {
  return (
    <Dropdown.Label className="px-2.5 pt-1.5 pb-1 text-xs font-medium text-fg-subtle">
      {children}
    </Dropdown.Label>
  );
}

export function MenuSeparator() {
  return <Dropdown.Separator className="my-1.5 h-px bg-border" />;
}

const itemClass =
  "relative flex h-9 cursor-default items-center gap-2.5 rounded-md px-2.5 text-sm text-fg outline-none select-none " +
  "data-highlighted:bg-surface-hover data-disabled:opacity-50 [&>svg]:size-4 [&>svg]:text-fg-muted";

export function MenuItem({
  children,
  onSelect,
  disabled,
}: {
  children: ReactNode;
  onSelect?: () => void;
  disabled?: boolean;
}) {
  return (
    <Dropdown.Item className={itemClass} onSelect={onSelect} disabled={disabled}>
      {children}
    </Dropdown.Item>
  );
}

export function MenuRadioGroup<T extends string>({
  value,
  onValueChange,
  children,
}: {
  value: T;
  onValueChange: (value: T) => void;
  children: ReactNode;
}) {
  return (
    <Dropdown.RadioGroup value={value} onValueChange={(v) => onValueChange(v as T)}>
      {children}
    </Dropdown.RadioGroup>
  );
}

export function MenuRadioItem({
  value,
  children,
}: {
  value: string;
  children: ReactNode;
}) {
  return (
    <Dropdown.RadioItem value={value} className={cn(itemClass, "pr-9")}>
      {children}
      <Dropdown.ItemIndicator className="absolute right-2.5 text-accent">
        <Check className="size-4" />
      </Dropdown.ItemIndicator>
    </Dropdown.RadioItem>
  );
}

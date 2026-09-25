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
  side = "bottom",
  className,
}: {
  children: ReactNode;
  align?: "start" | "center" | "end";
  side?: "top" | "bottom" | "left" | "right";
  className?: string;
}) {
  return (
    <Dropdown.Portal>
      <Dropdown.Content
        align={align}
        side={side}
        sideOffset={6}
        collisionPadding={8}
        className={cn(
          "z-50 min-w-44 rounded-md bg-elevated p-1 shadow-elevated animate-pop-in origin-(--radix-dropdown-menu-content-transform-origin)",
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

const itemBase =
  "relative flex h-8 cursor-default items-center gap-2.5 rounded-sm px-2.5 text-sm outline-none select-none " +
  "data-highlighted:bg-hover-wash data-disabled:opacity-50 [&>svg]:size-4";

/**
 * itemTone は項目の面ごとの文字とアイコンの色。取り消せない操作（削除など）は
 * `danger` にする。`[&>svg]:*` の組を variant ごとに1つだけ出すことで、
 * `cn` が単純結合のためクラスの重なりで色が決まらなくなるのを避ける。
 */
const itemTone = {
  default: "text-fg [&>svg]:text-fg-muted",
  danger: "text-danger [&>svg]:text-danger",
} as const;

export function MenuItem({
  children,
  onSelect,
  disabled,
  tone = "default",
}: {
  children: ReactNode;
  onSelect?: () => void;
  disabled?: boolean;
  tone?: keyof typeof itemTone;
}) {
  return (
    <Dropdown.Item
      className={cn(itemBase, itemTone[tone])}
      onSelect={onSelect}
      disabled={disabled}
    >
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
    <Dropdown.RadioItem value={value} className={cn(itemBase, itemTone.default, "pr-9")}>
      {children}
      <Dropdown.ItemIndicator className="absolute right-2.5 text-accent">
        <Check className="size-4" />
      </Dropdown.ItemIndicator>
    </Dropdown.RadioItem>
  );
}

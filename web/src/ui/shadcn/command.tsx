import { Command as CommandPrimitive } from "cmdk";
import { SearchIcon } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の Command（cmdk）。打った文字で絞り込む一覧で、Combobox の中身になる。
// 窓で開く CommandDialog は Dialog を待つので、この単位では持たない。
// 規則は web/registry/rules/components.md の Combobox and Command。

function Command({ className, ...props }: ComponentProps<typeof CommandPrimitive>) {
  return (
    <CommandPrimitive
      data-slot="command"
      className={cn(
        "flex h-full w-full flex-col overflow-hidden rounded-md bg-popover text-popover-foreground",
        className,
      )}
      {...props}
    />
  );
}

// 入力は共通のフォーカスの輪（index.css の :focus-visible）を出す。上流の outline-hidden は
// その輪を消すので外し、輪が Command の overflow-hidden で切れないよう入力を行より低い
// h-6 にしている。
// 入力を囲む枠は wrapperClassName で、入力の後ろに置く印（送信中の回転など）は trailing で
// 足せる（ui/TagCommand の dropdown・inline。web/registry/rules/components.md の
// TagCommand）。
function CommandInput({
  className,
  icon,
  wrapperClassName,
  trailing,
  ...props
}: ComponentProps<typeof CommandPrimitive.Input> & {
  /** 入力の先頭の印。既定は虫眼鏡。 */
  icon?: ReactNode;
  /** 入力を囲む枠のクラス。既定の枠に重ねる。 */
  wrapperClassName?: string;
  /** 入力の後ろ、枠の中に置くもの。 */
  trailing?: ReactNode;
}) {
  return (
    <div
      data-slot="command-input-wrapper"
      className={cn(
        "flex h-9 items-center gap-2 border-b border-border px-3 [&>svg]:size-4 [&>svg]:shrink-0 [&>svg]:text-muted-foreground",
        wrapperClassName,
      )}
    >
      {icon ?? <SearchIcon aria-hidden="true" />}
      <CommandPrimitive.Input
        data-slot="command-input"
        className={cn(
          "flex h-6 w-full min-w-0 rounded-sm bg-transparent px-1 text-sm text-foreground placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        {...props}
      />
      {trailing}
    </div>
  );
}

function CommandList({
  className,
  ...props
}: ComponentProps<typeof CommandPrimitive.List>) {
  return (
    <CommandPrimitive.List
      data-slot="command-list"
      className={cn(
        "max-h-popover scroll-py-1 overflow-x-hidden overflow-y-auto",
        className,
      )}
      {...props}
    />
  );
}

function CommandEmpty({
  className,
  ...props
}: ComponentProps<typeof CommandPrimitive.Empty>) {
  return (
    <CommandPrimitive.Empty
      data-slot="command-empty"
      className={cn("py-6 text-center text-sm text-muted-foreground", className)}
      {...props}
    />
  );
}

function CommandGroup({
  className,
  heading,
  ...props
}: ComponentProps<typeof CommandPrimitive.Group>) {
  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      className={cn("overflow-hidden p-1 text-foreground", className)}
      // 見出しは cmdk の [cmdk-group-heading] の中に入る。属性の選択子は入れ子の [] に
      // なって lint で落ちるので、見出しの見た目はここで包んで付ける。
      heading={
        heading === undefined ? undefined : (
          <span className="block px-2 py-1.5 text-xs font-medium text-muted-foreground">
            {heading}
          </span>
        )
      }
      {...props}
    />
  );
}

function CommandSeparator({
  className,
  ...props
}: ComponentProps<typeof CommandPrimitive.Separator>) {
  return (
    <CommandPrimitive.Separator
      data-slot="command-separator"
      className={cn("-mx-1 h-px bg-border", className)}
      {...props}
    />
  );
}

function CommandItem({
  className,
  ...props
}: ComponentProps<typeof CommandPrimitive.Item>) {
  return (
    <CommandPrimitive.Item
      data-slot="command-item"
      className={cn(
        "relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-foreground outline-hidden select-none data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
        className,
      )}
      {...props}
    />
  );
}

function CommandShortcut({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      data-slot="command-shortcut"
      className={cn("ml-auto text-xs tracking-widest text-muted-foreground", className)}
      {...props}
    />
  );
}

export {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
};

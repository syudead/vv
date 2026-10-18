import { Command as CommandPrimitive } from "cmdk";
import { SearchIcon } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";

import { cn } from "@/lib/cn";

// Adapted from shadcn/ui Command (cmdk); see web/registry/rules/components.md, Combobox and Command.

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

// Remove upstream outline-hidden and shrink the input to h-6 so the shared focus ring is not hidden or clipped.
// TagCommand）。
function CommandInput({
  className,
  icon,
  wrapperClassName,
  trailing,
  ...props
}: ComponentProps<typeof CommandPrimitive.Input> & {
  icon?: ReactNode;
  wrapperClassName?: string;
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
      // Wrap heading styling because cmdk's nested attribute selectors fail the class lint rule.
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

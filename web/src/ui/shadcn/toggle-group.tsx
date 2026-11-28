import type { VariantProps } from "class-variance-authority";
import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui";
import { type ComponentProps, createContext, useContext } from "react";

import { cn } from "@/lib/cn";
import { toggleVariants } from "@/ui/shadcn/toggle";

// Adapted from shadcn/ui Radix ToggleGroup; single items expose radio semantics.
// spacing="separate" stands the items apart as chips, each with its own radius and border (components.md, Toggle and ToggleGroup).

type Spacing = "joined" | "separate";

const ToggleGroupContext = createContext<
  VariantProps<typeof toggleVariants> & { spacing?: Spacing }
>({
  size: "default",
  variant: "default",
  spacing: "joined",
});

function ToggleGroup({
  className,
  variant,
  size,
  spacing = "joined",
  children,
  ...props
}: ComponentProps<typeof ToggleGroupPrimitive.Root> &
  VariantProps<typeof toggleVariants> & { spacing?: Spacing }) {
  return (
    <ToggleGroupPrimitive.Root
      data-slot="toggle-group"
      data-variant={variant}
      data-size={size}
      data-spacing={spacing}
      className={cn(
        "group/toggle-group flex w-fit items-center rounded-md",
        spacing === "separate" && "gap-2",
        className,
      )}
      {...props}
    >
      <ToggleGroupContext.Provider value={{ variant, size, spacing }}>
        {children}
      </ToggleGroupContext.Provider>
    </ToggleGroupPrimitive.Root>
  );
}

function ToggleGroupItem({
  className,
  children,
  variant,
  size,
  ...props
}: ComponentProps<typeof ToggleGroupPrimitive.Item> &
  VariantProps<typeof toggleVariants>) {
  const context = useContext(ToggleGroupContext);

  return (
    <ToggleGroupPrimitive.Item
      data-slot="toggle-group-item"
      data-variant={context.variant ?? variant}
      data-size={context.size ?? size}
      className={cn(
        toggleVariants({
          variant: context.variant ?? variant,
          size: context.size ?? size,
        }),
        "min-w-0 shrink-0 focus:z-10 focus-visible:z-10",
        context.spacing === "separate"
          ? "flex-none"
          : "flex-1 rounded-none first:rounded-l-md last:rounded-r-md data-[variant=outline]:border-l-0 data-[variant=outline]:first:border-l",
        className,
      )}
      {...props}
    >
      {children}
    </ToggleGroupPrimitive.Item>
  );
}

export { ToggleGroup, ToggleGroupItem };

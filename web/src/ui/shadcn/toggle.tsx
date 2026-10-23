import { cva, type VariantProps } from "class-variance-authority";
import { Toggle as TogglePrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

// Also style aria-checked and aria-pressed because Tooltip can replace the toggle's data-state while open.
const toggleVariants = cva(
  "inline-flex items-center justify-center gap-2 text-sm font-medium whitespace-nowrap text-foreground transition-colors hover:bg-accent hover:text-accent-foreground active:bg-accent/70 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive data-[state=on]:bg-primary-soft data-[state=on]:text-primary aria-checked:bg-primary-soft aria-checked:text-primary aria-pressed:bg-primary-soft aria-pressed:text-primary [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "bg-transparent",
        outline:
          "border border-input bg-transparent data-[state=on]:border-primary-active aria-checked:border-primary-active aria-pressed:border-primary-active",
      },
      size: {
        default: "h-9 min-w-9 px-2",
        sm: "h-8 min-w-8 px-1.5",
        lg: "h-10 min-w-10 px-3",
      },
      shape: {
        default: "rounded-md",
        pill: "rounded-full",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
      shape: "default",
    },
  },
);

function Toggle({
  className,
  variant,
  size,
  shape,
  ...props
}: ComponentProps<typeof TogglePrimitive.Root> & VariantProps<typeof toggleVariants>) {
  return (
    <TogglePrimitive.Root
      data-slot="toggle"
      data-shape={shape ?? "default"}
      className={cn(toggleVariants({ variant, size, shape, className }))}
      {...props}
    />
  );
}

export { Toggle, toggleVariants };

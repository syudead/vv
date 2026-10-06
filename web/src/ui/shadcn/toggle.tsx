import { cva, type VariantProps } from "class-variance-authority";
import { Toggle as TogglePrimitive } from "radix-ui";
import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の Toggle（radix）。押した状態（data-state=on）は primary-soft の地と
// primary の文字で、選択と同じ見た目にする。Tooltip の引き金を asChild で重ねると、
// 開いている間は data-state が引き金のもの（delayed-open）に置き換わるので、同じ見た目を
// aria-checked（ToggleGroup の項目）と aria-pressed（単独の Toggle）にも付ける。
// 規則は web/registry/rules/components.md の Toggle and ToggleGroup。
const toggleVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap text-foreground transition-colors hover:bg-accent hover:text-accent-foreground active:bg-accent/70 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive data-[state=on]:bg-primary-soft data-[state=on]:text-primary aria-checked:bg-primary-soft aria-checked:text-primary aria-pressed:bg-primary-soft aria-pressed:text-primary [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
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
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

function Toggle({
  className,
  variant,
  size,
  ...props
}: ComponentProps<typeof TogglePrimitive.Root> & VariantProps<typeof toggleVariants>) {
  return (
    <TogglePrimitive.Root
      data-slot="toggle"
      className={cn(toggleVariants({ variant, size, className }))}
      {...props}
    />
  );
}

export { Toggle, toggleVariants };

import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の alert（radix-nova）を vv のトークンで着せたもの。区画の中の失敗・注意・案内
// （web/registry/rules/components.md「Alert」）。上流の default と destructive に、vv の状態の
// 役割（tokens.css の warning・success）を同じ形で足す。

const alertVariants = cva(
  "group/alert relative grid w-full gap-0.5 rounded-lg border px-3 py-2 text-left text-sm has-data-[slot=alert-action]:pr-16 has-[>svg]:grid-cols-[auto_1fr] has-[>svg]:gap-x-2 *:[svg]:row-span-2 *:[svg]:translate-y-0.5 *:[svg]:text-current *:[svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "border-border bg-card text-card-foreground",
        destructive:
          "border-transparent bg-destructive-soft text-destructive *:data-[slot=alert-description]:text-foreground",
        warning:
          "border-transparent bg-warning-soft text-warning *:data-[slot=alert-description]:text-foreground",
        success:
          "border-transparent bg-success-soft text-success *:data-[slot=alert-description]:text-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

function Alert({
  className,
  variant,
  ...props
}: React.ComponentProps<"div"> & VariantProps<typeof alertVariants>) {
  return (
    <div
      data-slot="alert"
      role="alert"
      className={cn(alertVariants({ variant }), className)}
      {...props}
    />
  );
}

function AlertTitle({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-title"
      className={cn(
        "font-medium group-has-[>svg]/alert:col-start-2 [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

function AlertDescription({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-description"
      className={cn(
        "text-sm text-balance text-muted-foreground group-has-[>svg]/alert:col-start-2 md:text-pretty [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground [&_p:not(:last-child)]:mb-4",
        className,
      )}
      {...props}
    />
  );
}

function AlertAction({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-action"
      className={cn("absolute top-1 right-1", className)}
      {...props}
    />
  );
}

export { Alert, AlertAction, AlertDescription, AlertTitle, alertVariants };

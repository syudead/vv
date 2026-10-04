import { cva, type VariantProps } from "class-variance-authority";
import { AlertDialog as AlertDialogPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の alert-dialog（radix-nova）を vv のトークンで着せたもの。取り消せない操作の確認に
// 使う（web/registry/rules/components.md「AlertDialog」）。

function AlertDialog({
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Root>) {
  return <AlertDialogPrimitive.Root data-slot="alert-dialog" {...props} />;
}

function AlertDialogTrigger({
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Trigger>) {
  return <AlertDialogPrimitive.Trigger data-slot="alert-dialog-trigger" {...props} />;
}

function AlertDialogPortal({
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Portal>) {
  return <AlertDialogPrimitive.Portal data-slot="alert-dialog-portal" {...props} />;
}

function AlertDialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Overlay>) {
  return (
    <AlertDialogPrimitive.Overlay
      data-slot="alert-dialog-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-overlay data-open:animate-fade-in motion-reduce:animate-none",
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogContent({
  className,
  size = "default",
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Content> & {
  size?: "default" | "sm";
}) {
  return (
    <AlertDialogPortal>
      <AlertDialogOverlay />
      <AlertDialogPrimitive.Content
        data-slot="alert-dialog-content"
        data-size={size}
        className={cn(
          "group/alert-dialog-content fixed top-1/2 right-4 left-4 z-50 mx-auto grid max-w-sm -translate-y-1/2 gap-4 rounded-lg bg-popover p-4 text-popover-foreground shadow-elevated outline-none data-open:animate-pop-in motion-reduce:animate-none data-[size=sm]:max-w-xs",
          className,
        )}
        {...props}
      />
    </AlertDialogPortal>
  );
}

function AlertDialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-header"
      className={cn(
        "flex flex-col gap-2 text-left group-data-[size=sm]/alert-dialog-content:items-center group-data-[size=sm]/alert-dialog-content:text-center",
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogFooter({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-footer"
      className={cn(
        "-mx-4 -mb-4 flex flex-col-reverse gap-2 rounded-b-lg border-t border-border bg-muted p-4 group-data-[size=sm]/alert-dialog-content:grid group-data-[size=sm]/alert-dialog-content:grid-cols-2 sm:flex-row sm:justify-end",
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogMedia({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-media"
      className={cn(
        "mb-2 inline-flex size-10 items-center justify-center rounded-md bg-muted [&_svg]:size-6",
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Title>) {
  return (
    <AlertDialogPrimitive.Title
      data-slot="alert-dialog-title"
      className={cn("text-lg font-semibold", className)}
      {...props}
    />
  );
}

function AlertDialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Description>) {
  return (
    <AlertDialogPrimitive.Description
      data-slot="alert-dialog-description"
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  );
}

/**
 * 確認と取り消しのボタンの見た目。上流は Button（Issue 770 の部品）を asChild で被せるが、
 * 部品の単位を並行して作るため、同じ variant 名（default・outline・destructive）と大きさで
 * ここに持つ。Button が入ったら、上流どおり Button へ戻す。
 */
const actionVariants = cva(
  "inline-flex h-8 shrink-0 items-center justify-center gap-2 rounded-md border border-transparent px-3 text-sm font-medium whitespace-nowrap outline-none transition-colors select-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-active",
        outline: "border-input bg-transparent text-foreground hover:bg-accent",
        destructive:
          "bg-destructive-strong text-destructive-foreground hover:bg-destructive-strong/80",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

function AlertDialogAction({
  className,
  variant = "default",
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Action> &
  VariantProps<typeof actionVariants>) {
  return (
    <AlertDialogPrimitive.Action
      data-slot="alert-dialog-action"
      className={cn(actionVariants({ variant }), className)}
      {...props}
    />
  );
}

function AlertDialogCancel({
  className,
  variant = "outline",
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Cancel> &
  VariantProps<typeof actionVariants>) {
  return (
    <AlertDialogPrimitive.Cancel
      data-slot="alert-dialog-cancel"
      className={cn(actionVariants({ variant }), className)}
      {...props}
    />
  );
}

export {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogTitle,
  AlertDialogTrigger,
};

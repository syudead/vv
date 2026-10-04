import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react";
import type * as React from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";

// shadcn/ui の sonner（radix-nova）を vv のトークンで着せたもの。通知は `sonner` の
// `toast()` で出し、Toaster はアプリに 1 つ置く（web/registry/rules/components.md「Sonner」）。
// vv は暗い配色だけなので theme は dark に固定する。

const Toaster = ({ ...props }: ToasterProps) => {
  return (
    <Sonner
      theme="dark"
      duration={2800}
      icons={{
        success: <CircleCheckIcon className="size-4 text-success" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4 text-warning" />,
        error: <OctagonXIcon className="size-4 text-destructive" />,
        loading: (
          <Loader2Icon className="size-4 animate-spin motion-reduce:animate-none" />
        ),
      }}
      style={
        {
          "--normal-bg": "var(--color-popover)",
          "--normal-text": "var(--color-popover-foreground)",
          "--normal-border": "var(--color-border)",
          "--border-radius": "var(--radius-lg)",
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: "shadow-elevated text-sm",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };

import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の Textarea。Input と同じ地と縁で、高さは中身に合わせて伸びる。
function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "flex field-sizing-content min-h-16 w-full rounded-md border border-input bg-muted px-3 py-2 text-sm text-foreground transition-colors placeholder:text-muted-foreground hover:border-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive",
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };

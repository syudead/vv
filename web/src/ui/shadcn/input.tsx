import type { ComponentProps } from "react";

import { cn } from "@/lib/cn";

// shadcn/ui の Input。地は muted、縁は input、誤りは aria-invalid で destructive の縁。
// 規則は web/registry/rules/components.md の Input, Textarea, Label and Field。
function Input({ className, type, ...props }: ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-9 w-full min-w-0 rounded-md border border-input bg-muted px-3 py-1 text-sm text-foreground transition-colors selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-8 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground hover:border-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive",
        className,
      )}
      {...props}
    />
  );
}

export { Input };

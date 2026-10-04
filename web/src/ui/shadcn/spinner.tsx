import { Loader2Icon } from "lucide-react";
import type * as React from "react";

import { t } from "@/i18n";
import { cn } from "@/lib/cn";

// shadcn/ui の spinner（radix-nova）を vv のトークンで着せたもの。一覧の末尾の追加読み込みや
// 押した操作の待ちに使う（web/registry/rules/components.md「Spinner」）。

function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
  return (
    <Loader2Icon
      data-slot="spinner"
      role="status"
      aria-label={t.common.loading}
      className={cn("size-4 animate-spin motion-reduce:animate-none", className)}
      {...props}
    />
  );
}

export { Spinner };

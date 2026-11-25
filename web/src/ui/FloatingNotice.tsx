import type { ReactNode } from "react";

import { Button } from "@/ui/shadcn/button";

// See web/registry/rules/components.md, FloatingNotice. ListPage's notice slot sticks it over the body.

export interface FloatingNoticeProps {
  message: ReactNode;
  actionLabel: ReactNode;
  onAction: () => void;
}

/** One line and one action, for a condition that lasts until the viewer acts. */
export function FloatingNotice({ message, actionLabel, onAction }: FloatingNoticeProps) {
  return (
    <div
      role="status"
      data-slot="floating-notice"
      className="flex max-w-full animate-fade-in items-center gap-2 rounded-full bg-popover py-1 ps-4 pe-1 text-sm text-popover-foreground shadow-elevated motion-reduce:animate-none"
    >
      <span className="min-w-0 truncate">{message}</span>
      <Button variant="ghost" size="sm" shape="pill" onClick={onAction}>
        {actionLabel}
      </Button>
    </div>
  );
}

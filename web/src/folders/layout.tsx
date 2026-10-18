import type { ReactNode } from "react";

import { formatNumber, type UiText } from "../i18n";

export function Section({
  title,
  count,
  action,
  children,
}: {
  title: UiText;
  count?: number;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2">
      <div className="flex min-h-8 items-center justify-between gap-2">
        <h2 className="text-xs font-semibold text-muted-foreground">
          {title}
          {count !== undefined && (
            // Use a literal space so assistive technology separates the name and count.
            <span className="tabular-nums"> {formatNumber(count)}</span>
          )}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}

import type { ReactNode } from "react";

import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/ui/shadcn/empty";

// 空（状態）。骨格の本体の位置に、印・なぜ空かの 1 行・説明と、見る人が取れるなら空を
// 埋める操作を出す。規則は web/registry/rules/patterns.md の States。

export interface EmptyStateProps {
  /** lucide の印。 */
  icon: ReactNode;
  /** なぜ空か（「No videos yet」）。 */
  title: ReactNode;
  description?: ReactNode;
  /** 空を埋める操作（default の Button 1 つ）。見る人が取れないときは渡さない。 */
  action?: ReactNode;
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <Empty data-state="empty" className="border border-border py-12">
      <EmptyHeader>
        <EmptyMedia variant="icon">{icon}</EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  );
}

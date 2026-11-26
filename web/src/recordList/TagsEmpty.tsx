import type { ReactNode } from "react";

import type { UiText } from "../i18n";
import { EmptyState } from "../ui/patterns/empty-state";

/** Use h2 because the shared state title has no heading semantics. */
export function TagsEmpty({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: UiText;
  description?: UiText;
  action: ReactNode;
}) {
  return (
    <EmptyState
      icon={icon}
      title={<h2>{title}</h2>}
      description={description}
      action={action}
    />
  );
}

import type { ReactNode } from "react";
import { Link } from "react-router";

import { EmptyState } from "../ui/patterns/empty-state";
import { Button } from "../ui/shadcn/button";

/** A record page's gone state: the record no longer exists, with a link back to its list. */
export function CreditGone({
  icon,
  title,
  backLabel,
  backTo,
}: {
  icon: ReactNode;
  title: string;
  backLabel: string;
  backTo: string;
}) {
  return (
    <EmptyState
      icon={icon}
      title={title}
      action={
        <Button asChild variant="outline" size="sm">
          <Link to={backTo}>{backLabel}</Link>
        </Button>
      }
    />
  );
}

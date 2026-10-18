import { FolderOpen, type LucideIcon, SearchX } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";

import { currentPath, loginPath } from "../auth/pageNavigation";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { EmptyState as EmptyStateBlock } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";

// Use h2 state titles so screen-reader heading navigation can reach them; see web/registry/rules/patterns.md, States.

/** Use danger when the entire screen is unavailable. */
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  tone = "neutral",
}: {
  icon: LucideIcon;
  title: UiText;
  description?: ReactNode;
  action?: ReactNode;
  tone?: "neutral" | "danger";
}) {
  return (
    <EmptyStateBlock
      icon={
        <Icon
          aria-hidden="true"
          className={cn(tone === "danger" && "text-destructive")}
        />
      }
      title={<h2>{title}</h2>}
      description={description}
      action={action}
    />
  );
}

/** Offer guests login rather than owner-only import or settings actions. */
export function GuestEmpty() {
  const location = useLocation();
  return (
    <EmptyState
      icon={FolderOpen}
      title={t.list.guestEmpty.title}
      description={t.list.guestEmpty.description}
      action={
        <Button asChild size="sm">
          <Link to={loginPath(currentPath(location))}>{t.list.guestEmpty.signIn}</Link>
        </Button>
      }
    />
  );
}

export function NoMatches({ onSearch }: { onSearch?: () => void }) {
  return (
    <EmptyState
      icon={SearchX}
      title={t.list.noMatches}
      description={t.list.noMatchesHint}
      action={
        onSearch && (
          <Button size="sm" onClick={onSearch}>
            {t.list.changeSearch}
          </Button>
        )
      }
    />
  );
}

export function LoadFailed({ reason, onRetry }: { reason: UiText; onRetry: () => void }) {
  return (
    <ErrorState
      title={<h2>{t.list.loadFailed}</h2>}
      description={reason}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

/** Retain loaded rows and retry only the failed continuation. */
export function LoadMoreFailed({
  reason,
  onRetry,
}: {
  reason: UiText;
  onRetry: () => void;
}) {
  return (
    <LoadMoreRow
      status="failed"
      title={t.list.loadMoreFailed(reason)}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

export function CardSkeleton({ count }: { count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, index) => (
        <div key={index} className="flex flex-col gap-2" aria-hidden="true">
          <Skeleton className="aspect-video w-full" />
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-3 w-1/2" />
        </div>
      ))}
    </>
  );
}

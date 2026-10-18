import { FolderX } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import { t, type UiText } from "../i18n";
import type { Zoom } from "../preferences/viewPreferences";
import { EmptyState } from "../ui/patterns/empty-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { Button } from "../ui/shadcn/button";
import { FOLDERS_ROOT } from "./folderPath";

export { GuestEmpty, LoadFailed, NoMatches } from "../videoList/states";

// Wrap state titles in h2 so screen-reader heading navigation can reach them.

export function FolderEmpty({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: UiText;
  description?: UiText;
  action?: ReactNode;
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

export function FolderNotFound() {
  return (
    <FolderEmpty
      icon={<FolderX aria-hidden="true" />}
      title={t.folders.notFound.title}
      description={t.folders.notFound.description}
      action={
        <Button asChild size="sm">
          <Link to={FOLDERS_ROOT}>{t.folders.notFound.back}</Link>
        </Button>
      }
    />
  );
}

export function MoreRow({
  loadingMore,
  error,
  onRetry,
}: {
  loadingMore: boolean;
  /** Pass pagination failures only when existing rows remain visible. */
  error: UiText | null;
  onRetry: () => void;
}) {
  if (error !== null) {
    return (
      <LoadMoreRow
        status="failed"
        title={t.list.loadMoreFailed(error)}
        retryLabel={t.common.retry}
        onRetry={onRetry}
      />
    );
  }
  return loadingMore ? <LoadMoreRow status="loading" label={t.common.loading} /> : null;
}

export function CardsLoading({ zoom, count }: { zoom: Zoom; count: number }) {
  return <LoadingState label={t.list.loading} layout="grid" size={zoom} count={count} />;
}

export function ResultCount({ children }: { children: ReactNode }) {
  return (
    <p
      role="status"
      aria-live="polite"
      className="text-center text-xs text-muted-foreground tabular-nums"
    >
      {children}
    </p>
  );
}

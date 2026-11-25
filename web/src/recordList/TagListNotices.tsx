import { CircleAlert, RefreshCw, RotateCw } from "lucide-react";

import { t, type UiText } from "../i18n";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { Alert, AlertAction, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";

export function TagLoadingMore({ label = t.tags.loadingMore }: { label?: UiText }) {
  return <LoadMoreRow status="loading" label={label} />;
}

/** Retain loaded rows and retry the same cursor. */
export function TagLoadMoreFailed({
  reason,
  onRetry,
}: {
  reason: UiText;
  onRetry: () => void;
}) {
  return (
    <LoadMoreRow
      status="failed"
      title={t.tags.loadMoreFailed(reason)}
      retryLabel={t.common.retry}
      onRetry={onRetry}
    />
  );
}

/** A changed total is informational, not a request failure; restart from page one. */
export function TagListChanged({
  onReload,
  message = t.tags.listChanged,
}: {
  onReload: () => void;
  message?: UiText;
}) {
  return (
    <Alert role="status">
      <RefreshCw aria-hidden="true" />
      <AlertTitle>{message}</AlertTitle>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={onReload}>
          {t.tags.reloadList}
        </Button>
      </AlertAction>
    </Alert>
  );
}

/** Keep the disabled Retry button mounted during loading to preserve focus. */
export function TagLoadFailed({
  pending,
  onRetry,
  title = t.tags.loadFailed,
}: {
  pending: boolean;
  onRetry: () => void;
  title?: UiText;
}) {
  return (
    <Alert data-state="error" variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>
        <h2>{title}</h2>
      </AlertTitle>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={onRetry} disabled={pending}>
          <RotateCw aria-hidden="true" />
          {t.common.retry}
        </Button>
      </AlertAction>
    </Alert>
  );
}

export function TagStaleList({
  reason,
  pending,
  onRetry,
  message = t.tags.staleList,
}: {
  reason: UiText;
  pending: boolean;
  onRetry: () => void;
  message?: (reason: UiText) => UiText;
}) {
  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{message(reason)}</AlertTitle>
      <AlertAction>
        <Button variant="outline" size="sm" onClick={onRetry} disabled={pending}>
          <RotateCw aria-hidden="true" />
          {t.common.retry}
        </Button>
      </AlertAction>
    </Alert>
  );
}

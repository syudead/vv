import { CircleAlert } from "lucide-react";
import { type ReactNode, useState } from "react";

import { errorText, t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { PageSection } from "../ui/patterns/page-section";
import { Alert, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { useToast } from "../ui/Toast";

// The Progress section and loading state shared by the pages of features that analyse videos.

export type RunScope = "missing" | "all";

export interface RunProgressText {
  progressHeading: UiText;
  off: UiText;
  offHint: UiText;
  runMissing: UiText;
  /** Absent for a feature whose analysed videos are never queued again by the owner. */
  runAll?: UiText;
  lastFailure: UiText;
}

export interface RunCounts {
  queued: number;
  running: number;
  done: number;
  failed: number;
}

export function RunProgressSection({
  text,
  enabled,
  counts,
  lastFailure,
  starting,
  locked,
  onStart,
}: {
  text: RunProgressText & {
    queue: (queued: number, running: number, done: number, failed: number) => UiText;
  };
  enabled: boolean;
  counts: RunCounts;
  lastFailure: string | undefined;
  starting: boolean;
  locked?: boolean;
  onStart: (scope: RunScope) => void;
}) {
  if (!enabled)
    return (
      <PageSection title={text.progressHeading} description={text.off}>
        <p>{text.offHint}</p>
      </PageSection>
    );
  const disabled = locked || starting || undefined;
  return (
    <PageSection
      title={text.progressHeading}
      description={
        <span role="status">
          {text.queue(counts.queued, counts.running, counts.done, counts.failed)}
        </span>
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => onStart("missing")}
          aria-disabled={disabled}
        >
          {text.runMissing}
        </Button>
        {text.runAll !== undefined && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onStart("all")}
            aria-disabled={disabled}
          >
            {text.runAll}
          </Button>
        )}
      </div>
      {lastFailure !== undefined && (
        <div>
          <Alert variant="warning" role="note">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{text.lastFailure}</AlertTitle>
            <AlertDescription>
              <p className="break-all">{lastFailure}</p>
            </AlertDescription>
          </Alert>
        </div>
      )}
    </PageSection>
  );
}

/** Ignores a press while locked or while a start is pending, then reloads the counts. */
export function useRunStart({
  start,
  started,
  startFailed,
  reload,
  locked,
}: {
  start: (scope: RunScope) => Promise<number>;
  started: (count: number) => UiText;
  startFailed: (reason: UiText) => UiText;
  reload: () => Promise<void>;
  locked?: boolean;
}) {
  const toast = useToast();
  const [starting, setStarting] = useState<RunScope | null>(null);

  const run = async (scope: RunScope) => {
    if (locked || starting !== null) return;
    setStarting(scope);
    try {
      toast(started(await start(scope)));
      await reload();
    } catch (failure) {
      toast(startFailed(errorText(failure)));
    } finally {
      setStarting(null);
    }
  };

  return { starting: starting !== null, run };
}

export function RunSectionsLoading({
  progressHeading,
  heading,
  description,
  loadingLabel,
  loadError,
  onRetry,
}: {
  progressHeading: UiText;
  heading: UiText;
  description: ReactNode;
  loadingLabel: UiText;
  /** Already formatted with the page's loadFailed sentence. */
  loadError: UiText | null;
  onRetry: () => void;
}) {
  return (
    <>
      {loadError === null && (
        <PageSection title={progressHeading}>
          <Skeleton className="h-8" />
        </PageSection>
      )}
      <PageSection title={heading} description={description}>
        {loadError !== null ? (
          <div>
            <ErrorState title={loadError} retryLabel={t.common.retry} onRetry={onRetry} />
          </div>
        ) : (
          <div role="status" aria-label={loadingLabel} className="flex flex-col gap-3">
            <Skeleton className="h-9" />
            <Skeleton className="h-9" />
          </div>
        )}
      </PageSection>
    </>
  );
}

import type { ReactNode } from "react";

import { t, type UiText } from "../i18n";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { ProfilePage } from "../ui/patterns/profile-page";
import { Skeleton } from "../ui/shadcn/skeleton";

function IdentitySkeleton() {
  return (
    <>
      <Skeleton className="h-6 w-3/4" />
      <Skeleton className="h-4 w-1/2" />
      <Skeleton className="h-10 w-full" />
    </>
  );
}

/**
 * A record's profile page before the record is shown: skeletons while it loads, the failure with
 * Retry in place of the shelves, or `gone` in place of the whole page.
 */
export function CreditPageStatus({
  state,
  profileLabel,
  faceClassName,
  failedTitle,
  gone,
  onRetry,
}: {
  state: { kind: "loading" } | { kind: "failed"; reason: UiText } | { kind: "gone" };
  profileLabel: string;
  /** The face skeleton's frame, matching the image the page will show. */
  faceClassName: string;
  failedTitle: (reason: string) => string;
  gone: ReactNode;
  onRetry: () => void;
}) {
  if (state.kind === "gone") {
    return (
      <ProfilePage profileLabel={profileLabel} face={null} identity={null} status={gone}>
        {null}
      </ProfilePage>
    );
  }
  const loading = state.kind === "loading";
  return (
    <ProfilePage
      profileLabel={profileLabel}
      face={loading ? <Skeleton className={faceClassName} /> : null}
      identity={loading ? <IdentitySkeleton /> : null}
    >
      {state.kind === "failed" ? (
        <ErrorState
          title={failedTitle(state.reason)}
          retryLabel={t.common.retry}
          onRetry={onRetry}
        />
      ) : (
        <LoadingState
          label={t.credits.personPage.loading}
          layout="grid"
          size={0}
          count={6}
        />
      )}
    </ProfilePage>
  );
}

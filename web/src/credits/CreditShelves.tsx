import { Film } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useLocation } from "react-router";

import type { components } from "../api/gen/openapi";
import { useVideos, type VideosState } from "../api/useVideos";
import { t } from "../i18n";
import { CardGrid } from "../ui/patterns/card-grid";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { Shelf } from "../ui/patterns/shelf";
import { DEFAULT_SORT } from "../videoList/listCriteria";
import { useLoadMoreSentinel } from "../videoList/pageHooks";
import VideoCard from "../videoList/VideoCard";

// One shelf per role for the person page and the organization page; see docs/design-docs/people.md.

type PersonRole = components["schemas"]["WorkPersonRole"];
type OrganizationRole = components["schemas"]["WorkOrganizationRole"];

/** Whose videos the shelves show, and the roles in their order with each shelf's heading. */
export type ShelfSubject =
  | {
      kind: "person";
      id: number;
      roles: readonly { role: PersonRole; title: string }[];
    }
  | {
      kind: "organization";
      id: number;
      roles: readonly { role: OrganizationRole; title: string }[];
    };

const text = t.credits.personPage;

const noPreviewReset = () => undefined;

/** The subject's videos in the library's default order, in one role or in any. */
function useSubjectVideos(
  kind: ShelfSubject["kind"],
  id: number,
  role?: string,
): VideosState {
  const criteria = useMemo(
    () =>
      kind === "person"
        ? { sort: DEFAULT_SORT, person: id, personRole: role as PersonRole | undefined }
        : {
            sort: DEFAULT_SORT,
            organization: id,
            organizationRole: role as OrganizationRole | undefined,
          },
    [kind, id, role],
  );
  return useVideos(criteria, undefined, "videos");
}

/**
 * One shelf per role the subject has videos in, in the roles' order. The subject's videos in any
 * role stand for the shelves while loading or failing, and with none the empty state shows instead.
 * With `keepFailedRole` a role whose first read fails keeps its shelf with the failure.
 */
export function CreditShelves({
  subject,
  keepFailedRole = false,
}: {
  subject: ShelfSubject;
  keepFailedRole?: boolean;
}) {
  const all = useSubjectVideos(subject.kind, subject.id);
  if (all.loading) {
    return <LoadingState label={text.loading} layout="grid" size={0} count={6} />;
  }
  if (all.error !== null && all.items.length === 0) {
    return (
      <ErrorState
        title={text.videosFailed(all.error)}
        retryLabel={t.common.retry}
        onRetry={all.reload}
      />
    );
  }
  if (all.items.length === 0) {
    return <EmptyState icon={<Film aria-hidden="true" />} title={text.noVideos} />;
  }
  return (
    <>
      {subject.roles.map(({ role, title }) => (
        <RoleShelf
          key={role}
          kind={subject.kind}
          id={subject.id}
          role={role}
          title={title}
          keepFailed={keepFailedRole}
        />
      ))}
    </>
  );
}

/**
 * Every video in one role, wrapping and read a page at a time as the end comes into view. A role
 * without videos draws nothing; one whose first read failed draws nothing too, unless `keepFailed`,
 * which also keeps the heading while that failed read is retried.
 */
function RoleShelf({
  kind,
  id,
  role,
  title,
  keepFailed,
}: {
  kind: ShelfSubject["kind"];
  id: number;
  role: string;
  title: string;
  keepFailed: boolean;
}) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const videos = useSubjectVideos(kind, id, role);
  const { items, loading, loadingMore, error, hasMore, loadMore, retryLoadMore } = videos;
  const [retrying, setRetrying] = useState(false);
  const more = useCallback(() => loadMore(), [loadMore]);
  const sentinel = useLoadMoreSentinel(
    hasMore && !loading && !loadingMore && error === null,
    more,
    noPreviewReset,
  );
  if (loading) {
    if (!retrying) return null;
    return (
      <Shelf title={title}>
        <LoadingState label={text.loading} layout="grid" size={0} count={6} />
      </Shelf>
    );
  }
  if (items.length === 0) {
    if (!keepFailed || error === null) return null;
    const retry = () => {
      setRetrying(true);
      videos.reload();
    };
    return (
      <Shelf title={title}>
        <ErrorState
          title={text.videosFailed(error)}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      </Shelf>
    );
  }
  return (
    <Shelf title={title} count={t.credits.row.videoCount(videos.total)}>
      <CardGrid size={0}>
        {items.map((item) =>
          item.kind === "video" ? (
            <VideoCard
              key={item.video.id}
              video={item.video}
              backTo={backTo}
              selected={false}
              selectionMode={false}
            />
          ) : null,
        )}
      </CardGrid>
      {loadingMore && <LoadMoreRow status="loading" label={text.loadingMore} />}
      {error !== null && (
        <LoadMoreRow
          status="failed"
          title={text.moreFailed}
          description={error}
          retryLabel={t.common.retry}
          onRetry={retryLoadMore}
        />
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </Shelf>
  );
}

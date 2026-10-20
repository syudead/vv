import { Film } from "lucide-react";
import { useCallback, useMemo } from "react";
import { useLocation } from "react-router";

import type { components } from "../../api/gen/openapi";
import { useVideos, type VideosState } from "../../api/useVideos";
import { t } from "../../i18n";
import { CardGrid } from "../../ui/patterns/card-grid";
import { EmptyState } from "../../ui/patterns/empty-state";
import { ErrorState } from "../../ui/patterns/error-state";
import { LoadMoreRow } from "../../ui/patterns/load-more-row";
import { LoadingState } from "../../ui/patterns/loading-state";
import { Shelf } from "../../ui/patterns/shelf";
import { DEFAULT_SORT } from "../../videoList/listCriteria";
import { useLoadMoreSentinel } from "../../videoList/pageHooks";
import VideoCard from "../../videoList/VideoCard";

type Role = components["schemas"]["WorkPersonRole"];

const roles: readonly Role[] = ["performer", "director", "photographer", "streamer"];

const text = t.credits.personPage;

const noPreviewReset = () => undefined;

/** The person's videos in the library's default order, in one role or in any. */
function usePersonVideos(personId: number, role?: Role): VideosState {
  const criteria = useMemo(
    () => ({ sort: DEFAULT_SORT, person: personId, personRole: role }),
    [personId, role],
  );
  return useVideos(criteria, undefined, "videos");
}

/**
 * One shelf per role the person has videos in, in the roles' order. The person's videos in any role
 * stand for the shelves while loading or failing, and with none the empty state shows instead.
 */
export function PersonShelves({ personId }: { personId: number }) {
  const all = usePersonVideos(personId);
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
      {roles.map((role) => (
        <RoleShelf key={role} personId={personId} role={role} />
      ))}
    </>
  );
}

/**
 * Every video in one role, wrapping and read a page at a time as the end comes into view. A role
 * without videos, or whose first read failed, draws nothing; the other shelves still show.
 */
function RoleShelf({ personId, role }: { personId: number; role: Role }) {
  const location = useLocation();
  const backTo = `${location.pathname}${location.search}`;
  const videos = usePersonVideos(personId, role);
  const { items, loading, loadingMore, error, hasMore, loadMore, retryLoadMore } = videos;
  const more = useCallback(() => loadMore(), [loadMore]);
  const sentinel = useLoadMoreSentinel(
    hasMore && !loading && !loadingMore && error === null,
    more,
    noPreviewReset,
  );
  if (loading || items.length === 0) return null;
  return (
    <Shelf title={text.roles[role]} count={t.credits.row.videoCount(videos.total)}>
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
